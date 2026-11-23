package kopia

import (
	"context"
	"fmt"
	"strings"

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
	return &KopiaRestoreEngine{
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
			enqueue(job.Annotations[joblifecycle.AnnotationParentNamespace], parent)
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
	return ke.restoreVolume(vmr, vmb, vr, vb)
}

func (ke *KopiaRestoreEngine) restoreVolume(
	vmr *harvesterv1.VirtualMachineRestore, vmb *harvesterv1.VirtualMachineBackup,
	vr *harvesterv1.VolumeRestore, vb *harvesterv1.VolumeBackup,
) error {
	handled, err := ke.resumeHandoff(vmr, vr, vb)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	job, err := ke.transferJob(vmr, vmb, vr, vb)
	if err != nil {
		return err
	}
	if err := ke.ensureSecret(vmb, job); err != nil {
		return err
	}
	return ke.syncFromJob(vmr, vr, vb, job)
}

func (ke *KopiaRestoreEngine) resumeHandoff(
	vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) (bool, error) {
	volumes := datamover.Volumes{Client: ke.clientset}
	found, ready, err := volumes.ResumeHandoff(
		vmr, ke.stagingName(vmr, vr), ke.destinationPVC(vmr, vr, vb),
	)
	if err != nil {
		return found, pending(err)
	}
	if found && !ready {
		return true, engine.ErrRetryLater
	}
	if ready {
		return true, ke.vmro.SetVolRestoreProgress(vr, 100)
	}
	return false, nil
}

// transferJob reuses captured credentials and results before checking remote state.
func (ke *KopiaRestoreEngine) transferJob(
	vmr *harvesterv1.VirtualMachineRestore, vmb *harvesterv1.VirtualMachineBackup,
	vr *harvesterv1.VolumeRestore, vb *harvesterv1.VolumeBackup,
) (*batchv1.Job, error) {
	job, err := ke.getJob(vmr, ke.jobName(vmr, vr))
	if !apierrors.IsNotFound(err) {
		return job, err
	}
	if err := ke.checkRemoteSnapshot(vmr, vmb, vr, vb); err != nil {
		return nil, err
	}
	if err := ke.ensurePVC(vmr, vr, vb); err != nil {
		return nil, err
	}
	return ke.ensureRestoreJob(vmr, vmb, vr, vb)
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

func (ke *KopiaRestoreEngine) UpdateProgress(vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore) (int64, error) {
	current := ke.vmro.GetVolRestoreProgress(vr)
	if current == 100 {
		return 100, nil
	}
	pct, err := jobprogress.FetchRestoreProgress(context.Background(), ke.clientset, joblifecycle.Namespace, ke.jobName(vmr, vr))
	if err != nil {
		return int64(current), nil
	}
	return int64(min(pct, 99)), nil
}

func (ke *KopiaRestoreEngine) ensurePVC(
	vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) error {
	desired := ke.stagingPVC(vmr, vr, vb)
	pvc, err := ke.getStage(desired)
	if err != nil {
		return err
	}
	if err := datamover.ValidateClaim(vmr, vmRestoreKind, pvc, desired); err != nil {
		return pending(err)
	}
	if pvc.Spec.DataSource != nil || pvc.Spec.DataSourceRef != nil {
		return fmt.Errorf("restore staging PVC has an unexpected source")
	}
	if ke.waitsForImmediateBinding(pvc) && pvc.Status.Phase != corev1.ClaimBound {
		return engine.ErrRetryLater
	}
	return nil
}

func (ke *KopiaRestoreEngine) stagingPVC(
	vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) *corev1.PersistentVolumeClaim {
	desired := ke.destinationPVC(vmr, vr, vb)
	desired.Namespace = joblifecycle.Namespace
	desired.Name = ke.stagingName(vmr, vr)
	joblifecycle.Track(&desired.ObjectMeta, vmr, vmRestoreKind)
	return desired
}

func (ke *KopiaRestoreEngine) getStage(
	desired *corev1.PersistentVolumeClaim,
) (*corev1.PersistentVolumeClaim, error) {
	pvc, err := ke.pvcClient.Get(desired.Namespace, desired.Name, metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		return pvc, err
	}
	if _, err := ke.pvcClient.Create(desired); err != nil {
		return nil, pending(err)
	}
	return nil, engine.ErrRetryLater
}

func (ke *KopiaRestoreEngine) destinationPVC(
	vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) *corev1.PersistentVolumeClaim {
	source := ke.vmbo.GetVolBackupPVCSpec(vb)
	annotations := pvchelper.BuildRestoreAnnotations(
		ke.vmbo.GetVolBackupPVCAnnotations(vb), ke.vmro.GetName(vmr), restorecommon.RestoreNameAnnotation,
	)
	// A new staging claim must be scheduled using its own consumer and topology.
	delete(annotations, "volume.kubernetes.io/selected-node")
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: ke.vmro.GetVolRestorePVCName(vr), Namespace: ke.vmro.GetNamespace(vmr),
			Annotations: annotations,
			Labels:      pvchelper.BuildRestoreLabels(ke.vmbo.GetVolBackupPVCLabels(vb)),
		},
		Spec: corev1.PersistentVolumeClaimSpec{AccessModes: source.AccessModes, Resources: source.Resources,
			StorageClassName: source.StorageClassName, VolumeMode: source.VolumeMode},
	}
}

func (ke *KopiaRestoreEngine) stagingName(vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore) string {
	return joblifecycle.Name("kopia-restore-volume", vmr, ke.vmro.GetVolRestoreVolumeName(vr))
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
	runtime, err := ke.jobRuntime(vmb, jobName)
	if err != nil {
		return err
	}
	vbName := ke.vmbo.GetVolBackupName(vb)
	if vbName == nil {
		return fmt.Errorf("volume backup name is nil")
	}

	pvcName := ke.stagingName(vmr, vr)
	pvName := ke.vmbo.GetVolBackupPVName(vb)
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

	job := newRestoreJob(runtime, command, pvcName, jobName)
	return ke.startJob(vmr, vmb, job)
}

func newRestoreJob(runtime *kopiautil.JobRuntime, command, pvcName, name string) *batchv1.Job {
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

func (ke *KopiaRestoreEngine) checkRemoteSnapshot(
	vmr *harvesterv1.VirtualMachineRestore,
	vmb *harvesterv1.VirtualMachineBackup,
	vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup,
) error {
	checkName := ke.checkJobName(vmr, vr)
	namespace := joblifecycle.Namespace
	result, err := kopiautil.CheckSnapshotJob(
		context.Background(),
		ke.clientset,
		checkName,
		func(name string) (*batchv1.Job, error) {
			job, err := ke.getJob(vmr, name)
			if err != nil {
				return nil, err
			}
			return job, ke.ensureSecret(vmb, job)
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
	return ke.startJob(vmr, vmb, job)
}

// jobRuntime validates shared credentials before a Job can be created.
func (ke *KopiaRestoreEngine) jobRuntime(
	vmb *harvesterv1.VirtualMachineBackup, name string,
) (*kopiautil.JobRuntime, error) {
	if _, err := backuputil.CredentialData(
		ke.secretClient, ke.vmbo.GetBackupTarget(vmb), kopiautil.PasswordKey,
	); err != nil {
		return nil, fmt.Errorf("reading repository credentials: %w: %w", engine.ErrRetryLater, err)
	}
	repository, err := ke.repository(vmb)
	if err != nil {
		return nil, err
	}
	runtime, hasCapacity, err := kopiautil.NewJobRuntime(
		context.Background(), ke.clientset,
		jobcredentials.Name(name), repository, kopiautil.JobLabels(nil),
	)
	if err != nil {
		return nil, err
	}
	if !hasCapacity {
		return nil, engine.ErrRetryLater
	}
	return runtime, nil
}

func (ke *KopiaRestoreEngine) startJob(
	vmr *harvesterv1.VirtualMachineRestore, vmb *harvesterv1.VirtualMachineBackup, job *batchv1.Job,
) error {
	joblifecycle.Track(&job.ObjectMeta, vmr, vmRestoreKind)
	client := jobcredentials.Client{Jobs: ke.jobClient, Secrets: ke.secretClient}
	job, err := client.Create(job)
	if apierrors.IsAlreadyExists(err) {
		return engine.ErrRetryLater
	}
	if err != nil {
		return fmt.Errorf("creating job: %w: %w", engine.ErrRetryLater, err)
	}
	return ke.ensureSecret(vmb, job)
}

func (ke *KopiaRestoreEngine) ensureSecret(vmb *harvesterv1.VirtualMachineBackup, job *batchv1.Job) error {
	client := jobcredentials.Client{Jobs: ke.jobClient, Secrets: ke.secretClient}
	err := client.Ensure(job, func() (map[string][]byte, error) {
		return backuputil.CredentialData(
			ke.secretClient, ke.vmbo.GetBackupTarget(vmb), kopiautil.PasswordKey,
		)
	})
	if err != nil {
		return fmt.Errorf("preparing job credentials: %w: %w", engine.ErrRetryLater, err)
	}
	return nil
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
func (ke *KopiaRestoreEngine) syncFromJob(
	vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore,
	vb *harvesterv1.VolumeBackup, job *batchv1.Job,
) error {
	if job.Status.Failed > 0 {
		return fmt.Errorf("kopia restore job %s/%s failed", job.Namespace, job.Name)
	}
	if job.Status.Succeeded == 0 {
		return engine.ErrRetryLater
	}
	volumes := datamover.Volumes{Client: ke.clientset}
	err := volumes.BeginHandoff(
		vmr, ke.stagingName(vmr, vr), job.Name, ke.destinationPVC(vmr, vr, vb),
	)
	return pending(err)
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
	return joblifecycle.Name(jobNamePrefix, vmr, ke.vmro.GetVolRestoreVolumeName(vr))
}

func (ke *KopiaRestoreEngine) checkJobName(vmr *harvesterv1.VirtualMachineRestore, vr *harvesterv1.VolumeRestore) string {
	return joblifecycle.Name(checkJobNamePrefix, vmr, ke.vmro.GetVolRestoreVolumeName(vr))
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

// Delete releases and removes Jobs belonging to this VMRestore.
func (ke *KopiaRestoreEngine) Delete(vmr *harvesterv1.VirtualMachineRestore) error {
	if ke.vmro.GetType(vmr) != harvesterv1.Kopia {
		return nil
	}
	if err := datamover.StopJobs(ke.clientset, vmr, vmRestoreKind); err != nil {
		return pending(err)
	}
	return pending((datamover.Volumes{Client: ke.clientset}).Cleanup(vmr))
}

func pending(err error) error {
	if datamover.Retryable(err) {
		return engine.ErrRetryLater
	}
	return err
}
