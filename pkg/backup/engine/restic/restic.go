package restic

import (
	"context"
	"fmt"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/backup/engine"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/util/acceleratedio"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/datamover"
	"github.com/harvester/harvester/pkg/util/jobcredentials"
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
	imports       datamover.Snapshots
	vmbo          common.VMBackupOperator
	vsHelper      *common.VolumeSnapshotHelper
	vsClient      ctlsnapshotv1.VolumeSnapshotClient
	pvcCache      ctlcorev1.PersistentVolumeClaimCache
	pvcClient     ctlcorev1.PersistentVolumeClaimClient
	scCache       ctlstoragev1.StorageClassCache
	secretClient  ctlcorev1.SecretClient
	targets       backuputil.TargetClient
	jobController ctlbatchv1.JobController
	jobClient     ctlbatchv1.JobClient
	clientset     kubernetes.Interface
}

func GetBackupEngine(
	vmbo common.VMBackupOperator,
	vsCache ctlsnapshotv1.VolumeSnapshotCache,
	vsClient ctlsnapshotv1.VolumeSnapshotClient,
	snapshotClient dynamic.Interface,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	pvcClient ctlcorev1.PersistentVolumeClaimClient,
	secretClient ctlcorev1.SecretClient,
	settingClient ctlharvesterv1.SettingClient,
	scCache ctlstoragev1.StorageClassCache,
	jobController ctlbatchv1.JobController,
	clientset kubernetes.Interface,
) engine.BackupEngine {
	return &ResticEngine{
		imports:       datamover.Snapshots{Client: snapshotClient},
		vmbo:          vmbo,
		vsHelper:      common.NewVolumeSnapshotHelper(vsCache, vsClient, nil, nil, vmbo, pvcCache, scCache),
		vsClient:      vsClient,
		pvcCache:      pvcCache,
		pvcClient:     pvcClient,
		scCache:       scCache,
		secretClient:  secretClient,
		targets:       backuputil.TargetClient{Settings: settingClient, Secrets: secretClient},
		jobController: jobController,
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
			enqueue(job.Annotations[joblifecycle.AnnotationParentNamespace], name)
			return job, nil
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
	jobName, err := re.jobName(vmb, vb)
	if err != nil {
		return err
	}
	// Prefer the existing Job result. A failed or running Job must not be
	// overridden by a previously cached remote snapshot check.
	job, err := joblifecycle.GetJob(re.jobClient, vmb, vmBackupKind, jobName)
	if apierrors.IsNotFound(err) {
		return re.startBackupJob(vmb, vb, jobName)
	}
	if err != nil {
		return err
	}
	if err := re.ensureSecret(vmb, job); err != nil {
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

func (re *ResticEngine) refreshProgress(vb *harvesterv1.VolumeBackup, namespace, jobName string) int {
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

func (re *ResticEngine) UpdateProgress(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) (int64, error) {
	if re.vmbo.GetVolBackupReadyToUse(vb) {
		return 100, nil
	}
	jobName, nameErr := re.jobName(vmb, vb)
	if nameErr != nil {
		return int64(re.vmbo.GetVolBackupProgress(vb)), nil
	}
	return int64(re.refreshProgress(vb, joblifecycle.Namespace, jobName)), nil
}

// Remove records completed or skipped remote cleanup before deleting Jobs.
func (re *ResticEngine) Remove(
	vmb *harvesterv1.VirtualMachineBackup,
	removeRemote bool,
) (*harvesterv1.VirtualMachineBackup, error) {
	if err := re.Cleanup(vmb); err != nil {
		return nil, err
	}

	if err := re.forgetVolumes(vmb, removeRemote); err != nil {
		return nil, err
	}
	updated, err := common.RecordCleanup(re.vmbo, vmb, removeRemote)
	if err != nil {
		return nil, err
	}
	return updated, re.deleteJobs(updated)
}

func (re *ResticEngine) forgetVolumes(vmb *harvesterv1.VirtualMachineBackup, removeRemote bool) error {
	if !removeRemote || common.CleanupRecorded(re.vmbo, vmb) {
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
	if err := re.ensureSecret(vmb, job); err != nil {
		return err
	}
	return re.syncFromForgetJob(job)
}

func (re *ResticEngine) deleteJobs(vmb *harvesterv1.VirtualMachineBackup) error {
	for _, vb := range re.vmbo.GetVolBackups(vmb) {
		if re.vmbo.GetVolBackupName(&vb) == nil {
			continue
		}
		name, err := re.forgetJobName(vmb, &vb)
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
	forgetName, err := re.forgetJobName(vmb, vb)
	if err != nil {
		return nil, err
	}
	namespace := joblifecycle.Namespace
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

func (re *ResticEngine) ensureClonePVC(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup, name string) error {
	if err := re.imports.Import(vmb, common.SnapshotName(vmb, vb), re.importedSnapshot(vmb, vb)); err != nil {
		return pending(err)
	}
	pvc, err := re.pvcCache.Get(joblifecycle.Namespace, name)
	if apierrors.IsNotFound(err) {
		return re.createClonePVC(vmb, vb, name)
	}
	if err != nil {
		return err
	}
	if err := joblifecycle.Validate(vmb, vmBackupKind, pvc.ObjectMeta); err != nil {
		return err
	}
	if err := datamover.SnapshotSource(pvc, re.importedSnapshot(vmb, vb)); err != nil {
		return err
	}
	if pvc.DeletionTimestamp != nil || (re.waitsForImmediateBinding(pvc) && pvc.Status.Phase != corev1.ClaimBound) {
		return engine.ErrRetryLater
	}
	return nil
}

func (re *ResticEngine) createClonePVC(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	clonePVCName string,
) error {
	namespace := joblifecycle.Namespace
	sourceSpec := re.vmbo.GetVolBackupPVCSpec(vb)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clonePVCName,
			Namespace: namespace,
		},
		Spec: sourceSpec,
	}
	pvc.Spec.VolumeName = ""
	pvc.Spec.DataSource = &corev1.TypedLocalObjectReference{
		APIGroup: ptr.To("snapshot.storage.k8s.io"),
		Kind:     "VolumeSnapshot",
		Name:     re.importedSnapshot(vmb, vb),
	}
	pvc.Spec.DataSourceRef = nil
	joblifecycle.Track(&pvc.ObjectMeta, vmb, vmBackupKind)
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

func (re *ResticEngine) createBackupJob(
	vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup,
	clonePVCName, jobName string,
) error {
	runtime, err := re.jobRuntime(vmb, jobName)
	if err != nil {
		return err
	}
	vbName := re.vmbo.GetVolBackupName(vb)
	if vbName == nil {
		return fmt.Errorf("volume backup name is nil")
	}
	command := resticutil.BackupCommand(
		runtime.Command,
		acceleratedio.ReadCommand("/dev/"+clonePVCName),
		re.vmbo.GetVolBackupPVName(vb), re.snapshotTags(vmb, *vbName),
	)
	return re.startJob(vmb, newBackupJob(runtime, command, clonePVCName, jobName))
}

func newBackupJob(runtime *resticutil.JobRuntime, command, pvcName, name string) *batchv1.Job {
	labels := runtime.Labels
	volumes := runtime.VolumesWith(corev1.Volume{
		Name: "volume",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
		},
	})
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: joblifecycle.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To[int32](0),
			TTLSecondsAfterFinished: ptr.To[int32](60),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:         "backup",
						Image:        runtime.Image,
						Env:          runtime.Env,
						Command:      []string{"/bin/sh", "-c"},
						Resources:    runtime.Resources,
						VolumeMounts: runtime.VolumeMounts,
						Args:         []string{command},
						VolumeDevices: []corev1.VolumeDevice{{
							Name:       "volume",
							DevicePath: "/dev/" + pvcName,
						}},
					}},
					Volumes: volumes,
				},
			},
		},
	}
	job.Finalizers = []string{joblifecycle.CompletionFinalizer}
	return job
}

func (re *ResticEngine) checkRemoteSnapshot(
	vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup,
) (bool, error) {
	name, err := re.checkJobName(vmb, vb)
	if err != nil {
		return false, err
	}
	result, err := resticutil.CheckSnapshotJob(
		context.Background(), re.clientset, name,
		func(name string) (*batchv1.Job, error) {
			return re.getCheckJob(vmb, name)
		},
		func(name string) error {
			return re.createCheckJob(vmb, vb, name)
		},
	)
	if err != nil {
		return false, err
	}
	return re.checkResult(vmb, vb, name, result)
}

func (re *ResticEngine) getCheckJob(vmb *harvesterv1.VirtualMachineBackup, name string) (*batchv1.Job, error) {
	job, err := joblifecycle.GetJob(re.jobClient, vmb, vmBackupKind, name)
	if err != nil {
		return nil, err
	}
	return job, re.ensureSecret(vmb, job)
}

func (re *ResticEngine) checkResult(
	vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup,
	name string, result resticutil.SnapshotCheckResult,
) (bool, error) {
	// New backups can proceed to upload when the remote snapshot is absent or
	// its check failed. Discovered backups have no local source to upload.
	if !re.isDiscoveredBackup(vmb) &&
		(result == resticutil.SnapshotCheckMissing || result == resticutil.SnapshotCheckFailed) {
		return false, nil
	}
	switch result {
	case resticutil.SnapshotCheckPending:
		return false, engine.ErrRetryLater
	case resticutil.SnapshotCheckMissing:
		return false, fmt.Errorf("restic snapshot for discovered backup %s/%s not found", joblifecycle.Namespace, name)
	case resticutil.SnapshotCheckFailed:
		return false, fmt.Errorf("restic snapshot check job %s/%s failed", joblifecycle.Namespace, name)
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

func (re *ResticEngine) createCheckJob(
	vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup, jobName string,
) error {
	runtime, err := re.jobRuntime(vmb, jobName)
	if err != nil {
		return err
	}
	vbName := re.vmbo.GetVolBackupName(vb)
	if vbName == nil || *vbName == "" {
		return fmt.Errorf("volume backup name is nil")
	}
	job := resticutil.NewSnapshotCheckJob(resticutil.SnapshotCheckJobOptions{
		Name:      jobName,
		Namespace: joblifecycle.Namespace,
		Labels:    runtime.Labels,
		Runtime:   runtime,
		Tags:      re.snapshotTags(vmb, *vbName),
	})
	return re.startJob(vmb, job)
}

// createForgetJob retains its credentials through Job ownership until cleanup completes.
func (re *ResticEngine) createForgetJob(
	vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup, jobName string,
) error {
	runtime, err := re.jobRuntime(vmb, jobName)
	if err != nil {
		return err
	}
	vbName := re.vmbo.GetVolBackupName(vb)
	if vbName == nil || *vbName == "" {
		return nil
	}
	command := resticutil.ForgetCommand(runtime.Command, re.snapshotTags(vmb, *vbName))
	return re.startJob(vmb, newForgetJob(runtime, command, jobName))
}

func newForgetJob(runtime *resticutil.JobRuntime, command, name string) *batchv1.Job {
	labels := runtime.Labels
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  joblifecycle.Namespace,
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
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:         "forget",
						Image:        runtime.Image,
						Env:          runtime.Env,
						Command:      []string{"/bin/sh", "-c"},
						Resources:    runtime.Resources,
						VolumeMounts: runtime.VolumeMounts,
						Args:         []string{command},
					}},
					Volumes: runtime.Volumes,
				},
			},
		},
	}
	return job
}

func (re *ResticEngine) jobRuntime(
	vmb *harvesterv1.VirtualMachineBackup, name string,
) (*resticutil.JobRuntime, error) {
	repository, err := re.repository(vmb)
	if err != nil {
		return nil, err
	}
	runtime, hasCapacity, err := resticutil.NewJobRuntime(
		context.Background(), re.clientset, jobcredentials.Name(name), repository, resticutil.JobLabels(nil),
	)
	if err != nil {
		return nil, err
	}
	if !hasCapacity {
		return nil, engine.ErrRetryLater
	}
	return runtime, nil
}

func (re *ResticEngine) startJob(vmb *harvesterv1.VirtualMachineBackup, job *batchv1.Job) error {
	data, err := re.targets.ThirdPartyCredentials(
		re.vmbo.GetBackupTarget(vmb), resticutil.PasswordKey,
	)
	if err != nil {
		return err
	}
	joblifecycle.Track(&job.ObjectMeta, vmb, vmBackupKind)
	client := jobcredentials.Client{Jobs: re.jobClient, Secrets: re.secretClient}
	job, err = client.Create(job)
	if apierrors.IsAlreadyExists(err) {
		return engine.ErrRetryLater
	}
	if err != nil {
		return err
	}
	return client.Ensure(job, func() (map[string][]byte, error) {
		return data, nil
	})
}

func (re *ResticEngine) snapshotTags(vmb *harvesterv1.VirtualMachineBackup, name string) []string {
	return []string{
		resticutil.NamespaceTag(re.vmbo.GetNamespace(vmb)),
		resticutil.VMBackupTag(re.vmbo.GetName(vmb)),
		resticutil.SnapshotTag(name),
	}
}

func (re *ResticEngine) ensureSecret(vmb *harvesterv1.VirtualMachineBackup, job *batchv1.Job) error {
	client := jobcredentials.Client{Jobs: re.jobClient, Secrets: re.secretClient}
	return client.Ensure(job, func() (map[string][]byte, error) {
		return re.targets.ThirdPartyCredentials(
			re.vmbo.GetBackupTarget(vmb), resticutil.PasswordKey,
		)
	})
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
		re.refreshProgress(vb, job.Namespace, job.Name)
		return engine.ErrRetryLater
	}
	if _, err := re.markReady(vb); err != nil {
		return err
	}
	// Cleanup is deferred to the next reconcile (when ReadyToUse is persisted)
	// to avoid a race where Update fails after we've deleted the VS/clone PVC.
	logrus.Debugf("restic backup job %s/%s completed", job.Namespace, job.Name)
	return nil
}

// Cleanup removes temporary resources after upload or before backup deletion.
func (re *ResticEngine) Cleanup(vmb *harvesterv1.VirtualMachineBackup) error {
	for _, vb := range re.vmbo.GetVolBackups(vmb) {
		if re.vmbo.GetVolBackupName(&vb) == nil {
			continue
		}
		if err := re.cleanupTemporaryResources(vmb, &vb); err != nil {
			return err
		}
	}
	return nil
}

// cleanupTemporaryResources stops consumers before removing the clone and
// imported snapshot. The original snapshot is deleted last, with its CSI
// protection finalizers intact. The same order applies on cancellation.
func (re *ResticEngine) cleanupTemporaryResources(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) error {
	if err := re.stopVolumeJobs(vmb, vb); err != nil {
		return err
	}
	if err := re.deleteClonePVC(vmb, vb); err != nil {
		return pending(err)
	}
	if err := re.imports.Delete(vmb, re.importedSnapshot(vmb, vb)); err != nil {
		return pending(err)
	}
	return re.deleteSnapshot(vmb, common.SnapshotName(vmb, vb))
}

func (re *ResticEngine) stopVolumeJobs(
	vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup,
) error {
	for _, prefix := range []string{jobNamePrefix, checkJobNamePrefix} {
		name := joblifecycle.Name(prefix, vmb, re.volumeSnapshotName(vb))
		if err := datamover.StopJob(re.clientset, vmb, vmBackupKind, name); err != nil {
			return pending(err)
		}
	}
	return nil
}

func (re *ResticEngine) deleteSnapshot(vmb *harvesterv1.VirtualMachineBackup, name string) error {
	if name == "" {
		return nil
	}
	snapshot, err := re.vsClient.Get(vmb.Namespace, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := joblifecycle.Validate(vmb, vmBackupKind, snapshot.ObjectMeta); err != nil {
		return err
	}
	err = re.vsClient.Delete(vmb.Namespace, name, &metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &snapshot.UID}})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return engine.ErrRetryLater
}

func (re *ResticEngine) volumeSnapshotName(vb *harvesterv1.VolumeBackup) string {
	if name := re.vmbo.GetVolBackupName(vb); name != nil {
		return *name
	}
	return ""
}

func (re *ResticEngine) clonePVCName(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) string {
	return joblifecycle.Name("restic-clone", vmb, re.volumeSnapshotName(vb))
}

func (re *ResticEngine) importedSnapshot(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) string {
	return joblifecycle.Name("restic-snapshot", vmb, re.volumeSnapshotName(vb))
}

func (re *ResticEngine) jobName(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) (string, error) {
	name := re.vmbo.GetVolBackupName(vb)
	if name == nil {
		return "", fmt.Errorf("volume backup name is nil")
	}
	return joblifecycle.Name(jobNamePrefix, vmb, *name), nil
}

func (re *ResticEngine) forgetJobName(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) (string, error) {
	name := re.vmbo.GetVolBackupName(vb)
	if name == nil {
		return "", fmt.Errorf("volume backup name is nil")
	}
	return joblifecycle.Name(forgetJobNamePrefix, vmb, *name), nil
}

func (re *ResticEngine) checkJobName(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) (string, error) {
	name := re.vmbo.GetVolBackupName(vb)
	if name == nil {
		return "", fmt.Errorf("volume backup name is nil")
	}
	return joblifecycle.Name(checkJobNamePrefix, vmb, *name), nil
}

func pending(err error) error {
	if datamover.Retryable(err) {
		return engine.ErrRetryLater
	}
	return err
}

func (re *ResticEngine) deleteClonePVC(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) error {
	name := re.clonePVCName(vmb, vb)
	pvc, err := re.clientset.CoreV1().PersistentVolumeClaims(joblifecycle.Namespace).Get(context.Background(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := joblifecycle.Validate(vmb, vmBackupKind, pvc.ObjectMeta); err != nil {
		return err
	}
	if err := datamover.SnapshotSource(pvc, re.importedSnapshot(vmb, vb)); err != nil {
		return err
	}
	return datamover.DeleteClaim(re.clientset, vmb, vmBackupKind, name)
}
