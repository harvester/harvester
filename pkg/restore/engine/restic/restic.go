package restic

import (
	"context"
	"fmt"

	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
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
	"github.com/harvester/harvester/pkg/util/datamover"
	"github.com/harvester/harvester/pkg/util/jobcredentials"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
	"github.com/harvester/harvester/pkg/util/jobprogress"
	resticutil "github.com/harvester/harvester/pkg/util/restic"
)

const (
	jobNamePrefix      = "restic-restore"
	checkJobNamePrefix = "restic-restore-check"
	vmRestoreKind      = "VirtualMachineRestore"
)

type ResticRestoreEngine struct {
	vmbo          backupcommon.VMBackupOperator
	vmro          restorecommon.VMRestoreOperator
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
	pvcClient ctlcorev1.PersistentVolumeClaimClient,
	scCache ctlstoragev1.StorageClassCache,
	secretClient ctlcorev1.SecretClient,
	jobController ctlbatchv1.JobController,
	vmrController ctlharvesterv1.VirtualMachineRestoreController,
	clientset kubernetes.Interface,
) engine.RestoreEngine {
	return &ResticRestoreEngine{
		vmbo:          vmbo,
		vmro:          vmro,
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
// the tracked VMRestore and enqueues it.
func (re *ResticRestoreEngine) RegisterWatchers(ctx context.Context, enqueue func(namespace, name string)) {
	lifecycle := restorecommon.JobHandler{
		Reader: re.vmro, Delete: re.Delete,
	}
	re.vmrController.OnChange(ctx, "restic-restore-job-cleanup", lifecycle.OnRestoreChange)
	re.jobController.OnChange(ctx, "restic-restore-job-watcher", func(_ string, job *batchv1.Job) (*batchv1.Job, error) {
		if job == nil {
			return nil, nil
		}
		if job.Labels[resticutil.LabelResticJob] != resticutil.LabelValueTrue {
			return job, nil
		}
		if parent := joblifecycle.ParentName(job.ObjectMeta, vmRestoreKind); parent != "" {
			enqueue(job.Annotations[joblifecycle.AnnotationParentNamespace], parent)
		}
		return job, nil
	})
}

func (re *ResticRestoreEngine) Reconcile(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	volIndex int,
) error {
	vr := re.vmro.GetVolRestore(vmr, volIndex)
	if vr == nil {
		return fmt.Errorf("volume restore at index %d not found", volIndex)
	}
	// Persisted completion allows the Job TTL controller to remove the Job.
	if re.vmro.GetVolRestoreProgress(vr) == 100 {
		return re.releaseJob(vmr, re.jobName(vmr, vr))
	}
	vb := re.vmbo.GetVolBackup(vmb, volIndex)
	if vb == nil {
		return fmt.Errorf("volume backup at index %d not found", volIndex)
	}
	return re.restoreVolume(vmr, vmb, vr, vb)
}

func (re *ResticRestoreEngine) restoreVolume(
	vmr *harvesterv1.VirtualMachineRestore, vmb *harvesterv1.VirtualMachineBackup,
	vr *harvesterv1.VolumeRestore, vb *harvesterv1.VolumeBackup,
) error {
	handled, err := re.resumeHandoff(vmr, vr, vb)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	job, err := re.transferJob(vmr, vmb, vr, vb)
	if err != nil {
		return err
	}
	if err := re.ensureSecret(vmb, job); err != nil {
		return err
	}
	return re.syncFromJob(vmr, vr, vb, job)
}

func (re *ResticRestoreEngine) resumeHandoff(
	vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) (bool, error) {
	volumes := datamover.Volumes{Client: re.clientset}
	found, ready, err := volumes.ResumeHandoff(
		vmr, re.stagingName(vmr, vr), re.destinationPVC(vmr, vr, vb),
	)
	if err != nil {
		return found, pending(err)
	}
	if found && !ready {
		return true, engine.ErrRetryLater
	}
	if ready {
		return true, re.vmro.SetVolRestoreProgress(vr, 100)
	}
	return false, nil
}

// transferJob reuses captured credentials and results before checking remote state.
func (re *ResticRestoreEngine) transferJob(
	vmr *harvesterv1.VirtualMachineRestore, vmb *harvesterv1.VirtualMachineBackup,
	vr *harvesterv1.VolumeRestore, vb *harvesterv1.VolumeBackup,
) (*batchv1.Job, error) {
	job, err := re.getJob(vmr, re.jobName(vmr, vr))
	if !apierrors.IsNotFound(err) {
		return job, err
	}
	if err := re.checkRemoteSnapshot(vmr, vmb, vr, vb); err != nil {
		return nil, err
	}
	if err := re.ensurePVC(vmr, vr, vb); err != nil {
		return nil, err
	}
	return re.ensureRestoreJob(vmr, vmb, vr, vb)
}

func (re *ResticRestoreEngine) ensureRestoreJob(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) (*batchv1.Job, error) {
	jobName := re.jobName(vmr, vr)
	job, err := re.getJob(vmr, jobName)
	if !apierrors.IsNotFound(err) {
		return job, err
	}
	if err := re.createRestoreJob(vmr, vmb, vr, vb, jobName); err != nil {
		return nil, err
	}
	return nil, engine.ErrRetryLater
}

func (re *ResticRestoreEngine) UpdateProgress(vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore) (int64, error) {
	current := re.vmro.GetVolRestoreProgress(vr)
	if current == 100 {
		return 100, nil
	}
	pct, err := jobprogress.FetchRestoreProgress(context.Background(), re.clientset, joblifecycle.Namespace, re.jobName(vmr, vr))
	if err != nil {
		return int64(current), nil
	}
	return int64(min(pct, 99)), nil
}

// waitsForImmediateBinding reports whether the PVC's StorageClass binds eagerly,
// in which case ClaimBound is reachable without a consumer pod. If the SC is
// missing or unreadable we conservatively assume Immediate (the K8s default).
func (re *ResticRestoreEngine) waitsForImmediateBinding(pvc *corev1.PersistentVolumeClaim) bool {
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

func (re *ResticRestoreEngine) ensurePVC(
	vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) error {
	desired := re.stagingPVC(vmr, vr, vb)
	pvc, err := re.getStage(desired)
	if err != nil {
		return err
	}
	if err := datamover.ValidateClaim(vmr, vmRestoreKind, pvc, desired); err != nil {
		return pending(err)
	}
	if pvc.Spec.DataSource != nil || pvc.Spec.DataSourceRef != nil {
		return fmt.Errorf("restore staging PVC has an unexpected source")
	}
	if re.waitsForImmediateBinding(pvc) && pvc.Status.Phase != corev1.ClaimBound {
		return engine.ErrRetryLater
	}
	return nil
}

func (re *ResticRestoreEngine) stagingPVC(
	vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) *corev1.PersistentVolumeClaim {
	desired := re.destinationPVC(vmr, vr, vb)
	desired.Namespace = joblifecycle.Namespace
	desired.Name = re.stagingName(vmr, vr)
	joblifecycle.Track(&desired.ObjectMeta, vmr, vmRestoreKind)
	return desired
}

func (re *ResticRestoreEngine) getStage(
	desired *corev1.PersistentVolumeClaim,
) (*corev1.PersistentVolumeClaim, error) {
	pvc, err := re.pvcClient.Get(desired.Namespace, desired.Name, metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		return pvc, err
	}
	if _, err := re.pvcClient.Create(desired); err != nil {
		return nil, pending(err)
	}
	return nil, engine.ErrRetryLater
}

func (re *ResticRestoreEngine) destinationPVC(
	vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) *corev1.PersistentVolumeClaim {
	source := re.vmbo.GetVolBackupPVCSpec(vb)
	annotations := pvchelper.BuildRestoreAnnotations(
		re.vmbo.GetVolBackupPVCAnnotations(vb), re.vmro.GetName(vmr), restorecommon.RestoreNameAnnotation,
	)
	// A new staging claim must be scheduled using its own consumer and topology.
	delete(annotations, "volume.kubernetes.io/selected-node")
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: re.vmro.GetVolRestorePVCName(vr), Namespace: re.vmro.GetNamespace(vmr),
			Annotations: annotations,
			Labels:      pvchelper.BuildRestoreLabels(re.vmbo.GetVolBackupPVCLabels(vb)),
		},
		Spec: corev1.PersistentVolumeClaimSpec{AccessModes: source.AccessModes, Resources: source.Resources,
			StorageClassName: source.StorageClassName, VolumeMode: source.VolumeMode},
	}
}

func (re *ResticRestoreEngine) stagingName(vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore) string {
	return joblifecycle.Name("restic-restore-volume", vmr, re.vmro.GetVolRestoreVolumeName(vr))
}

func (re *ResticRestoreEngine) createRestoreJob(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
	jobName string,
) error {
	runtime, err := re.jobRuntime(vmb, jobName)
	if err != nil {
		return err
	}
	vbName := re.vmbo.GetVolBackupName(vb)
	if vbName == nil {
		return fmt.Errorf("volume backup name is nil")
	}

	pvcName := re.stagingName(vmr, vr)
	pvName := re.vmbo.GetVolBackupPVName(vb)
	tags := []string{
		resticutil.NamespaceTag(re.vmbo.GetNamespace(vmb)),
		resticutil.VMBackupTag(re.vmbo.GetName(vmb)),
		resticutil.SnapshotTag(*vbName),
	}
	writer := fmt.Sprintf("/usr/bin/harvester io-mode -device /dev/%s -mode=write", pvcName)
	command := resticutil.RestoreCommand(
		runtime.Command, writer, pvName, tags,
	)
	job := newRestoreJob(runtime, command, pvcName, jobName)
	return re.startJob(vmr, vmb, job)
}

func newRestoreJob(runtime *resticutil.JobRuntime, command, pvcName, name string) *batchv1.Job {
	labels := runtime.Labels
	volumes := runtime.VolumesWith(corev1.Volume{
		Name: "volume",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
		},
	})
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  joblifecycle.Namespace,
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
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:         "restore",
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
	return job
}

func (re *ResticRestoreEngine) checkRemoteSnapshot(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) error {
	checkName := re.checkJobName(vmr, vr)
	namespace := joblifecycle.Namespace
	result, err := resticutil.CheckSnapshotJob(
		context.Background(),
		re.clientset,
		checkName,
		func(name string) (*batchv1.Job, error) {
			job, err := re.getJob(vmr, name)
			if err != nil {
				return nil, err
			}
			return job, re.ensureSecret(vmb, job)
		},
		func(name string) error {
			return re.createCheckJob(vmr, vmb, vb, name)
		},
	)
	if err != nil {
		return err
	}
	switch result {
	case resticutil.SnapshotCheckPending:
		return engine.ErrRetryLater
	case resticutil.SnapshotCheckMissing:
		return fmt.Errorf("restic snapshot for restore job %s/%s not found", namespace, checkName)
	case resticutil.SnapshotCheckFailed:
		return fmt.Errorf("restic snapshot check job %s/%s failed", namespace, checkName)
	case resticutil.SnapshotCheckFound:
		return nil
	default:
		return fmt.Errorf("unknown restic snapshot check result %q", result)
	}
}

func (re *ResticRestoreEngine) createCheckJob(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	vb *harvesterv1.VolumeBackup,
	jobName string,
) error {
	runtime, err := re.jobRuntime(vmb, jobName)
	if err != nil {
		return err
	}
	vbName := re.vmbo.GetVolBackupName(vb)
	if vbName == nil {
		return fmt.Errorf("volume backup name is nil")
	}
	job := resticutil.NewSnapshotCheckJob(resticutil.SnapshotCheckJobOptions{
		Name:      jobName,
		Namespace: joblifecycle.Namespace,
		Labels:    runtime.Labels,
		Runtime:   runtime,
		Tags:      re.snapshotTags(vmb, *vbName),
	})
	return re.startJob(vmr, vmb, job)
}

// jobRuntime validates shared credentials before a Job can be created.
func (re *ResticRestoreEngine) jobRuntime(
	vmb *harvesterv1.VirtualMachineBackup, name string,
) (*resticutil.JobRuntime, error) {
	if _, err := backuputil.CredentialData(
		re.secretClient, re.vmbo.GetBackupTarget(vmb), resticutil.PasswordKey,
	); err != nil {
		return nil, fmt.Errorf("reading repository credentials: %w: %w", engine.ErrRetryLater, err)
	}
	repository, err := re.repository(vmb)
	if err != nil {
		return nil, err
	}
	runtime, hasCapacity, err := resticutil.NewJobRuntime(
		context.Background(), re.clientset,
		jobcredentials.Name(name), repository, resticutil.JobLabels(nil),
	)
	if err != nil {
		return nil, err
	}
	if !hasCapacity {
		return nil, engine.ErrRetryLater
	}
	return runtime, nil
}

func (re *ResticRestoreEngine) startJob(
	vmr *harvesterv1.VirtualMachineRestore, vmb *harvesterv1.VirtualMachineBackup, job *batchv1.Job,
) error {
	joblifecycle.Track(&job.ObjectMeta, vmr, vmRestoreKind)
	client := jobcredentials.Client{Jobs: re.jobClient, Secrets: re.secretClient}
	job, err := client.Create(job)
	if apierrors.IsAlreadyExists(err) {
		return engine.ErrRetryLater
	}
	if err != nil {
		return fmt.Errorf("creating job: %w: %w", engine.ErrRetryLater, err)
	}
	return re.ensureSecret(vmb, job)
}

func (re *ResticRestoreEngine) snapshotTags(
	vmb *harvesterv1.VirtualMachineBackup, name string,
) []string {
	return []string{
		resticutil.NamespaceTag(re.vmbo.GetNamespace(vmb)),
		resticutil.VMBackupTag(re.vmbo.GetName(vmb)),
		resticutil.SnapshotTag(name),
	}
}

func (re *ResticRestoreEngine) ensureSecret(vmb *harvesterv1.VirtualMachineBackup, job *batchv1.Job) error {
	client := jobcredentials.Client{Jobs: re.jobClient, Secrets: re.secretClient}
	err := client.Ensure(job, func() (map[string][]byte, error) {
		return backuputil.CredentialData(
			re.secretClient, re.vmbo.GetBackupTarget(vmb), resticutil.PasswordKey,
		)
	})
	if err != nil {
		return fmt.Errorf("preparing job credentials: %w: %w", engine.ErrRetryLater, err)
	}
	return nil
}

func (re *ResticRestoreEngine) repository(vmb *harvesterv1.VirtualMachineBackup) (*resticutil.RepositoryConfig, error) {
	target, err := backuputil.TargetFromStatus(re.vmbo.GetBackupTarget(vmb))
	if err != nil {
		return nil, err
	}
	return resticutil.Repository(target)
}

// syncFromJob marks successful work for the controller to persist before release.
// Failed Jobs remain protected until the controller persists the failure.
func (re *ResticRestoreEngine) syncFromJob(
	vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup, job *batchv1.Job,
) error {
	if job.Status.Failed > 0 {
		return fmt.Errorf("restic restore job %s/%s failed", job.Namespace, job.Name)
	}
	if job.Status.Succeeded == 0 {
		return engine.ErrRetryLater
	}
	volumes := datamover.Volumes{Client: re.clientset}
	err := volumes.BeginHandoff(
		vmr, re.stagingName(vmr, vr), job.Name, re.destinationPVC(vmr, vr, vb),
	)
	return pending(err)
}

func (re *ResticRestoreEngine) getJob(vmr *harvesterv1.VirtualMachineRestore, name string) (*batchv1.Job, error) {
	job, err := joblifecycle.GetJob(re.jobClient, vmr, vmRestoreKind, name)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("getting job: %w: %w", engine.ErrRetryLater, err)
	}
	return job, err
}

func (re *ResticRestoreEngine) releaseJob(vmr *harvesterv1.VirtualMachineRestore, name string) error {
	if err := joblifecycle.ReleaseJob(re.jobClient, vmr, vmRestoreKind, name); err != nil {
		return fmt.Errorf("releasing restore job: %w: %w", engine.ErrRetryLater, err)
	}
	return nil
}

func (re *ResticRestoreEngine) jobName(vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore) string {
	return joblifecycle.Name(jobNamePrefix, vmr, re.vmro.GetVolRestoreVolumeName(vr))
}

func (re *ResticRestoreEngine) checkJobName(vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore) string {
	return joblifecycle.Name(checkJobNamePrefix, vmr, re.vmro.GetVolRestoreVolumeName(vr))
}

// Delete releases and removes independent Jobs when the restore ends or is deleted.
func (re *ResticRestoreEngine) Delete(vmr *harvesterv1.VirtualMachineRestore) error {
	if re.vmro.GetType(vmr) != harvesterv1.Restic {
		return nil
	}
	if err := datamover.StopJobs(re.clientset, vmr, vmRestoreKind); err != nil {
		return pending(err)
	}
	return pending((datamover.Volumes{Client: re.clientset}).Cleanup(vmr))
}

func pending(err error) error {
	if datamover.Retryable(err) {
		return engine.ErrRetryLater
	}
	return err
}
