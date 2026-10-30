package kopia

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	"github.com/harvester/harvester/pkg/restore/engine"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/util/jobcredentials"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

func TestJobsOwnCredentials(t *testing.T) {
	t.Setenv("HARVESTER_IMAGE", "harvester:test")
	ke, vmb, vmr, client := credentialFixture(t)
	vb, vr := &vmb.Status.VolumeBackups[0], &vmr.Status.VolumeRestores[0]
	checkName, restoreName := ke.checkJobName(vmr, vr), ke.jobName(vmr, vr)
	require.NoError(t, ke.createCheckJob(vmr, vmb, vb, checkName))
	require.NoError(t, ke.createRestoreJob(vmr, vmb, vr, vb, restoreName))
	secrets, jobs := client.CoreV1().Secrets(vmr.Namespace), client.BatchV1().Jobs(vmr.Namespace)
	for _, name := range []string{checkName, restoreName} {
		job, err := jobs.Get(context.Background(), name, metav1.GetOptions{})
		require.NoError(t, err)
		require.False(t, *job.Spec.Suspend)
		require.Empty(t, job.OwnerReferences)
		require.NoError(t, joblifecycle.ValidateJob(vmr, vmRestoreKind, job))
		require.Equal(t, int32(60), *job.Spec.TTLSecondsAfterFinished)
		if name == restoreName {
			require.Contains(t, job.Finalizers, joblifecycle.CompletionFinalizer)
		}
		secretName := jobcredentials.Name(name)
		var refs []string
		for _, env := range job.Spec.Template.Spec.Containers[0].Env {
			if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
				continue
			}
			refs = append(refs, env.ValueFrom.SecretKeyRef.Name)
		}
		require.Equal(t, []string{secretName, secretName, secretName}, refs)
		secret, err := secrets.Get(context.Background(), secretName, metav1.GetOptions{})
		require.NoError(t, err)
		require.True(t, metav1.IsControlledBy(secret, job))
		require.NoError(t, secrets.Delete(context.Background(), secretName, metav1.DeleteOptions{}))
	}
	require.ErrorIs(t, ke.checkRemoteSnapshot(vmr, vmb, vr, vb), engine.ErrRetryLater)
	// Allow normal restore reconciliation to proceed past the snapshot check.
	check, err := jobs.Get(context.Background(), checkName, metav1.GetOptions{})
	require.NoError(t, err)
	check.Status.Succeeded = 1
	_, err = jobs.UpdateStatus(context.Background(), check, metav1.UpdateOptions{})
	require.NoError(t, err)
	_, err = client.CoreV1().PersistentVolumeClaims(vmr.Namespace).Create(
		context.Background(), &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "restored-volume", Namespace: vmr.Namespace},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		}, metav1.CreateOptions{},
	)
	require.NoError(t, err)
	require.ErrorIs(t, ke.Reconcile(vmr, vmb, 0), engine.ErrRetryLater)
	list, err := secrets.List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, list.Items, 2)
}

func credentialFixture(t *testing.T) (
	*KopiaRestoreEngine, *harvesterv1.VirtualMachineBackup, *harvesterv1.VirtualMachineRestore, *fake.Clientset,
) {
	source := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: util.BackupTargetSecretName, Namespace: util.LonghornSystemNamespaceName},
		Data:       map[string][]byte{util.AWSAccessKey: []byte("access"), util.AWSSecretKey: []byte("secret")},
	}
	client := fake.NewSimpleClientset(source)
	client.PrependReactor("create", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		job := action.(clienttesting.CreateAction).GetObject().(*batchv1.Job)
		job.UID = uuid.NewUUID()
		return false, nil, nil
	})
	ke := &KopiaRestoreEngine{
		vmbo:         backupcommon.NewVMBackupOperatorBuilder().Build(),
		vmro:         restorecommon.NewVMRestoreOperatorBuilder().Build(),
		jobClient:    fakeclients.JobClient(client.BatchV1().Jobs),
		secretCache:  fakeclients.SecretCache(client.CoreV1().Secrets),
		secretClient: fakeclients.SecretClient(client.CoreV1().Secrets),
		pvcCache:     fakeclients.PersistentVolumeClaimCache(client.CoreV1().PersistentVolumeClaims),
		clientset:    kubefake.NewSimpleClientset(),
	}
	vmb := &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "default", UID: "backup-uid"},
		Status: harvesterv1.VirtualMachineBackupStatus{
			BackupTarget: backuputil.BackupTargetStatus(&settings.BackupTarget{
				Type: settings.S3BackupType, Endpoint: "https://s3.example.com", BucketName: "backups",
			}),
			VolumeBackups: []harvesterv1.VolumeBackup{{Name: ptr.To("volume")}},
		},
	}
	vmr := &harvesterv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "default", UID: "restore-uid"},
		Status: harvesterv1.VirtualMachineRestoreStatus{
			VolumeRestores: []harvesterv1.VolumeRestore{{
				VolumeName: "volume",
				PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
					ObjectMeta: metav1.ObjectMeta{Name: "restored-volume", Namespace: "default"},
				},
			}},
		},
	}
	_, err := client.HarvesterhciV1beta1().VirtualMachineRestores(vmr.Namespace).Create(
		context.Background(), vmr, metav1.CreateOptions{},
	)
	require.NoError(t, err)
	client.ClearActions()
	return ke, vmb, vmr, client
}

func TestCredentialFailureRetriesRestore(t *testing.T) {
	t.Setenv("HARVESTER_IMAGE", "harvester:test")
	ke, vmb, vmr, client := credentialFixture(t)
	apiErr := errors.New("API unavailable")
	fail := true
	client.PrependReactor("create", "secrets", func(clienttesting.Action) (bool, runtime.Object, error) {
		if fail {
			return true, nil, apiErr
		}
		return false, nil, nil
	})
	vb, vr := &vmb.Status.VolumeBackups[0], &vmr.Status.VolumeRestores[0]
	name := ke.checkJobName(vmr, vr)
	err := ke.createCheckJob(vmr, vmb, vb, name)
	require.ErrorIs(t, err, engine.ErrRetryLater)
	require.ErrorIs(t, err, apiErr)
	job, err := ke.jobClient.Get(vmr.Namespace, name, metav1.GetOptions{})
	require.NoError(t, err)
	require.True(t, *job.Spec.Suspend)
	fail = false
	// Existing suspended Jobs must recover instead of permanently failing the restore.
	require.ErrorIs(t, ke.checkRemoteSnapshot(vmr, vmb, vr, vb), engine.ErrRetryLater)
	job, err = ke.jobClient.Get(vmr.Namespace, name, metav1.GetOptions{})
	require.NoError(t, err)
	require.False(t, *job.Spec.Suspend)
}
