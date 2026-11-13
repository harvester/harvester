package kopia

import (
	"context"
	"fmt"
	"strings"

	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	wranglername "github.com/rancher/wrangler/v3/pkg/name"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	"github.com/harvester/harvester/pkg/restore/engine"
	"github.com/harvester/harvester/pkg/restore/pvchelper"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
	"github.com/harvester/harvester/pkg/util/jobprogress"
	kopiautil "github.com/harvester/harvester/pkg/util/kopia"
)

const (
	jobNamePrefix      = "kopia-restore"
	checkJobNamePrefix = "kopia-restore-check"
	vmRestoreKind      = "VirtualMachineRestore"
)

type KopiaRestoreEngine struct {
	vmbo          backupcommon.VMBackupOperator
	vmro          restorecommon.VMRestoreOperator
	pvcCache      ctlcorev1.PersistentVolumeClaimCache
	pvcClient     ctlcorev1.PersistentVolumeClaimClient
	scCache       ctlstoragev1.StorageClassCache
	secretClient  ctlcorev1.SecretClient
	jobController ctlbatchv1.JobController
	jobClient     ctlbatchv1.JobClient
	vmrController ctlharvesterv1.VirtualMachineRestoreController
	clientset     kubernetes.Interface
}

func GetRestoreEngine(
	vmbo backupcommon.VMBackupOperator,
	vmro restorecommon.VMRestoreOperator,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	pvcClient ctlcorev1.PersistentVolumeClaimClient,
	scCache ctlstoragev1.StorageClassCache,
	secretClient ctlcorev1.SecretClient,
	jobController ctlbatchv1.JobController,
	vmrController ctlharvesterv1.VirtualMachineRestoreController,
	clientset kubernetes.Interface,
) engine.RestoreEngine {
	return &KopiaRestoreEngine{
		vmbo:          vmbo,
		vmro:          vmro,
		pvcCache:      pvcCache,
		pvcClient:     pvcClient,
		scCache:       scCache,
		secretClient:  secretClient,
		jobController: jobController,
		jobClient:     jobController,
		vmrController: vmrController,
		clientset:     clientset,
	}
}

// RegisterWatchers registers a Job event handler that maps Job changes back to
// its VMRestore and enqueues it.
func (ke *KopiaRestoreEngine) RegisterWatchers(ctx context.Context, enqueue func(namespace, name string)) {
	lifecycle := restorecommon.JobHandler{
		Reader: ke.vmro, Delete: ke.Delete,
	}
	ke.vmrController.OnChange(ctx, "kopia-restore-job-cleanup", lifecycle.OnRestoreChange)
	ke.jobController.OnChange(ctx, "kopia-restore-job-watcher", func(_ string, job *batchv1.Job) (*batchv1.Job, error) {
		if job == nil {
			return nil, nil
		}
		if job.Labels[kopiautil.LabelKopiaJob] != kopiautil.LabelValueTrue {
			return job, nil
		}
		if parent := joblifecycle.ParentName(job.ObjectMeta, vmRestoreKind); parent != "" {
			enqueue(job.Namespace, parent)
		}
		return job, nil
	})
}

func (ke *KopiaRestoreEngine) Reconcile(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	volIndex int,
) error {
	vr := ke.vmro.GetVolRestore(vmr, volIndex)
	if vr == nil {
		return fmt.Errorf("volume restore at index %d not found", volIndex)
	}
	// Persisted completion allows the Job TTL controller to remove the Job.
	if ke.vmro.GetVolRestoreProgress(vr) == 100 {
		return ke.releaseJob(vmr, ke.jobName(vmr, vr))
	}
	vb := ke.vmbo.GetVolBackup(vmb, volIndex)
	if vb == nil {
		return fmt.Errorf("volume backup at index %d not found", volIndex)
	}

	if err := ke.checkRemoteSnapshot(vmr, vmb, vr, vb); err != nil {
		return err
	}
	if err := ke.ensurePVC(vmr, vr, vb); err != nil {
		return err
	}

	job, err := ke.ensureRestoreJob(vmr, vmb, vr, vb)
	if err != nil {
		return err
	}
	return ke.syncFromJob(vr, job)
}

func (ke *KopiaRestoreEngine) ensureRestoreJob(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) (*batchv1.Job, error) {
	jobName := ke.jobName(vmr, vr)
	job, err := ke.getJob(vmr, jobName)
	if !apierrors.IsNotFound(err) {
		return job, err
	}
	if err := ke.createRestoreJob(vmr, vmb, vr, vb, jobName); err != nil {
		return nil, err
	}
	return nil, engine.ErrRetryLater
}

func (ke *KopiaRestoreEngine) UpdateProgress(vr *harvesterv1.VolumeRestore) (int64, error) {
	current := ke.vmro.GetVolRestoreProgress(vr)
	if current == 100 {
		return 100, nil
	}
	jobName, namespace, ok := ke.jobLocator(vr)
	if !ok {
		return int64(current), nil
	}
	pct, err := jobprogress.FetchRestoreProgress(context.Background(), ke.clientset, namespace, jobName)
	if err != nil {
		return int64(current), nil
	}
	return int64(pct), nil
}

func (ke *KopiaRestoreEngine) ensurePVC(
	vmr *harvesterv1.VirtualMachineRestore,
	vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) error {
	namespace := ke.vmro.GetNamespace(vmr)
	pvcName := ke.vmro.GetVolRestorePVCName(vr)
	pvc, err := ke.pvcCache.Get(namespace, pvcName)
	if err == nil {
		// WaitForFirstConsumer storage classes only bind once a pod (the restore Job)
		// mounts the PVC, so gating here would deadlock that path.
		if ke.waitsForImmediateBinding(pvc) && pvc.Status.Phase != corev1.ClaimBound {
			return engine.ErrRetryLater
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	pvcSpec := ke.vmbo.GetVolBackupPVCSpec(vb)
	annotations := pvchelper.BuildRestoreAnnotations(
		ke.vmbo.GetVolBackupPVCAnnotations(vb),
		ke.vmro.GetName(vmr),
		restorecommon.RestoreNameAnnotation,
	)
	labels := pvchelper.BuildRestoreLabels(ke.vmbo.GetVolBackupPVCLabels(vb))
	pvc = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pvcName,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      pvcSpec.AccessModes,
			Resources:        pvcSpec.Resources,
			StorageClassName: pvcSpec.StorageClassName,
			VolumeMode:       pvcSpec.VolumeMode,
		},
	}
	pvc.Spec.VolumeName = ""
	pvc.Spec.DataSource = nil
	pvc.Spec.DataSourceRef = nil

	_, err = ke.pvcClient.Create(pvc)
	if apierrors.IsAlreadyExists(err) {
		return engine.ErrRetryLater
	}
	return err
}

func (ke *KopiaRestoreEngine) waitsForImmediateBinding(pvc *corev1.PersistentVolumeClaim) bool {
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

func (ke *KopiaRestoreEngine) createRestoreJob(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
	jobName string,
) error {
	vbName := ke.vmbo.GetVolBackupName(vb)
	if vbName == nil {
		return fmt.Errorf("volume backup name is nil")
	}

	namespace := ke.vmro.GetNamespace(vmr)
	pvcName := ke.vmro.GetVolRestorePVCName(vr)
	pvName := ke.vmbo.GetVolBackupPVName(vb)
	runtime, err := ke.jobRuntime(vmb, namespace)
	if err != nil {
		return err
	}
	labels := runtime.Labels
	tags := ke.snapshotTags(vmb, vb)
	command := fmt.Sprintf(
		strings.Join([]string{
			"set -eo pipefail",
			// `kopia snapshot list --json` returns an array of snapshot manifests
			// sorted oldest-first; `.[-1].rootEntry.obj` gives the object ID of
			// the newest matching snapshot. jq is far less brittle than the prior
			// sed/grep pipeline that depended on the literal output formatting.
			"%s",
			"%s show \"$OBJ/%s\" | /usr/bin/harvester io-mode -device /dev/%s -mode=write",
		}, "\n"),
		kopiautil.SnapshotObjectCommand(kopiautil.ConnectCommand(), tags),
		runtime.Command,
		pvName,
		pvcName,
	)
	volumes := runtime.VolumesWith(corev1.Volume{
		Name: "volume",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
		},
	})
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:       jobName,
			Namespace:  namespace,
			Labels:     labels,
			Finalizers: []string{joblifecycle.CompletionFinalizer},
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
						Name:         "restore",
						Image:        runtime.Image,
						Env:          runtime.Env,
						Command:      runtime.Entrypoint,
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
	joblifecycle.Track(&job.ObjectMeta, vmr, vmRestoreKind)
	_, err = ke.jobClient.Create(job)
	if apierrors.IsAlreadyExists(err) {
		return engine.ErrRetryLater
	}
	if err != nil {
		return fmt.Errorf("creating job: %w: %w", engine.ErrRetryLater, err)
	}
	return nil
}

func (ke *KopiaRestoreEngine) checkRemoteSnapshot(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) error {
	checkName := ke.checkJobName(vmr, vr)
	namespace := ke.vmro.GetNamespace(vmr)
	result, err := kopiautil.CheckSnapshotJob(
		context.Background(),
		ke.clientset,
		checkName,
		func(name string) (*batchv1.Job, error) {
			return ke.getJob(vmr, name)
		},
		func(name string) error {
			return ke.createCheckJob(vmr, vmb, vb, name)
		},
	)
	if err != nil {
		return err
	}
	switch result {
	case kopiautil.SnapshotCheckPending:
		return engine.ErrRetryLater
	case kopiautil.SnapshotCheckMissing:
		return fmt.Errorf("kopia snapshot for restore job %s/%s not found", namespace, checkName)
	case kopiautil.SnapshotCheckFailed:
		return fmt.Errorf("kopia snapshot check job %s/%s failed", namespace, checkName)
	case kopiautil.SnapshotCheckFound:
		return nil
	default:
		return fmt.Errorf("unknown kopia snapshot check result %q", result)
	}
}

func (ke *KopiaRestoreEngine) createCheckJob(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	jobName string,
) error {
	namespace := ke.vmro.GetNamespace(vmr)
	runtime, err := ke.jobRuntime(vmb, namespace)
	if err != nil {
		return err
	}
	labels := runtime.Labels
	job := kopiautil.NewSnapshotCheckJob(kopiautil.SnapshotCheckJobOptions{
		Name:           jobName,
		Namespace:      namespace,
		Labels:         labels,
		Runtime:        runtime,
		Tags:           ke.snapshotTags(vmb, vb),
		ConnectCommand: kopiautil.ConnectCommand(),
	})
	joblifecycle.Track(&job.ObjectMeta, vmr, vmRestoreKind)
	_, err = ke.jobClient.Create(job)
	if apierrors.IsAlreadyExists(err) {
		return engine.ErrRetryLater
	}
	if err != nil {
		return fmt.Errorf("creating job: %w: %w", engine.ErrRetryLater, err)
	}
	return nil
}

func (ke *KopiaRestoreEngine) jobRuntime(
	vmb *harvesterv1.VirtualMachineBackup, namespace string,
) (*kopiautil.JobRuntime, error) {
	repository, err := ke.repository(vmb)
	if err != nil {
		return nil, err
	}
	err = backuputil.CheckCredentials(
		ke.secretClient, ke.vmbo.GetBackupTarget(vmb), kopiautil.PasswordKey,
	)
	if err != nil {
		return nil, fmt.Errorf("preparing job credentials: %w: %w", engine.ErrRetryLater, err)
	}
	runtime, capacity, err := kopiautil.NewJobRuntime(
		context.Background(), ke.clientset, ke.vmbo.GetRepositoryID(vmb),
		repository, kopiautil.JobLabels(nil),
	)
	if err != nil {
		return nil, err
	}
	if !capacity {
		return nil, engine.ErrRetryLater
	}
	if err := backuputil.EnsureJobAccess(context.Background(), ke.clientset, namespace); err != nil {
		return nil, fmt.Errorf("preparing backup Job access: %w: %w", engine.ErrRetryLater, err)
	}
	return runtime, nil
}

func (ke *KopiaRestoreEngine) repository(vmb *harvesterv1.VirtualMachineBackup) (*kopiautil.Repository, error) {
	target, err := backuputil.TargetFromStatus(ke.vmbo.GetBackupTarget(vmb))
	if err != nil {
		return nil, err
	}
	return kopiautil.S3Repository(target)
}

// syncFromJob marks success for the controller to persist before releasing the
// Job on the next reconcile. Failed Jobs stay protected until failure is persisted.
func (ke *KopiaRestoreEngine) syncFromJob(vr *harvesterv1.VolumeRestore, job *batchv1.Job) error {
	if job.Status.Failed > 0 {
		return fmt.Errorf("kopia restore job %s/%s failed", job.Namespace, job.Name)
	}
	if job.Status.Succeeded == 0 {
		return engine.ErrRetryLater
	}
	if err := ke.vmro.SetVolRestoreProgress(vr, 100); err != nil {
		return err
	}
	return engine.ErrRetryLater
}

func (ke *KopiaRestoreEngine) getJob(vmr *harvesterv1.VirtualMachineRestore, name string) (*batchv1.Job, error) {
	job, err := joblifecycle.GetJob(ke.jobClient, vmr, vmRestoreKind, name)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("getting job: %w: %w", engine.ErrRetryLater, err)
	}
	return job, err
}

func (ke *KopiaRestoreEngine) releaseJob(vmr *harvesterv1.VirtualMachineRestore, name string) error {
	if err := joblifecycle.ReleaseJob(ke.jobClient, vmr, vmRestoreKind, name); err != nil {
		return fmt.Errorf("releasing restore job: %w: %w", engine.ErrRetryLater, err)
	}
	return nil
}

func (ke *KopiaRestoreEngine) jobName(vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore) string {
	return wranglername.SafeConcatName(
		jobNamePrefix,
		ke.vmro.GetName(vmr),
		ke.vmro.GetVolRestoreVolumeName(vr),
	)
}

func (ke *KopiaRestoreEngine) checkJobName(vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore) string {
	return wranglername.SafeConcatName(
		checkJobNamePrefix,
		ke.vmro.GetName(vmr),
		ke.vmro.GetVolRestoreVolumeName(vr),
	)
}

func (ke *KopiaRestoreEngine) snapshotTags(vmb *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup) []string {
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

// jobLocator returns the name and namespace to look up the restore Job for vr.
// UpdateProgress is called with only a VolumeRestore handle but the job name
// embeds the VMRestore name; we recover that from the destination PVC's
// restore-name annotation (set in createPVC via BuildRestoreAnnotations).
func (ke *KopiaRestoreEngine) jobLocator(vr *harvesterv1.VolumeRestore) (name, namespace string, ok bool) {
	volName := ke.vmro.GetVolRestoreVolumeName(vr)
	ns := ke.vmro.GetVolRestorePVCNamespace(vr)
	if volName == "" || ns == "" {
		return "", "", false
	}
	pvc, err := ke.pvcCache.Get(ns, ke.vmro.GetVolRestorePVCName(vr))
	if err != nil {
		return "", "", false
	}
	restoreName := pvc.Annotations[restorecommon.RestoreNameAnnotation]
	if restoreName == "" {
		return "", "", false
	}
	return wranglername.SafeConcatName(jobNamePrefix, restoreName, volName), ns, true
}

// Delete releases and removes Jobs belonging to this VMRestore.
func (ke *KopiaRestoreEngine) Delete(vmr *harvesterv1.VirtualMachineRestore) error {
	if ke.vmro.GetType(vmr) != harvesterv1.Kopia {
		return nil
	}
	return joblifecycle.DeleteJobs(ke.jobClient, vmr, vmRestoreKind)
}
