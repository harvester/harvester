package kopia

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/backup/engine"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	kopiautil "github.com/harvester/harvester/pkg/util/kopia"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
)

func TestSyncFromForgetJob(t *testing.T) {
	tests := []struct {
		name       string
		status     batchv1.JobStatus
		wantRetry  bool
		wantFailed bool
		wantDelete bool
	}{
		{name: "pending", wantRetry: true},
		{name: "running", status: batchv1.JobStatus{Active: 1}, wantRetry: true},
		{name: "failed pod still retrying", status: batchv1.JobStatus{Failed: 1}, wantRetry: true},
		{name: "succeeded", status: batchv1.JobStatus{Succeeded: 1}},
		{name: "succeeded after failed attempt", status: batchv1.JobStatus{Failed: 1, Succeeded: 1}},
		{
			name: "terminal failure", wantFailed: true, wantDelete: true,
			status: batchv1.JobStatus{
				Failed: 4,
				Conditions: []batchv1.JobCondition{{
					Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded",
				}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "forget", Namespace: "default", UID: "forget-uid"},
				Status:     tt.status,
			}
			clientset := fake.NewSimpleClientset(job)
			ke := &KopiaEngine{jobClient: fakeclients.JobClient(clientset.BatchV1().Jobs)}
			err := ke.syncFromForgetJob(job)
			if tt.wantRetry {
				require.ErrorIs(t, err, engine.ErrRetryLater)
			} else if tt.wantFailed {
				require.Error(t, err)
				require.NotErrorIs(t, err, engine.ErrRetryLater)
			} else {
				require.NoError(t, err)
			}
			_, err = clientset.BatchV1().Jobs(job.Namespace).Get(
				context.Background(), job.Name, metav1.GetOptions{},
			)
			if tt.wantDelete {
				require.True(t, apierrors.IsNotFound(err))
				actions := clientset.Actions()
				deleteAction, ok := actions[0].(clienttesting.DeleteAction)
				require.True(t, ok)
				require.Equal(t, job.UID, *deleteAction.GetDeleteOptions().Preconditions.UID)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestForgetJobDeletionFailureIsRetried(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "forget", Namespace: "default"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}},
		},
	}
	clientset := fake.NewSimpleClientset(job)
	deleteErr := errors.New("API unavailable")
	clientset.PrependReactor("delete", "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, deleteErr
	})
	ke := &KopiaEngine{jobClient: fakeclients.JobClient(clientset.BatchV1().Jobs)}
	require.ErrorIs(t, ke.syncFromForgetJob(job), deleteErr)
}

func TestReconcileRetriesTemporaryCleanup(t *testing.T) {
	ready := true
	vmb := &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "default"},
		Status: harvesterv1.VirtualMachineBackupStatus{
			VolumeBackups: []harvesterv1.VolumeBackup{{
				Name:       ptr.To("volume"),
				ReadyToUse: &ready,
				PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
					ObjectMeta: metav1.ObjectMeta{Name: "source"},
				},
			}},
		},
	}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: "kopia-backup-volume", Namespace: vmb.Namespace,
	}}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name: "backup-source-kopia-clone", Namespace: vmb.Namespace,
	}}
	vs := &snapshotv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{
		Name: "volume", Namespace: vmb.Namespace,
	}}
	clientset := fake.NewSimpleClientset(job, pvc, vs)
	deleteErr := errors.New("API unavailable")
	failPVCDelete := true
	clientset.PrependReactor("delete", "persistentvolumeclaims", func(clienttesting.Action) (bool, runtime.Object, error) {
		if !failPVCDelete {
			return false, nil, nil
		}
		failPVCDelete = false
		return true, nil, deleteErr
	})

	vmbo := backupcommon.NewVMBackupOperatorBuilder().Build()
	ke := &KopiaEngine{
		vmbo:      vmbo,
		jobClient: fakeclients.JobClient(clientset.BatchV1().Jobs),
		pvcClient: fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims),
		vsClient:  fakeclients.VolumeSnapshotClient(clientset.SnapshotV1().VolumeSnapshots),
	}

	reconcileErr := ke.Reconcile(vmb, 0, nil)
	require.ErrorIs(t, reconcileErr, deleteErr)
	_, err := clientset.BatchV1().Jobs(vmb.Namespace).Get(
		context.Background(), job.Name, metav1.GetOptions{},
	)
	require.True(t, apierrors.IsNotFound(err))
	_, err = clientset.SnapshotV1().VolumeSnapshots(vmb.Namespace).Get(
		context.Background(), vs.Name, metav1.GetOptions{},
	)
	require.True(t, apierrors.IsNotFound(err))

	require.NoError(t, ke.Reconcile(vmb, 0, nil))
	_, err = clientset.CoreV1().PersistentVolumeClaims(vmb.Namespace).Get(
		context.Background(), pvc.Name, metav1.GetOptions{},
	)
	require.True(t, apierrors.IsNotFound(err))
}

func TestForgetJobRetainsCompletion(t *testing.T) {
	t.Setenv(kopiautil.ImageEnvVar, "harvester:test")
	for _, setting := range []settings.Setting{
		settings.KopiaMaxConcurrentJobs,
		settings.KopiaCacheSize,
		settings.KopiaEphemeralRequest,
		settings.KopiaEphemeralLimit,
	} {
		old := setting.Get()
		t.Cleanup(func() { require.NoError(t, setting.Set(old)) })
		require.NoError(t, setting.Set(setting.GetDefault()))
	}

	target := &settings.BackupTarget{
		Type:         settings.S3BackupType,
		Endpoint:     "http://s3.test:9000",
		BucketName:   "test",
		BucketRegion: "test-region",
	}
	vmb := &harvesterv1.VirtualMachineBackup{
		TypeMeta:   metav1.TypeMeta{Kind: "VirtualMachineBackup"},
		ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "default", UID: "backup-uid"},
		Status: harvesterv1.VirtualMachineBackupStatus{
			VolumeBackups: []harvesterv1.VolumeBackup{{Name: ptr.To("volume")}},
			BackupTarget:  backuputil.BackupTargetStatus(target),
		},
	}
	credentials := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: util.BackupTargetSecretName, Namespace: util.LonghornSystemNamespaceName,
		},
		Data: map[string][]byte{
			util.AWSAccessKey: []byte("access"),
			util.AWSSecretKey: []byte("secret"),
		},
	}
	clientset := fake.NewSimpleClientset(credentials)
	vmbo := backupcommon.NewVMBackupOperatorBuilder().Build()
	ke := &KopiaEngine{
		vmbo:         vmbo,
		vsHelper:     backupcommon.NewVolumeSnapshotHelper(nil, nil, nil, nil, vmbo, nil, nil),
		secretCache:  fakeclients.SecretCache(clientset.CoreV1().Secrets),
		secretClient: fakeclients.SecretClient(clientset.CoreV1().Secrets),
		jobClient:    fakeclients.JobClient(clientset.BatchV1().Jobs),
		jobCache:     fakeclients.JobCache(clientset.BatchV1().Jobs),
		clientset:    kubefake.NewSimpleClientset(),
	}

	jobName, err := ke.forgetJobName(&vmb.Status.VolumeBackups[0])
	require.NoError(t, err)
	require.ErrorIs(t, ke.ForceDelete(vmb, 0), engine.ErrRetryLater)
	job, err := clientset.BatchV1().Jobs(vmb.Namespace).Get(
		context.Background(), jobName, metav1.GetOptions{},
	)
	require.NoError(t, err)
	require.Equal(t, int32(3), *job.Spec.BackoffLimit)
	require.Nil(t, job.Spec.TTLSecondsAfterFinished)
	require.Equal(t, vmb.UID, job.OwnerReferences[0].UID)
	require.NotContains(t, job.Labels, "harvesterhci.io/vm-backup-name")
	require.NotContains(t, job.Labels, "harvesterhci.io/vm-backup-namespace")

	job.Status.Succeeded = 1
	job.Status.Failed = 1
	_, err = clientset.BatchV1().Jobs(vmb.Namespace).UpdateStatus(
		context.Background(), job, metav1.UpdateOptions{},
	)
	require.NoError(t, err)
	clientset.ClearActions()
	require.NoError(t, ke.ForceDelete(vmb, 0))
	for _, action := range clientset.Actions() {
		require.NotEqual(t, "create", action.GetVerb(), "completed volume cleanup must not be recreated")
		require.NotEqual(t, "delete", action.GetVerb(), "completion checkpoint must survive until VMBackup deletion")
	}
}

func TestEnsureKopiaSecretUsesDeterministicPassword(t *testing.T) {
	source := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: util.BackupTargetSecretName, Namespace: util.LonghornSystemNamespaceName,
		},
		Data: map[string][]byte{
			util.AWSAccessKey: []byte("access"),
			util.AWSSecretKey: []byte("iam-secret"),
		},
	}
	clientset := fake.NewSimpleClientset(source)
	vmbo := backupcommon.NewVMBackupOperatorBuilder().Build()
	ke := &KopiaEngine{
		vmbo:         vmbo,
		vsHelper:     backupcommon.NewVolumeSnapshotHelper(nil, nil, nil, nil, vmbo, nil, nil),
		secretCache:  fakeclients.SecretCache(clientset.CoreV1().Secrets),
		secretClient: fakeclients.SecretClient(clientset.CoreV1().Secrets),
	}
	vmb := &harvesterv1.VirtualMachineBackup{
		TypeMeta: metav1.TypeMeta{Kind: "VirtualMachineBackup"},
		ObjectMeta: metav1.ObjectMeta{
			Name: strings.Repeat("backup", 20), Namespace: "default", UID: "backup-uid",
		},
		Status: harvesterv1.VirtualMachineBackupStatus{
			BackupTarget: backuputil.BackupTargetStatus(&settings.BackupTarget{
				Type: settings.S3BackupType, Endpoint: "https://s3.example.com", BucketName: "backups",
			}),
		},
	}
	password, err := backuputil.RepositoryPassword(
		vmb.Status.BackupTarget.RepositoryID,
		harvesterv1.Kopia,
	)
	require.NoError(t, err)

	name, err := ke.ensureKopiaSecret(vmb)
	require.NoError(t, err)
	require.LessOrEqual(t, len(name), 63)
	credentials, err := clientset.CoreV1().Secrets(vmb.Namespace).Get(
		context.Background(), name, metav1.GetOptions{},
	)
	require.NoError(t, err)
	require.Equal(t, []byte("iam-secret"), credentials.Data[util.AWSSecretKey])
	require.Equal(t, []byte(password), credentials.Data[kopiautil.PasswordKey])
}

func TestGeneratedNamesAreBounded(t *testing.T) {
	longName := strings.Repeat("backup", 20)
	vmb := &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{Name: longName, Namespace: "default"},
	}
	vb := &harvesterv1.VolumeBackup{
		Name: ptr.To(longName + "-volume-a"),
		PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
			ObjectMeta: metav1.ObjectMeta{Name: longName, Namespace: "default"},
		},
	}
	ke := &KopiaEngine{vmbo: backupcommon.NewVMBackupOperatorBuilder().Build()}

	jobName, err := ke.jobName(vb)
	require.NoError(t, err)
	forgetName, err := ke.forgetJobName(vb)
	require.NoError(t, err)
	checkName, err := ke.checkJobName(vb)
	require.NoError(t, err)
	locatorName, _, ok := ke.jobLocator(vb)
	require.True(t, ok)

	for _, generatedName := range []string{
		ke.credentialsName(vmb),
		ke.clonePVCName(vmb, vb),
		jobName,
		forgetName,
		checkName,
		locatorName,
	} {
		require.LessOrEqual(t, len(generatedName), 63)
	}

	other := vb.DeepCopy()
	other.Name = ptr.To(longName + "-volume-b")
	otherJobName, err := ke.jobName(other)
	require.NoError(t, err)
	require.NotEqual(t, jobName, otherJobName)
}
