package kopia

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
	kopiautil "github.com/harvester/harvester/pkg/util/kopia"
)

const (
	jobNamePrefix       = "kopia-backup"
	forgetJobNamePrefix = "kopia-forget"
	checkJobNamePrefix  = "kopia-check"
	vmBackupKind        = "VirtualMachineBackup"
)

type KopiaEngine struct {
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
	return &KopiaEngine{
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
// removes polling latency when backup, check, or forget Job status changes.
func (ke *KopiaEngine) RegisterWatchers(ctx context.Context, enqueue func(namespace, name string)) {
	ke.jobController.OnChange(ctx, "kopia-backup-job-watcher", func(_ string, job *batchv1.Job) (*batchv1.Job, error) {
		if job == nil {
			return nil, nil
		}
		if job.Labels[kopiautil.LabelKopiaJob] != kopiautil.LabelValueTrue {
			return job, nil
		}
		if name := joblifecycle.ParentName(job.ObjectMeta, vmBackupKind); name != "" {
			enqueue(job.Annotations[joblifecycle.AnnotationParentNamespace], name)
			return job, nil
		}

		return job, nil
	})
}

func (ke *KopiaEngine) Reconcile(
	vmb *harvesterv1.VirtualMachineBackup,
	volIndex int,
	_ map[string]snapshotv1.VolumeSnapshotClass,
) error {
	vb := ke.vmbo.GetVolBackup(vmb, volIndex)
	if vb == nil {
		return fmt.Errorf("volume backup at index %d not found", volIndex)
	}
	if ke.vmbo.GetVolBackupReadyToUse(vb) {
		// Backup is persisted in S3; the temporary clone PVC and VolumeSnapshot
		// are no longer needed. Cleanup is idempotent, so returning an error keeps
		// the VMBackup non-ready and lets the controller retry it.
		return ke.cleanupTemporaryResources(vmb, vb)
	}
	if ke.isDiscoveredBackup(vmb) {
		_, err := ke.checkRemoteSnapshot(vmb, vb)
		return err
	}
	return ke.reconcileBackupJob(vmb, vb)
}

func (ke *KopiaEngine) Prepare(
	vmb *harvesterv1.VirtualMachineBackup,
	vsClassMap map[string]snapshotv1.VolumeSnapshotClass,
) (bool, error) {
	if ke.isDiscoveredBackup(vmb) {
		return true, nil
	}
	return ke.vsHelper.PrepareSnapshots(vmb, vsClassMap)
}

func (ke *KopiaEngine) reconcileBackupJob(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
) error {
	jobName, err := ke.jobName(vmb, vb)
	if err != nil {
		return err
	}
	// Prefer the existing Job result. A failed or running Job must not be
	// overridden by a previously cached remote snapshot check.
	job, err := joblifecycle.GetJob(ke.jobClient, vmb, vmBackupKind, jobName)
	if apierrors.IsNotFound(err) {
		return ke.startBackupJob(vmb, vb, jobName)
	}
	if err != nil {
		return err
	}
	if err := ke.ensureSecret(vmb, job); err != nil {
		return err
	}
	return ke.syncFromJob(vb, job)
}

func (ke *KopiaEngine) startBackupJob(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	jobName string,
) error {
	if ready, err := ke.checkRemoteSnapshot(vmb, vb); ready || err != nil {
		return err
	}

	clonePVCName := ke.clonePVCName(vmb, vb)
	if err := ke.ensureClonePVC(vmb, vb, clonePVCName); err != nil {
		return err
	}
	if err := ke.createBackupJob(vmb, vb, clonePVCName, jobName); err != nil {
		return err
	}
	return engine.ErrRetryLater
}

// refreshProgress reads Pod logs without failing reconciliation on progress errors.
func (ke *KopiaEngine) refreshProgress(vb *harvesterv1.VolumeBackup, namespace, jobName string) int {
	current := ke.vmbo.GetVolBackupProgress(vb)
	pct, err := jobprogress.FetchProgressPercent(context.Background(), ke.clientset, namespace, jobName)
	if err != nil {
		return current
	}
	if err := ke.vmbo.SetVolBackupProgress(vb, pct); err != nil {
		return current
	}
	return pct
}

func (ke *KopiaEngine) UpdateProgress(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) (int64, error) {
	if ke.vmbo.GetVolBackupReadyToUse(vb) {
		return 100, nil
	}
	jobName, nameErr := ke.jobName(vmb, vb)
	if nameErr != nil {
		return int64(ke.vmbo.GetVolBackupProgress(vb)), nil
	}
	pct, err := jobprogress.FetchProgressPercent(context.Background(), ke.clientset, joblifecycle.Namespace, jobName)
	if err != nil {
		return int64(ke.vmbo.GetVolBackupProgress(vb)), nil
	}
	if err := ke.vmbo.SetVolBackupProgress(vb, pct); err != nil {
		return 0, err
	}
	return int64(pct), nil
}

// Remove records completed or skipped remote cleanup before deleting Jobs.
func (ke *KopiaEngine) Remove(
	vmb *harvesterv1.VirtualMachineBackup,
	removeRemote bool,
) (*harvesterv1.VirtualMachineBackup, error) {
	if err := ke.Cleanup(vmb); err != nil {
		return nil, err
	}

	if err := ke.forgetVolumes(vmb, removeRemote); err != nil {
		return nil, err
	}
	updated, err := common.RecordCleanup(ke.vmbo, vmb, removeRemote)
	if err != nil {
		return nil, err
	}
	return updated, ke.deleteJobs(updated)
}

func (ke *KopiaEngine) forgetVolumes(vmb *harvesterv1.VirtualMachineBackup, removeRemote bool) error {
	if !removeRemote || common.CleanupRecorded(ke.vmbo, vmb) {
		return nil
	}
	for i, vb := range ke.vmbo.GetVolBackups(vmb) {
		if ke.vmbo.GetVolBackupName(&vb) == nil {
			continue
		}
		if err := ke.forgetVolume(vmb, i); err != nil {
			return fmt.Errorf("failed to delete volume backup %s: %w", *ke.vmbo.GetVolBackupName(&vb), err)
		}
	}
	return nil
}

func (ke *KopiaEngine) forgetVolume(vmb *harvesterv1.VirtualMachineBackup, volIndex int) error {
	vb := ke.vmbo.GetVolBackup(vmb, volIndex)
	if vb == nil {
		return nil
	}
	job, err := ke.ensureForgetJob(vmb, vb)
	if err != nil {
		return err
	}
	if err := joblifecycle.ValidateJob(vmb, vmBackupKind, job); err != nil {
		return err
	}
	if err := ke.ensureSecret(vmb, job); err != nil {
		return err
	}
	return ke.syncFromForgetJob(job)
}

func (ke *KopiaEngine) deleteJobs(vmb *harvesterv1.VirtualMachineBackup) error {
	for _, vb := range ke.vmbo.GetVolBackups(vmb) {
		if ke.vmbo.GetVolBackupName(&vb) == nil {
			continue
		}
		name, err := ke.forgetJobName(vmb, &vb)
		if err != nil {
			return err
		}
		if err := ke.deleteForgetJob(vmb, name); err != nil {
			return err
		}
	}
	return nil
}

func (ke *KopiaEngine) deleteForgetJob(vmb *harvesterv1.VirtualMachineBackup, name string) error {
	return joblifecycle.DeleteJob(ke.jobClient, vmb, vmBackupKind, name)
}

func (ke *KopiaEngine) ensureForgetJob(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
) (*batchv1.Job, error) {
	forgetName, err := ke.forgetJobName(vmb, vb)
	if err != nil {
		return nil, err
	}
	namespace := joblifecycle.Namespace
	job, err := ke.jobClient.Get(namespace, forgetName, metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		return job, err
	}
	if err := ke.createForgetJob(vmb, vb, forgetName); err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create kopia forget job %s/%s: %w", namespace, forgetName, err)
	}
	return nil, engine.ErrRetryLater
}

func (ke *KopiaEngine) syncFromForgetJob(job *batchv1.Job) error {
	if job.Status.Succeeded > 0 {
		return nil
	}
	if job.DeletionTimestamp != nil {
		return ke.retryForgetJob(job)
	}
	for _, condition := range job.Status.Conditions {
		if condition.Type != batchv1.JobFailed || condition.Status != corev1.ConditionTrue {
			continue
		}
		return ke.retryForgetJob(job)
	}
	return engine.ErrRetryLater
}

func (ke *KopiaEngine) retryForgetJob(job *batchv1.Job) error {
	if err := joblifecycle.Delete(ke.jobClient, job); err != nil {
		return err
	}
	return fmt.Errorf("kopia forget job %s/%s failed or was interrupted; retrying cleanup", job.Namespace, job.Name)
}

func (ke *KopiaEngine) ensureClonePVC(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup, name string) error {
	if err := ke.imports.Import(vmb, common.SnapshotName(vmb, vb), ke.importedSnapshot(vmb, vb)); err != nil {
		return err
	}
	pvc, err := ke.pvcCache.Get(joblifecycle.Namespace, name)
	if apierrors.IsNotFound(err) {
		return ke.createClonePVC(vmb, vb, name)
	}
	if err != nil {
		return err
	}
	if err := joblifecycle.Validate(vmb, vmBackupKind, pvc.ObjectMeta); err != nil {
		return err
	}
	if err := datamover.SnapshotSource(pvc, ke.importedSnapshot(vmb, vb)); err != nil {
		return err
	}
	if pvc.DeletionTimestamp != nil || (ke.waitsForImmediateBinding(pvc) && pvc.Status.Phase != corev1.ClaimBound) {
		return engine.ErrRetryLater
	}
	return nil
}

func (ke *KopiaEngine) createClonePVC(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	clonePVCName string,
) error {
	namespace := joblifecycle.Namespace
	sourceSpec := ke.vmbo.GetVolBackupPVCSpec(vb)
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
		Name:     ke.importedSnapshot(vmb, vb),
	}
	pvc.Spec.DataSourceRef = nil
	joblifecycle.Track(&pvc.ObjectMeta, vmb, vmBackupKind)
	_, err := ke.pvcClient.Create(pvc)
	return err
}

func (ke *KopiaEngine) waitsForImmediateBinding(pvc *corev1.PersistentVolumeClaim) bool {
	scName := ptr.Deref(pvc.Spec.StorageClassName, "")
	if scName == "" {
		return true
	}
	sc, err := ke.scCache.Get(scName)
	if err != nil || sc.VolumeBindingMode == nil {
		return true
	}
	return *sc.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer
}

func (ke *KopiaEngine) createBackupJob(
	vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup,
	clonePVCName, jobName string,
) error {
	runtime, err := ke.jobRuntime(vmb, jobName)
	if err != nil {
		return err
	}
	if ke.vmbo.GetVolBackupName(vb) == nil {
		return fmt.Errorf("volume backup name is nil")
	}
	command := kopiautil.BackupCommand(
		runtime.Command, kopiautil.ConnectCommand(),
		acceleratedio.ReadCommand("/dev/"+clonePVCName),
		ke.vmbo.GetVolBackupPVName(vb), ke.snapshotTags(vmb, vb),
	)
	return ke.startJob(vmb, newBackupJob(runtime, command, clonePVCName, jobName))
}

func newBackupJob(runtime *kopiautil.JobRuntime, command, pvcName, name string) *batchv1.Job {
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

func (ke *KopiaEngine) checkRemoteSnapshot(
	vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup,
) (bool, error) {
	name, err := ke.checkJobName(vmb, vb)
	if err != nil {
		return false, err
	}
	result, err := kopiautil.CheckSnapshotJob(
		context.Background(), ke.clientset, name,
		func(name string) (*batchv1.Job, error) {
			return ke.getCheckJob(vmb, name)
		},
		func(name string) error {
			return ke.createCheckJob(vmb, vb, name)
		},
	)
	if err != nil {
		return false, err
	}
	return ke.checkResult(vmb, vb, name, result)
}

func (ke *KopiaEngine) getCheckJob(vmb *harvesterv1.VirtualMachineBackup, name string) (*batchv1.Job, error) {
	job, err := joblifecycle.GetJob(ke.jobClient, vmb, vmBackupKind, name)
	if err != nil {
		return nil, err
	}
	return job, ke.ensureSecret(vmb, job)
}

func (ke *KopiaEngine) checkResult(
	vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup,
	name string, result kopiautil.SnapshotCheckResult,
) (bool, error) {
	// New backups can proceed to upload when the remote snapshot is absent or
	// its check failed. Discovered backups have no local source to upload.
	if !ke.isDiscoveredBackup(vmb) &&
		(result == kopiautil.SnapshotCheckMissing || result == kopiautil.SnapshotCheckFailed) {
		return false, nil
	}
	switch result {
	case kopiautil.SnapshotCheckPending:
		return false, engine.ErrRetryLater
	case kopiautil.SnapshotCheckMissing:
		return false, fmt.Errorf("kopia snapshot for discovered backup %s/%s not found", joblifecycle.Namespace, name)
	case kopiautil.SnapshotCheckFailed:
		return false, fmt.Errorf("kopia snapshot check job %s/%s failed", joblifecycle.Namespace, name)
	case kopiautil.SnapshotCheckFound:
		return ke.markReady(vb)
	default:
		return false, fmt.Errorf("unknown kopia snapshot check result %q", result)
	}
}

func (ke *KopiaEngine) isDiscoveredBackup(vmb *harvesterv1.VirtualMachineBackup) bool {
	return ke.vmbo.GetSourceUID(vmb) == nil
}

func (ke *KopiaEngine) markReady(vb *harvesterv1.VolumeBackup) (bool, error) {
	now := metav1.Now()
	ready := true
	if err := ke.vmbo.SetVolBackupReadyToUse(vb, &ready); err != nil {
		return false, err
	}
	if err := ke.vmbo.SetVolBackupCreationTime(vb, &now); err != nil {
		return false, err
	}
	if err := ke.vmbo.SetVolBackupProgress(vb, 100); err != nil {
		return false, err
	}
	return true, nil
}

func (ke *KopiaEngine) createCheckJob(
	vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup, jobName string,
) error {
	runtime, err := ke.jobRuntime(vmb, jobName)
	if err != nil {
		return err
	}
	job := kopiautil.NewSnapshotCheckJob(kopiautil.SnapshotCheckJobOptions{
		Name:           jobName,
		Namespace:      joblifecycle.Namespace,
		Labels:         runtime.Labels,
		Runtime:        runtime,
		Tags:           ke.snapshotTags(vmb, vb),
		ConnectCommand: kopiautil.ConnectCommand(),
	})
	return ke.startJob(vmb, job)
}

func (ke *KopiaEngine) createForgetJob(
	vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup, jobName string,
) error {
	runtime, err := ke.jobRuntime(vmb, jobName)
	if err != nil {
		return err
	}
	command := kopiautil.SnapshotDeleteCommand(kopiautil.ConnectCommand(), ke.snapshotTags(vmb, vb))
	return ke.startJob(vmb, newForgetJob(runtime, command, jobName))
}

func newForgetJob(runtime *kopiautil.JobRuntime, command, name string) *batchv1.Job {
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
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
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

func (ke *KopiaEngine) jobRuntime(
	vmb *harvesterv1.VirtualMachineBackup, name string,
) (*kopiautil.JobRuntime, error) {
	repository, err := ke.repository(vmb)
	if err != nil {
		return nil, err
	}
	runtime, hasCapacity, err := kopiautil.NewJobRuntime(
		context.Background(), ke.clientset, jobcredentials.Name(name), repository, kopiautil.JobLabels(nil),
	)
	if err != nil {
		return nil, err
	}
	if !hasCapacity {
		return nil, engine.ErrRetryLater
	}
	return runtime, nil
}

func (ke *KopiaEngine) startJob(vmb *harvesterv1.VirtualMachineBackup, job *batchv1.Job) error {
	data, err := ke.targets.ThirdPartyCredentials(
		ke.vmbo.GetBackupTarget(vmb), kopiautil.PasswordKey,
	)
	if err != nil {
		return err
	}
	joblifecycle.Track(&job.ObjectMeta, vmb, vmBackupKind)
	client := jobcredentials.Client{Jobs: ke.jobClient, Secrets: ke.secretClient}
	job, err = client.Create(job)
	if err != nil {
		return err
	}
	return client.Ensure(job, func() (map[string][]byte, error) {
		return data, nil
	})
}

func (ke *KopiaEngine) ensureSecret(vmb *harvesterv1.VirtualMachineBackup, job *batchv1.Job) error {
	client := jobcredentials.Client{Jobs: ke.jobClient, Secrets: ke.secretClient}
	return client.Ensure(job, func() (map[string][]byte, error) {
		return ke.targets.ThirdPartyCredentials(
			ke.vmbo.GetBackupTarget(vmb), kopiautil.PasswordKey,
		)
	})
}

func (ke *KopiaEngine) repository(vmb *harvesterv1.VirtualMachineBackup) (*kopiautil.Repository, error) {
	target, err := backuputil.TargetFromStatus(ke.vmbo.GetBackupTarget(vmb))
	if err != nil {
		return nil, err
	}
	return kopiautil.S3Repository(target)
}

func (ke *KopiaEngine) syncFromJob(vb *harvesterv1.VolumeBackup, job *batchv1.Job) error {
	if job.Status.Failed > 0 {
		return fmt.Errorf("kopia backup job %s/%s failed", job.Namespace, job.Name)
	}
	if job.Status.Succeeded == 0 {
		ke.refreshProgress(vb, job.Namespace, job.Name)
		return engine.ErrRetryLater
	}
	if _, err := ke.markReady(vb); err != nil {
		return err
	}
	// Cleanup is deferred to the next reconcile (when ReadyToUse is persisted)
	// to avoid a race where Update fails after we've deleted the VS/clone PVC.
	logrus.Debugf("kopia backup job %s/%s completed", job.Namespace, job.Name)
	return nil
}

// Cleanup removes temporary resources after upload or before backup deletion.
func (ke *KopiaEngine) Cleanup(vmb *harvesterv1.VirtualMachineBackup) error {
	for _, vb := range ke.vmbo.GetVolBackups(vmb) {
		if ke.vmbo.GetVolBackupName(&vb) == nil {
			continue
		}
		if err := ke.cleanupTemporaryResources(vmb, &vb); err != nil {
			return err
		}
	}
	return nil
}

// cleanupTemporaryResources stops consumers before removing the clone and
// imported snapshot. The original snapshot is deleted last, with its CSI
// protection finalizers intact. The same order applies on cancellation.
func (ke *KopiaEngine) cleanupTemporaryResources(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) error {
	if err := ke.stopVolumeJobs(vmb, vb); err != nil {
		return err
	}
	if err := ke.deleteClonePVC(vmb, vb); err != nil {
		return err
	}
	if err := ke.imports.Delete(vmb, ke.importedSnapshot(vmb, vb)); err != nil {
		return err
	}
	return ke.deleteSnapshot(vmb, common.SnapshotName(vmb, vb))
}

func (ke *KopiaEngine) stopVolumeJobs(
	vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup,
) error {
	for _, prefix := range []string{jobNamePrefix, checkJobNamePrefix} {
		name := joblifecycle.Name(prefix, vmb, ke.volumeSnapshotName(vb))
		if err := datamover.StopJob(ke.clientset, vmb, vmBackupKind, name); err != nil {
			return err
		}
	}
	return nil
}

func (ke *KopiaEngine) deleteSnapshot(vmb *harvesterv1.VirtualMachineBackup, name string) error {
	if name == "" {
		return nil
	}
	snapshot, err := ke.vsClient.Get(vmb.Namespace, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := joblifecycle.Validate(vmb, vmBackupKind, snapshot.ObjectMeta); err != nil {
		return err
	}
	err = ke.vsClient.Delete(vmb.Namespace, name, &metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &snapshot.UID}})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return engine.ErrRetryLater
}

func (ke *KopiaEngine) volumeSnapshotName(vb *harvesterv1.VolumeBackup) string {
	if name := ke.vmbo.GetVolBackupName(vb); name != nil {
		return *name
	}
	return ""
}

func (ke *KopiaEngine) clonePVCName(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) string {
	return joblifecycle.Name("kopia-clone", vmb, ke.volumeSnapshotName(vb))
}

func (ke *KopiaEngine) importedSnapshot(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) string {
	return joblifecycle.Name("kopia-snapshot", vmb, ke.volumeSnapshotName(vb))
}

func (ke *KopiaEngine) jobName(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) (string, error) {
	name := ke.vmbo.GetVolBackupName(vb)
	if name == nil {
		return "", fmt.Errorf("volume backup name is nil")
	}
	return joblifecycle.Name(jobNamePrefix, vmb, *name), nil
}

func (ke *KopiaEngine) forgetJobName(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) (string, error) {
	name := ke.vmbo.GetVolBackupName(vb)
	if name == nil {
		return "", fmt.Errorf("volume backup name is nil")
	}
	return joblifecycle.Name(forgetJobNamePrefix, vmb, *name), nil
}

func (ke *KopiaEngine) checkJobName(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) (string, error) {
	name := ke.vmbo.GetVolBackupName(vb)
	if name == nil {
		return "", fmt.Errorf("volume backup name is nil")
	}
	return joblifecycle.Name(checkJobNamePrefix, vmb, *name), nil
}

func (ke *KopiaEngine) snapshotTags(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) []string {
	vbName := ke.vmbo.GetVolBackupName(vb)
	if vbName == nil {
		return nil
	}
	return []string{
		kopiautil.NamespaceTag(ke.vmbo.GetNamespace(vmb)),
		kopiautil.VMBackupTag(ke.vmbo.GetName(vmb)),
		kopiautil.SnapshotTag(*vbName),
	}
}

func (ke *KopiaEngine) deleteClonePVC(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) error {
	name := ke.clonePVCName(vmb, vb)
	pvc, err := ke.clientset.CoreV1().PersistentVolumeClaims(joblifecycle.Namespace).Get(context.Background(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := joblifecycle.Validate(vmb, vmBackupKind, pvc.ObjectMeta); err != nil {
		return err
	}
	if err := datamover.SnapshotSource(pvc, ke.importedSnapshot(vmb, vb)); err != nil {
		return err
	}
	return datamover.DeleteClaim(ke.clientset, vmb, vmBackupKind, name)
}
