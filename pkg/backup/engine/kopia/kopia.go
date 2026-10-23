package kopia

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
	vmbo          common.VMBackupOperator
	vsHelper      *common.VolumeSnapshotHelper
	vsClient      ctlsnapshotv1.VolumeSnapshotClient
	pvcCache      ctlcorev1.PersistentVolumeClaimCache
	pvcClient     ctlcorev1.PersistentVolumeClaimClient
	scCache       ctlstoragev1.StorageClassCache
	secretCache   ctlcorev1.SecretCache
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
	secretCache ctlcorev1.SecretCache,
	secretClient ctlcorev1.SecretClient,
	scCache ctlstoragev1.StorageClassCache,
	jobController ctlbatchv1.JobController,
	clientset kubernetes.Interface,
) engine.BackupEngine {
	return &KopiaEngine{
		vmbo:          vmbo,
		vsHelper:      common.NewVolumeSnapshotHelper(vsCache, vsClient, nil, nil, vmbo, pvcCache, scCache),
		vsClient:      vsClient,
		pvcCache:      pvcCache,
		pvcClient:     pvcClient,
		scCache:       scCache,
		secretCache:   secretCache,
		secretClient:  secretClient,
		jobController: jobController,
		jobCache:      jobController.Cache(),
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
	if ready, err := ke.checkRemoteSnapshot(vmb, vb); ready || err != nil {
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
	jobName, err := ke.jobName(vb)
	if err != nil {
		return err
	}
	job, err := ke.jobCache.Get(ke.vmbo.GetNamespace(vmb), jobName)
	if err == nil {
		return ke.syncFromJob(vb, job)
	}
	if !apierrors.IsNotFound(err) {
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

func (ke *KopiaEngine) UpdateProgress(vb *harvesterv1.VolumeBackup) (int64, error) {
	if ke.vmbo.GetVolBackupReadyToUse(vb) {
		return 100, nil
	}
	jobName, jobNamespace, ok := ke.jobLocator(vb)
	if !ok {
		return int64(ke.vmbo.GetVolBackupProgress(vb)), nil
	}
	pct, err := jobprogress.FetchProgressPercent(context.Background(), ke.clientset, jobNamespace, jobName)
	if err != nil {
		return int64(ke.vmbo.GetVolBackupProgress(vb)), nil
	}
	if err := ke.vmbo.SetVolBackupProgress(vb, pct); err != nil {
		return 0, err
	}
	return int64(pct), nil
}

func (ke *KopiaEngine) ForceDelete(vmb *harvesterv1.VirtualMachineBackup, volIndex int) error {
	vb := ke.vmbo.GetVolBackup(vmb, volIndex)
	if vb == nil {
		return nil
	}
	forgetName, err := ke.forgetJobName(vb)
	if err != nil {
		return err
	}
	namespace := ke.vmbo.GetNamespace(vmb)
	job, err := ke.jobCache.Get(namespace, forgetName)
	if err == nil {
		return ke.syncFromForgetJob(job)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	if err := ke.createForgetJob(vmb, vb, forgetName); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create kopia forget job %s/%s: %w", namespace, forgetName, err)
	}
	return engine.ErrRetryLater
}

func (ke *KopiaEngine) syncFromForgetJob(job *batchv1.Job) error {
	if job.Status.Succeeded > 0 {
		return nil
	}
	for _, condition := range job.Status.Conditions {
		if condition.Type != batchv1.JobFailed || condition.Status != corev1.ConditionTrue {
			continue
		}
		if err := ke.jobClient.Delete(job.Namespace, job.Name, &metav1.DeleteOptions{
			PropagationPolicy: ptr.To(metav1.DeletePropagationBackground),
			Preconditions:     &metav1.Preconditions{UID: &job.UID},
		}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		return fmt.Errorf("kopia forget job %s/%s failed; retrying cleanup", job.Namespace, job.Name)
	}
	return engine.ErrRetryLater
}

func (ke *KopiaEngine) ensureClonePVC(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup, clonePVCName string) error {
	namespace := ke.vmbo.GetNamespace(vmb)
	pvc, err := ke.pvcCache.Get(namespace, clonePVCName)
	if err == nil {
		if ke.waitsForImmediateBinding(pvc) && pvc.Status.Phase != corev1.ClaimBound {
			return engine.ErrRetryLater
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	sourceSpec := ke.vmbo.GetVolBackupPVCSpec(vb)
	pvc = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            clonePVCName,
			Namespace:       namespace,
			OwnerReferences: []metav1.OwnerReference{ke.vsHelper.BuildOwnerReference(vmb)},
		},
		Spec: sourceSpec,
	}
	pvc.Spec.VolumeName = ""
	pvc.Spec.DataSource = &corev1.TypedLocalObjectReference{
		APIGroup: ptr.To("snapshot.storage.k8s.io"),
		Kind:     "VolumeSnapshot",
		Name:     ke.volumeSnapshotName(vb),
	}
	pvc.Spec.DataSourceRef = nil
	_, err = ke.pvcClient.Create(pvc)
	if apierrors.IsAlreadyExists(err) {
		return engine.ErrRetryLater
	}
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

func (ke *KopiaEngine) createBackupJob(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup, clonePVCName, jobName string) error {
	repository, err := ke.repository(vmb)
	if err != nil {
		return err
	}
	secretName, err := ke.ensureKopiaSecret(vmb)
	if err != nil {
		return err
	}
	vbName := ke.vmbo.GetVolBackupName(vb)
	if vbName == nil {
		return fmt.Errorf("volume backup name is nil")
	}

	namespace := ke.vmbo.GetNamespace(vmb)
	pvName := ke.vmbo.GetVolBackupPVName(vb)
	labels := kopiautil.JobLabels(nil)
	runtime, hasCapacity, err := kopiautil.NewJobRuntime(context.Background(), ke.clientset, secretName, repository, labels)
	if err != nil {
		return err
	}
	if !hasCapacity {
		return engine.ErrRetryLater
	}
	tags := ke.snapshotTags(vmb, vb)
	command := fmt.Sprintf(
		"set -eo pipefail\n%s\n/usr/bin/harvester io-mode -device /dev/%s -mode=read | %s snapshot create /%s --stdin-file %s %s",
		kopiautil.ConnectOrCreateCommand(),
		clonePVCName,
		runtime.Command,
		pvName,
		pvName,
		kopiautil.SnapshotTagsFlags(tags),
	)
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
			OwnerReferences: []metav1.OwnerReference{ke.vsHelper.BuildOwnerReference(vmb)},
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
							DevicePath: "/dev/" + clonePVCName,
						}},
					}},
					Volumes: volumes,
				},
			},
		},
	}
	_, err = ke.jobClient.Create(job)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (ke *KopiaEngine) checkRemoteSnapshot(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) (bool, error) {
	checkName, err := ke.checkJobName(vb)
	if err != nil {
		return false, err
	}
	namespace := ke.vmbo.GetNamespace(vmb)
	result, err := kopiautil.CheckSnapshotJob(
		context.Background(),
		ke.clientset,
		checkName,
		func(name string) (*batchv1.Job, error) {
			return ke.jobCache.Get(namespace, name)
		},
		func(name string) error {
			return ke.createCheckJob(vmb, vb, name)
		},
	)
	if err != nil {
		return false, err
	}
	discoveredBackup := ke.isDiscoveredBackup(vmb)
	switch result {
	case kopiautil.SnapshotCheckPending:
		return false, engine.ErrRetryLater
	case kopiautil.SnapshotCheckMissing:
		if discoveredBackup {
			return false, fmt.Errorf("kopia snapshot for discovered backup %s/%s not found", namespace, checkName)
		}
		return false, nil
	case kopiautil.SnapshotCheckFailed:
		if discoveredBackup {
			return false, fmt.Errorf("kopia snapshot check job %s/%s failed", namespace, checkName)
		}
		return false, nil
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

func (ke *KopiaEngine) createCheckJob(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup, jobName string) error {
	repository, err := ke.repository(vmb)
	if err != nil {
		return err
	}
	secretName, err := ke.ensureKopiaSecret(vmb)
	if err != nil {
		return err
	}

	namespace := ke.vmbo.GetNamespace(vmb)
	labels := kopiautil.JobLabels(nil)
	runtime, hasCapacity, err := kopiautil.NewJobRuntime(context.Background(), ke.clientset, secretName, repository, labels)
	if err != nil {
		return err
	}
	if !hasCapacity {
		return engine.ErrRetryLater
	}
	job := kopiautil.NewSnapshotCheckJob(kopiautil.SnapshotCheckJobOptions{
		Name:            jobName,
		Namespace:       namespace,
		Labels:          labels,
		OwnerReferences: []metav1.OwnerReference{ke.vsHelper.BuildOwnerReference(vmb)},
		Runtime:         runtime,
		Tags:            ke.snapshotTags(vmb, vb),
		ConnectCommand:  kopiautil.ConnectCommand(),
	})
	_, err = ke.jobClient.Create(job)
	if apierrors.IsAlreadyExists(err) {
		return engine.ErrRetryLater
	}
	return err
}

func (ke *KopiaEngine) createForgetJob(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup, jobName string) error {
	repository, err := ke.repository(vmb)
	if err != nil {
		return err
	}
	secretName, err := ke.ensureKopiaSecret(vmb)
	if err != nil {
		return err
	}

	namespace := ke.vmbo.GetNamespace(vmb)
	labels := kopiautil.JobLabels(nil)
	runtime, hasCapacity, err := kopiautil.NewJobRuntime(context.Background(), ke.clientset, secretName, repository, labels)
	if err != nil {
		return err
	}
	if !hasCapacity {
		return engine.ErrRetryLater
	}
	command := kopiautil.SnapshotDeleteCommand(kopiautil.ConnectCommand(), ke.snapshotTags(vmb, vb))
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            jobName,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{ke.vsHelper.BuildOwnerReference(vmb)},
		},
		Spec: batchv1.JobSpec{
			// Keep successful per-volume Jobs until the owning VMBackup is
			// finalized; otherwise a slow next volume can outlive their TTL
			// and cause completed cleanup to be repeated indefinitely.
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
	_, err = ke.jobClient.Create(job)
	if apierrors.IsAlreadyExists(err) {
		return engine.ErrRetryLater
	}
	return err
}

func (ke *KopiaEngine) ensureKopiaSecret(vmb *harvesterv1.VirtualMachineBackup) (string, error) {
	source, err := ke.secretCache.Get(util.LonghornSystemNamespaceName, util.BackupTargetSecretName)
	if err != nil {
		return "", err
	}
	backupTarget := ke.vmbo.GetBackupTarget(vmb)
	if backupTarget == nil {
		return "", fmt.Errorf("VMBackup does not record a backup target")
	}
	password, err := backuputil.RepositoryPassword(backupTarget.RepositoryID, harvesterv1.Kopia)
	if err != nil {
		return "", err
	}

	namespace := ke.vmbo.GetNamespace(vmb)
	name := ke.credentialsName(vmb)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			OwnerReferences: []metav1.OwnerReference{ke.vsHelper.BuildOwnerReference(vmb)},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			util.AWSAccessKey:     source.Data[util.AWSAccessKey],
			util.AWSSecretKey:     source.Data[util.AWSSecretKey],
			kopiautil.PasswordKey: []byte(password),
		},
	}
	if _, err := ke.secretClient.Create(secret); err != nil && !apierrors.IsAlreadyExists(err) {
		return "", err
	}
	return name, nil
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
		// Job still running — update progress from io-mode pod logs (best effort).
		if pct, err := jobprogress.FetchProgressPercent(context.Background(), ke.clientset, job.Namespace, job.Name); err == nil {
			_ = ke.vmbo.SetVolBackupProgress(vb, pct)
		}
		return engine.ErrRetryLater
	}
	now := metav1.Now()
	ready := true
	if err := ke.vmbo.SetVolBackupReadyToUse(vb, &ready); err != nil {
		return err
	}
	if err := ke.vmbo.SetVolBackupCreationTime(vb, &now); err != nil {
		return err
	}
	if err := ke.vmbo.SetVolBackupProgress(vb, 100); err != nil {
		return err
	}
	// Cleanup is deferred to the next reconcile (when ReadyToUse is persisted)
	// to avoid a race where Update fails after we've deleted the VS/clone PVC.
	logrus.Debugf("kopia backup job %s/%s completed", job.Namespace, job.Name)
	return nil
}

// cleanupTemporaryResources removes the backup Job, cloned PVC, and
// VolumeSnapshot created for this backup. Called after ReadyToUse is
// persisted, so failure of any step is safe to retry on the next reconcile.
// Delete the Job first so its Pod releases the cloned PVC before PVC cleanup.
func (ke *KopiaEngine) cleanupTemporaryResources(
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
) error {
	namespace := ke.vmbo.GetNamespace(vmb)
	return errors.Join(
		ke.deleteBackupJob(namespace, vb),
		ke.deleteClonePVC(namespace, ke.clonePVCName(vmb, vb)),
		ke.deleteSnapshot(namespace, ke.volumeSnapshotName(vb)),
	)
}

func (ke *KopiaEngine) deleteBackupJob(namespace string, vb *harvesterv1.VolumeBackup) error {
	jobName, err := ke.jobName(vb)
	if err != nil {
		return err
	}

	err = ke.jobClient.Delete(namespace, jobName, &metav1.DeleteOptions{
		PropagationPolicy: ptr.To(metav1.DeletePropagationBackground),
	})

	if err == nil || apierrors.IsNotFound(err) {
		return nil
	}

	return fmt.Errorf(
		"failed to delete kopia backup job %s/%s: %w",
		namespace, jobName, err,
	)
}

func (ke *KopiaEngine) deleteClonePVC(namespace, name string) error {
	err := ke.pvcClient.Delete(namespace, name, &metav1.DeleteOptions{})
	if err == nil || apierrors.IsNotFound(err) {
		return nil
	}
	return fmt.Errorf(
		"failed to delete kopia temporary PVC %s/%s: %w",
		namespace, name, err,
	)
}

func (ke *KopiaEngine) deleteSnapshot(namespace, name string) error {
	if name == "" {
		return nil
	}
	err := ke.vsClient.Delete(namespace, name, &metav1.DeleteOptions{})
	if err == nil || apierrors.IsNotFound(err) {
		return nil
	}
	return fmt.Errorf(
		"failed to delete kopia temporary VolumeSnapshot %s/%s: %w",
		namespace, name, err,
	)
}

func (ke *KopiaEngine) volumeSnapshotName(vb *harvesterv1.VolumeBackup) string {
	if name := ke.vmbo.GetVolBackupName(vb); name != nil {
		return *name
	}
	return ""
}

func (ke *KopiaEngine) credentialsName(vmb *harvesterv1.VirtualMachineBackup) string {
	return wranglername.SafeConcatName("kopia-credentials", ke.vmbo.GetName(vmb))
}

func (ke *KopiaEngine) clonePVCName(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) string {
	return wranglername.SafeConcatName(
		ke.vmbo.GetName(vmb),
		ke.vmbo.GetVolBackupPVCName(vb),
		"kopia-clone",
	)
}

func (ke *KopiaEngine) jobName(vb *harvesterv1.VolumeBackup) (string, error) {
	name := ke.vmbo.GetVolBackupName(vb)
	if name == nil {
		return "", fmt.Errorf("volume backup name is nil")
	}
	return wranglername.SafeConcatName(jobNamePrefix, *name), nil
}

func (ke *KopiaEngine) forgetJobName(vb *harvesterv1.VolumeBackup) (string, error) {
	name := ke.vmbo.GetVolBackupName(vb)
	if name == nil {
		return "", fmt.Errorf("volume backup name is nil")
	}
	return wranglername.SafeConcatName(forgetJobNamePrefix, *name), nil
}

func (ke *KopiaEngine) checkJobName(vb *harvesterv1.VolumeBackup) (string, error) {
	name := ke.vmbo.GetVolBackupName(vb)
	if name == nil {
		return "", fmt.Errorf("volume backup name is nil")
	}
	return wranglername.SafeConcatName(checkJobNamePrefix, *name), nil
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

// jobLocator returns the name/namespace to look up the backup Job for vb,
// or false if the volume backup name isn't set yet. See restic.jobLocator.
func (ke *KopiaEngine) jobLocator(vb *harvesterv1.VolumeBackup) (name, namespace string, ok bool) {
	vbName := ke.vmbo.GetVolBackupName(vb)
	if vbName == nil {
		return "", "", false
	}
	ns := ke.vmbo.GetVolBackupPVCNameSpace(vb)
	if ns == "" {
		return "", "", false
	}
	return wranglername.SafeConcatName(jobNamePrefix, *vbName), ns, true
}
