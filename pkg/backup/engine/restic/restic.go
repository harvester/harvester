package restic

import (
	"context"
	"errors"
	"fmt"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	wranglername "github.com/rancher/wrangler/v3/pkg/name"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/backup/engine"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
	"github.com/harvester/harvester/pkg/util/jobprogress"
	resticutil "github.com/harvester/harvester/pkg/util/restic"
)

const (
	jobNamePrefix       = "restic-backup"
	forgetJobNamePrefix = "restic-forget"
	checkJobNamePrefix  = "restic-check"
	vmBackupKind        = "VirtualMachineBackup"
)

type ResticEngine struct {
	vmbo          common.VMBackupOperator
	vsHelper      *common.VolumeSnapshotHelper
	vsClient      ctlsnapshotv1.VolumeSnapshotClient
	pvcCache      ctlcorev1.PersistentVolumeClaimCache
	pvcClient     ctlcorev1.PersistentVolumeClaimClient
	scCache       ctlstoragev1.StorageClassCache
	secretClient  ctlcorev1.SecretClient
	jobController ctlbatchv1.JobController
	jobCache      ctlbatchv1.JobCache
	jobClient     ctlbatchv1.JobClient
	clientset     kubernetes.Interface
}

func GetBackupEngine(
	vmbo common.VMBackupOperator,
	vsCache ctlsnapshotv1.VolumeSnapshotCache,
	vsClient ctlsnapshotv1.VolumeSnapshotClient,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	pvcClient ctlcorev1.PersistentVolumeClaimClient,
	secretClient ctlcorev1.SecretClient,
	scCache ctlstoragev1.StorageClassCache,
	jobController ctlbatchv1.JobController,
	clientset kubernetes.Interface,
) engine.BackupEngine {
	return &ResticEngine{
		vmbo:          vmbo,
		vsHelper:      common.NewVolumeSnapshotHelper(vsCache, vsClient, nil, nil, vmbo, pvcCache, scCache),
		vsClient:      vsClient,
		pvcCache:      pvcCache,
		pvcClient:     pvcClient,
		scCache:       scCache,
		secretClient:  secretClient,
		jobController: jobController,
		jobCache:      jobController.Cache(),
		jobClient:     jobController,
		clientset:     clientset,
	}
}

// RegisterWatchers registers a Job event handler that maps Job changes back to
// the owning VMBackup and enqueues it. This
// removes polling latency — when our backup or forget Job's status changes the
// VMBackup is reconciled immediately.
func (re *ResticEngine) RegisterWatchers(ctx context.Context, enqueue func(namespace, name string)) {
	re.jobController.OnChange(ctx, "restic-backup-job-watcher", func(_ string, job *batchv1.Job) (*batchv1.Job, error) {
		if job == nil {
			return nil, nil
		}
		if job.Labels[resticutil.LabelResticJob] != resticutil.LabelValueTrue {
			return job, nil
		}
		if name := joblifecycle.ParentName(job.ObjectMeta, vmBackupKind); name != "" {
			enqueue(job.Namespace, name)
			return job, nil
		}
		owner := util.FindOwnerReference(
			job,
			harvesterv1.SchemeGroupVersion.String(),
			vmBackupKind,
		)
		if owner != nil {
			enqueue(job.Namespace, owner.Name)
		}
		return job, nil
	})
}

func (re *ResticEngine) Reconcile(
	vmb *harvesterv1.VirtualMachineBackup,
	volIndex int,
	_ map[string]snapshotv1.VolumeSnapshotClass,
) error {
	vb := re.vmbo.GetVolBackup(vmb, volIndex)
	if vb == nil {
		return fmt.Errorf("volume backup at index %d not found", volIndex)
	}
	if re.vmbo.GetVolBackupReadyToUse(vb) {
		// Backup is persisted in S3; the temporary clone PVC and VolumeSnapshot
		// are no longer needed. Cleanup is idempotent, so returning an error keeps
		// the VMBackup non-ready and lets the controller retry it.
		return re.cleanupTemporaryResources(vmb, vb)
	}
	if re.isDiscoveredBackup(vmb) {
		_, err := re.checkRemoteSnapshot(vmb, vb)
		return err
	}
	return re.reconcileBackupJob(vmb, vb)
}

func (re *ResticEngine) Prepare(
	vmb *harvesterv1.VirtualMachineBackup,
	vsClassMap map[string]snapshotv1.VolumeSnapshotClass,
) (bool, error) {
	if re.isDiscoveredBackup(vmb) {
		return true, nil
	}
	return re.vsHelper.PrepareSnapshots(vmb, vsClassMap)
}

func (re *ResticEngine) reconcileBackupJob(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
) error {
	jobName, err := re.jobName(vb)
	if err != nil {
		return err
	}
	// Prefer the existing Job result. A failed or running Job must not be
	// overridden by a previously cached remote snapshot check.
	job, err := re.jobCache.Get(re.vmbo.GetNamespace(vmb), jobName)
	if apierrors.IsNotFound(err) {
		return re.startBackupJob(vmb, vb, jobName)
	}
	if err != nil {
		return err
	}
	return re.syncFromJob(vb, job)
}

func (re *ResticEngine) startBackupJob(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	jobName string,
) error {
	if ready, err := re.checkRemoteSnapshot(vmb, vb); ready || err != nil {
		return err
	}

	clonePVCName := re.clonePVCName(vmb, vb)
	if err := re.ensureClonePVC(vmb, vb, clonePVCName); err != nil {
		return err
	}
	if err := re.createBackupJob(vmb, vb, clonePVCName, jobName); err != nil {
		return err
	}
	return engine.ErrRetryLater
}

func (re *ResticEngine) refreshProgressFromLogs(vb *harvesterv1.VolumeBackup, namespace, jobName string) int {
	current := re.vmbo.GetVolBackupProgress(vb)
	pct, err := jobprogress.FetchProgressPercent(context.Background(), re.clientset, namespace, jobName)
	if err != nil {
		return current
	}
	if err := re.vmbo.SetVolBackupProgress(vb, pct); err != nil {
		return current
	}
	return pct
}

func (re *ResticEngine) UpdateProgress(vb *harvesterv1.VolumeBackup) (int64, error) {
	if re.vmbo.GetVolBackupReadyToUse(vb) {
		return 100, nil
	}
	jobName, jobNamespace, ok := re.jobLocator(vb)
	if !ok {
		return int64(re.vmbo.GetVolBackupProgress(vb)), nil
	}
	return int64(re.refreshProgressFromLogs(vb, jobNamespace, jobName)), nil
}

// Remove records completed or skipped remote cleanup before deleting Jobs.
func (re *ResticEngine) Remove(
	vmb *harvesterv1.VirtualMachineBackup,
	removeRemote bool,
) (*harvesterv1.VirtualMachineBackup, error) {
	if err := re.forgetVolumes(vmb, removeRemote); err != nil {
		return nil, err
	}
	updated, err := common.Commit(re.vmbo, vmb, removeRemote)
	if err != nil {
		return nil, err
	}
	return updated, re.deleteJobs(updated)
}

func (re *ResticEngine) forgetVolumes(vmb *harvesterv1.VirtualMachineBackup, removeRemote bool) error {
	if !removeRemote || common.Ready(re.vmbo, vmb) {
		return nil
	}
	for i, vb := range re.vmbo.GetVolBackups(vmb) {
		if re.vmbo.GetVolBackupName(&vb) == nil {
			continue
		}
		if err := re.forgetVolume(vmb, i); err != nil {
			return fmt.Errorf("failed to delete volume backup %s: %w", *re.vmbo.GetVolBackupName(&vb), err)
		}
	}
	return nil
}

func (re *ResticEngine) forgetVolume(vmb *harvesterv1.VirtualMachineBackup, volIndex int) error {
	vb := re.vmbo.GetVolBackup(vmb, volIndex)
	if vb == nil {
		return nil
	}
	job, err := re.ensureForgetJob(vmb, vb)
	if err != nil {
		return err
	}
	if err := joblifecycle.ValidateJob(vmb, vmBackupKind, job); err != nil {
		return err
	}
	return re.syncFromForgetJob(job)
}

func (re *ResticEngine) deleteJobs(vmb *harvesterv1.VirtualMachineBackup) error {
	for _, vb := range re.vmbo.GetVolBackups(vmb) {
		if re.vmbo.GetVolBackupName(&vb) == nil {
			continue
		}
		name, err := re.forgetJobName(&vb)
		if err != nil {
			return err
		}
		if err := re.deleteForgetJob(vmb, name); err != nil {
			return err
		}
	}
	return nil
}

func (re *ResticEngine) deleteForgetJob(vmb *harvesterv1.VirtualMachineBackup, name string) error {
	return joblifecycle.DeleteJob(re.jobClient, vmb, vmBackupKind, name)
}

func (re *ResticEngine) ensureForgetJob(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
) (*batchv1.Job, error) {
	forgetName, err := re.forgetJobName(vb)
	if err != nil {
		return nil, err
	}
	namespace := re.vmbo.GetNamespace(vmb)
	job, err := re.jobClient.Get(namespace, forgetName, metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		return job, err
	}
	if err := re.createForgetJob(vmb, vb, forgetName); err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create restic forget job %s/%s: %w", namespace, forgetName, err)
	}
	return nil, engine.ErrRetryLater
}

// syncFromForgetJob retains successful Jobs as per-volume cleanup checkpoints.
// A failed pod is not terminal while the Job still has retries available.
// Terminal failures are removed so requeue can create a fresh attempt without
// depending on the TTL controller.
func (re *ResticEngine) syncFromForgetJob(job *batchv1.Job) error {
	if job.Status.Succeeded > 0 {
		return nil
	}
	if job.DeletionTimestamp != nil {
		return re.retryForgetJob(job)
	}
	for _, condition := range job.Status.Conditions {
		if condition.Type != batchv1.JobFailed || condition.Status != corev1.ConditionTrue {
			continue
		}
		return re.retryForgetJob(job)
	}
	return engine.ErrRetryLater
}

func (re *ResticEngine) retryForgetJob(job *batchv1.Job) error {
	if err := joblifecycle.Delete(re.jobClient, job); err != nil {
		return err
	}
	return fmt.Errorf("restic forget job %s/%s failed or was interrupted; retrying cleanup", job.Namespace, job.Name)
}

func (re *ResticEngine) ensureClonePVC(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	clonePVCName string,
) error {
	namespace := re.vmbo.GetNamespace(vmb)
	pvc, err := re.pvcCache.Get(namespace, clonePVCName)
	if apierrors.IsNotFound(err) {
		return re.createClonePVC(vmb, vb, clonePVCName)
	}
	if err != nil {
		return err
	}
	if err := common.ValidateClonePVC(
		pvc, re.vsHelper.BuildOwnerReference(vmb), re.volumeSnapshotName(vb),
	); err != nil {
		return err
	}
	if pvc.DeletionTimestamp != nil {
		return engine.ErrRetryLater
	}
	// WaitForFirstConsumer storage classes bind once a backup Job mounts the
	// PVC, so waiting for ClaimBound here would deadlock that path.
	if re.waitsForImmediateBinding(pvc) && pvc.Status.Phase != corev1.ClaimBound {
		return engine.ErrRetryLater
	}
	return nil
}

func (re *ResticEngine) createClonePVC(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	clonePVCName string,
) error {
	namespace := re.vmbo.GetNamespace(vmb)
	sourceSpec := re.vmbo.GetVolBackupPVCSpec(vb)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            clonePVCName,
			Namespace:       namespace,
			OwnerReferences: []metav1.OwnerReference{re.vsHelper.BuildOwnerReference(vmb)},
		},
		Spec: sourceSpec,
	}
	pvc.Spec.VolumeName = ""
	pvc.Spec.DataSource = &corev1.TypedLocalObjectReference{
		APIGroup: ptr.To("snapshot.storage.k8s.io"),
		Kind:     "VolumeSnapshot",
		Name:     re.volumeSnapshotName(vb),
	}
	pvc.Spec.DataSourceRef = nil
	_, err := re.pvcClient.Create(pvc)
	if apierrors.IsAlreadyExists(err) {
		return engine.ErrRetryLater
	}
	return err
}

// waitsForImmediateBinding reports whether the PVC's StorageClass binds eagerly,
// in which case ClaimBound is reachable without a consumer pod. If the SC is
// missing or unreadable we conservatively assume Immediate (the K8s default).
func (re *ResticEngine) waitsForImmediateBinding(pvc *corev1.PersistentVolumeClaim) bool {
	scName := ptr.Deref(pvc.Spec.StorageClassName, "")
	if scName == "" {
		return true
	}
	sc, err := re.scCache.Get(scName)
	if err != nil || sc.VolumeBindingMode == nil {
		return true
	}
	return *sc.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer
}

func (re *ResticEngine) createBackupJob(vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	clonePVCName, jobName string,
) error {
	runtime, err := re.jobRuntime(vmb)
	if err != nil {
		return err
	}
	labels := runtime.Labels

	namespace := re.vmbo.GetNamespace(vmb)
	pvName := re.vmbo.GetVolBackupPVName(vb)
	volumes := runtime.VolumesWith(corev1.Volume{
		Name: "volume",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: clonePVCName},
		},
	})
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            jobName,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{re.vsHelper.BuildOwnerReference(vmb)},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To[int32](0),
			TTLSecondsAfterFinished: ptr.To[int32](60),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:           backuputil.JobServiceAccount,
					AutomountServiceAccountToken: ptr.To(true),
					RestartPolicy:                corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:         "backup",
						Image:        runtime.Image,
						Env:          runtime.Env,
						Command:      runtime.Entrypoint,
						Resources:    runtime.Resources,
						VolumeMounts: runtime.VolumeMounts,
						Args: []string{resticutil.BackupCommand(
							runtime.Command,
							fmt.Sprintf("/usr/bin/harvester io-mode -device /dev/%s -mode=read", clonePVCName),
							pvName,
							[]string{
								resticutil.NamespaceTag(namespace),
								resticutil.VMBackupTag(re.vmbo.GetName(vmb)),
								resticutil.SnapshotTag(*re.vmbo.GetVolBackupName(vb)),
							},
						)},
						VolumeDevices: []corev1.VolumeDevice{{
							Name:       "volume",
							DevicePath: "/dev/" + clonePVCName,
						}},
					}},
					Volumes: volumes,
				},
			},
		},
	}
	_, err = re.jobClient.Create(job)
	if apierrors.IsAlreadyExists(err) {
		return engine.ErrRetryLater
	}
	if err != nil {
		return err
	}
	return nil
}

func (re *ResticEngine) checkRemoteSnapshot(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) (bool, error) {
	checkName, err := re.checkJobName(vb)
	if err != nil {
		return false, err
	}
	namespace := re.vmbo.GetNamespace(vmb)
	result, err := resticutil.CheckSnapshotJob(
		context.Background(),
		re.clientset,
		checkName,
		func(name string) (*batchv1.Job, error) {
			return re.jobCache.Get(namespace, name)
		},
		func(name string) error {
			return re.createCheckJob(vmb, vb, name)
		},
	)
	if err != nil {
		return false, err
	}
	discoveredBackup := re.isDiscoveredBackup(vmb)
	switch result {
	case resticutil.SnapshotCheckPending:
		return false, engine.ErrRetryLater
	case resticutil.SnapshotCheckMissing:
		if discoveredBackup {
			return false, fmt.Errorf("restic snapshot for discovered backup %s/%s not found", namespace, checkName)
		}
		return false, nil
	case resticutil.SnapshotCheckFailed:
		if discoveredBackup {
			return false, fmt.Errorf("restic snapshot check job %s/%s failed", namespace, checkName)
		}
		return false, nil
	case resticutil.SnapshotCheckFound:
		return re.markReady(vb)
	default:
		return false, fmt.Errorf("unknown restic snapshot check result %q", result)
	}
}

func (re *ResticEngine) isDiscoveredBackup(vmb *harvesterv1.VirtualMachineBackup) bool {
	return re.vmbo.GetSourceUID(vmb) == nil
}

func (re *ResticEngine) markReady(vb *harvesterv1.VolumeBackup) (bool, error) {
	now := metav1.Now()
	ready := true
	if err := re.vmbo.SetVolBackupReadyToUse(vb, &ready); err != nil {
		return false, err
	}
	if err := re.vmbo.SetVolBackupCreationTime(vb, &now); err != nil {
		return false, err
	}
	if err := re.vmbo.SetVolBackupProgress(vb, 100); err != nil {
		return false, err
	}
	return true, nil
}

func (re *ResticEngine) createCheckJob(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup, jobName string) error {
	namespace := re.vmbo.GetNamespace(vmb)
	vbName := re.vmbo.GetVolBackupName(vb)
	if vbName == nil || *vbName == "" {
		return fmt.Errorf("volume backup name is nil")
	}
	runtime, err := re.jobRuntime(vmb)
	if err != nil {
		return err
	}
	labels := runtime.Labels
	job := resticutil.NewSnapshotCheckJob(resticutil.SnapshotCheckJobOptions{
		Name:            jobName,
		Namespace:       namespace,
		Labels:          labels,
		OwnerReferences: []metav1.OwnerReference{re.vsHelper.BuildOwnerReference(vmb)},
		Runtime:         runtime,
		Tags: []string{
			resticutil.NamespaceTag(namespace),
			resticutil.VMBackupTag(re.vmbo.GetName(vmb)),
			resticutil.SnapshotTag(*vbName),
		},
	})
	_, err = re.jobClient.Create(job)
	if apierrors.IsAlreadyExists(err) {
		return engine.ErrRetryLater
	}
	if err != nil {
		return err
	}
	return nil
}

// createForgetJob validates credentials before starting this cleanup attempt.
func (re *ResticEngine) createForgetJob(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup, jobName string) error {
	namespace := re.vmbo.GetNamespace(vmb)
	vbName := re.vmbo.GetVolBackupName(vb)
	if vbName == nil || *vbName == "" {
		return nil
	}
	runtime, err := re.jobRuntime(vmb)
	if err != nil {
		return err
	}
	labels := runtime.Labels
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:       jobName,
			Namespace:  namespace,
			Labels:     labels,
			Finalizers: []string{joblifecycle.CompletionFinalizer},
		},
		Spec: batchv1.JobSpec{
			// Keep results without TTL until the cleanup checkpoint is durable.
			BackoffLimit: ptr.To[int32](3),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:           backuputil.JobServiceAccount,
					AutomountServiceAccountToken: ptr.To(true),
					RestartPolicy:                corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:         "forget",
						Image:        runtime.Image,
						Env:          runtime.Env,
						Command:      runtime.Entrypoint,
						Resources:    runtime.Resources,
						VolumeMounts: runtime.VolumeMounts,
						Args: []string{resticutil.ForgetCommand(runtime.Command, []string{
							resticutil.NamespaceTag(namespace),
							resticutil.VMBackupTag(re.vmbo.GetName(vmb)),
							resticutil.SnapshotTag(*vbName),
						})},
					}},
					Volumes: runtime.Volumes,
				},
			},
		},
	}
	joblifecycle.Track(&job.ObjectMeta, vmb, vmBackupKind)
	_, err = re.jobClient.Create(job)
	if apierrors.IsAlreadyExists(err) {
		return engine.ErrRetryLater
	}
	if err != nil {
		return err
	}
	return nil
}

func (re *ResticEngine) jobRuntime(vmb *harvesterv1.VirtualMachineBackup) (*resticutil.JobRuntime, error) {
	repository, err := re.repository(vmb)
	if err != nil {
		return nil, err
	}
	err = backuputil.CheckCredentials(
		re.secretClient, re.vmbo.GetBackupTarget(vmb), resticutil.PasswordKey,
	)
	if err != nil {
		return nil, err
	}
	runtime, capacity, err := resticutil.NewJobRuntime(
		context.Background(), re.clientset, re.vmbo.GetRepositoryID(vmb),
		repository, resticutil.JobLabels(nil),
	)
	if err != nil {
		return nil, err
	}
	if !capacity {
		return nil, engine.ErrRetryLater
	}
	if err := backuputil.EnsureJobAccess(context.Background(), re.clientset, re.vmbo.GetNamespace(vmb)); err != nil {
		return nil, fmt.Errorf("preparing backup Job access: %w: %w", engine.ErrRetryLater, err)
	}
	return runtime, nil
}

func (re *ResticEngine) repository(vmb *harvesterv1.VirtualMachineBackup) (*resticutil.RepositoryConfig, error) {
	target, err := backuputil.TargetFromStatus(re.vmbo.GetBackupTarget(vmb))
	if err != nil {
		return nil, err
	}
	return resticutil.Repository(target)
}

func (re *ResticEngine) syncFromJob(vb *harvesterv1.VolumeBackup, job *batchv1.Job) error {
	if job.Status.Failed > 0 {
		return fmt.Errorf("restic backup job %s/%s failed", job.Namespace, job.Name)
	}
	if job.Status.Succeeded == 0 {
		// Job still running — update progress from io-mode pod logs (best effort).
		if pct, err := jobprogress.FetchProgressPercent(context.Background(), re.clientset, job.Namespace, job.Name); err == nil {
			_ = re.vmbo.SetVolBackupProgress(vb, pct)
		}
		return engine.ErrRetryLater
	}
	now := metav1.Now()
	ready := true
	if err := re.vmbo.SetVolBackupReadyToUse(vb, &ready); err != nil {
		return err
	}
	if err := re.vmbo.SetVolBackupCreationTime(vb, &now); err != nil {
		return err
	}
	if err := re.vmbo.SetVolBackupProgress(vb, 100); err != nil {
		return err
	}
	// Cleanup is deferred to the next reconcile (when ReadyToUse is persisted)
	// to avoid a race where Update fails after we've deleted the VS/clone PVC.
	logrus.Debugf("restic backup job %s/%s completed", job.Namespace, job.Name)
	return nil
}

// cleanupTemporaryResources removes the backup Job, cloned PVC, and
// VolumeSnapshot created for this backup. Called after ReadyToUse is
// persisted, so the Job has already succeeded (otherwise we wouldn't be
// here) and failure of any step is safe to retry on the next reconcile.
//
// Order matters: delete the Job first so its Pod is GC'd (Background
// propagation), otherwise the clone PVC would stay stuck on the
// kubernetes.io/pvc-protection finalizer until TTLSecondsAfterFinished
// reaped the Pod about a minute later.
func (re *ResticEngine) cleanupTemporaryResources(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
) error {
	namespace := re.vmbo.GetNamespace(vmb)
	return errors.Join(
		re.deleteBackupJob(namespace, vb),
		re.deleteClonePVC(vmb, vb),
		re.deleteSnapshot(namespace, re.volumeSnapshotName(vb)),
	)
}

func (re *ResticEngine) deleteBackupJob(namespace string, vb *harvesterv1.VolumeBackup) error {
	jobName, err := re.jobName(vb)
	if err != nil {
		return err
	}

	err = re.jobClient.Delete(namespace, jobName, &metav1.DeleteOptions{
		PropagationPolicy: ptr.To(metav1.DeletePropagationBackground),
	})

	if err == nil || apierrors.IsNotFound(err) {
		return nil
	}

	return fmt.Errorf(
		"failed to delete restic backup job %s/%s: %w",
		namespace, jobName, err,
	)
}

func (re *ResticEngine) deleteClonePVC(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
) error {
	namespace := re.vmbo.GetNamespace(vmb)
	name := re.clonePVCName(vmb, vb)
	err := common.DeleteClonePVC(
		re.pvcClient, namespace, name,
		re.vsHelper.BuildOwnerReference(vmb), re.volumeSnapshotName(vb),
	)
	if err == nil || apierrors.IsNotFound(err) {
		return nil
	}
	return fmt.Errorf(
		"failed to delete restic temporary PVC %s/%s: %w",
		namespace, name, err,
	)
}

func (re *ResticEngine) deleteSnapshot(namespace, name string) error {
	if name == "" {
		return nil
	}
	err := re.vsClient.Delete(namespace, name, &metav1.DeleteOptions{})
	if err == nil || apierrors.IsNotFound(err) {
		return nil
	}
	return fmt.Errorf(
		"failed to delete restic temporary VolumeSnapshot %s/%s: %w",
		namespace, name, err,
	)
}

func (re *ResticEngine) volumeSnapshotName(vb *harvesterv1.VolumeBackup) string {
	if name := re.vmbo.GetVolBackupName(vb); name != nil {
		return *name
	}
	return ""
}

func (re *ResticEngine) clonePVCName(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) string {
	return wranglername.SafeConcatName(
		re.vmbo.GetName(vmb),
		re.vmbo.GetVolBackupPVCName(vb),
		"restic-clone",
	)
}

func (re *ResticEngine) jobName(vb *harvesterv1.VolumeBackup) (string, error) {
	name := re.vmbo.GetVolBackupName(vb)
	if name == nil {
		return "", fmt.Errorf("volume backup name is nil")
	}
	return wranglername.SafeConcatName(jobNamePrefix, *name), nil
}

func (re *ResticEngine) forgetJobName(vb *harvesterv1.VolumeBackup) (string, error) {
	name := re.vmbo.GetVolBackupName(vb)
	if name == nil {
		return "", fmt.Errorf("volume backup name is nil")
	}
	return wranglername.SafeConcatName(forgetJobNamePrefix, *name), nil
}

func (re *ResticEngine) checkJobName(vb *harvesterv1.VolumeBackup) (string, error) {
	name := re.vmbo.GetVolBackupName(vb)
	if name == nil {
		return "", fmt.Errorf("volume backup name is nil")
	}
	return wranglername.SafeConcatName(checkJobNamePrefix, *name), nil
}

// jobLocator returns the name/namespace to look up the backup Job for vb,
// or false if the volume backup name isn't set yet. Used by UpdateProgress
// which has only a VolumeBackup handle (no enclosing VMBackup). The Job is
// created in the same namespace as the source PVC.
func (re *ResticEngine) jobLocator(vb *harvesterv1.VolumeBackup) (name, namespace string, ok bool) {
	vbName := re.vmbo.GetVolBackupName(vb)
	if vbName == nil {
		return "", "", false
	}
	ns := re.vmbo.GetVolBackupPVCNameSpace(vb)
	if ns == "" {
		return "", "", false
	}
	return wranglername.SafeConcatName(jobNamePrefix, *vbName), ns, true
}
