package kopia

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
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
	secrets, jobs := client.CoreV1().Secrets(joblifecycle.Namespace), client.BatchV1().Jobs(joblifecycle.Namespace)
	for _, name := range []string{checkName, restoreName} {
		job, err := jobs.Get(context.Background(), name, metav1.GetOptions{})
		require.NoError(t, err)
		require.False(t, *job.Spec.Suspend)
		require.False(t, *job.Spec.Template.Spec.AutomountServiceAccountToken)
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
		require.Len(t, refs, 3)
		for _, ref := range refs {
			require.Equal(t, secretName, ref)
		}
		secret, err := secrets.Get(context.Background(), secretName, metav1.GetOptions{})
		require.NoError(t, err)
		require.True(t, metav1.IsControlledBy(secret, job))
		require.Len(t, secret.Data, 3)
		require.NoError(t, secrets.Delete(context.Background(), secretName, metav1.DeleteOptions{}))
	}
	require.ErrorIs(t, ke.checkRemoteSnapshot(vmr, vmb, vr, vb), engine.ErrRetryLater)
	// Allow normal restore reconciliation to proceed past the snapshot check.
	check, err := jobs.Get(context.Background(), checkName, metav1.GetOptions{})
	require.NoError(t, err)
	check.Status.Succeeded = 1
	_, err = jobs.UpdateStatus(context.Background(), check, metav1.UpdateOptions{})
	require.NoError(t, err)
	_, err = client.CoreV1().PersistentVolumeClaims(joblifecycle.Namespace).Create(
		context.Background(), &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "restored-volume", Namespace: joblifecycle.Namespace},
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
		secretClient: fakeclients.SecretClient(client.CoreV1().Secrets),
		clientset:    fakeclients.KubernetesClient(client),
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
	old := settings.BackupTargetSet.Get()
	t.Cleanup(func() { require.NoError(t, settings.BackupTargetSet.Set(old)) })
	require.NoError(t, settings.BackupTargetSet.Set(`{"type":"s3","endpoint":"https://s3.example.com","bucketName":"backups"}`))
	source.Data[backuputil.RepositoryIDKey] = []byte(vmb.Status.BackupTarget.RepositoryID)
	source.Data["KOPIA_PASSWORD"] = []byte("stored-password")
	source.UID, source.ResourceVersion = "source-uid", "1"
	target, err := backuputil.TargetFromStatus(vmb.Status.BackupTarget)
	require.NoError(t, err)
	target.ThirdPartyBackupEnabled = true
	value, err := json.Marshal(target)
	require.NoError(t, err)
	record, err := backuputil.RepoReadyRecord(target, source)
	require.NoError(t, err)
	setting := &harvesterv1.Setting{ObjectMeta: metav1.ObjectMeta{
		Name:        settings.BackupTargetSettingName,
		Annotations: map[string]string{backuputil.ReadyAnnotation: record},
	}, Value: string(value)}
	harvesterv1.SettingConfigured.True(setting)
	_, err = client.HarvesterhciV1beta1().Settings().Create(context.Background(), setting, metav1.CreateOptions{})
	require.NoError(t, err)
	ke.targets = backuputil.TargetClient{Settings: fakeclients.HarvesterSettingClient(client.HarvesterhciV1beta1().Settings), Secrets: ke.secretClient}
	_, updateErr := client.CoreV1().Secrets(source.Namespace).Update(context.Background(), source, metav1.UpdateOptions{})
	require.NoError(t, updateErr)

	_, err = client.HarvesterhciV1beta1().VirtualMachineRestores(vmr.Namespace).Create(
		context.Background(), vmr, metav1.CreateOptions{},
	)
	require.NoError(t, err)
	client.ClearActions()
	return ke, vmb, vmr, client
}

func TestCredentialFailureKeepsJobSuspended(t *testing.T) {
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
	require.ErrorIs(t, err, apiErr)
	require.NotErrorIs(t, err, engine.ErrRetryLater)
	job, err := ke.jobClient.Get(joblifecycle.Namespace, name, metav1.GetOptions{})
	require.NoError(t, err)
	require.True(t, *job.Spec.Suspend)
	fail = false
	// A subsequent engine reconcile can repair the Secret and resume the suspended Job.
	require.ErrorIs(t, ke.checkRemoteSnapshot(vmr, vmb, vr, vb), engine.ErrRetryLater)
	job, err = ke.jobClient.Get(joblifecycle.Namespace, name, metav1.GetOptions{})
	require.NoError(t, err)
	require.False(t, *job.Spec.Suspend)
}

func TestDeferredBindingAllowsJobCreation(t *testing.T) {
	for _, mode := range []storagev1.VolumeBindingMode{storagev1.VolumeBindingImmediate, storagev1.VolumeBindingWaitForFirstConsumer} {
		t.Run(string(mode), func(t *testing.T) {
			ke, vmb, vmr, client := credentialFixture(t)
			vb, vr := &vmb.Status.VolumeBackups[0], &vmr.Status.VolumeRestores[0]
			ke.pvcClient = fakeclients.PersistentVolumeClaimClient(client.CoreV1().PersistentVolumeClaims)
			require.ErrorIs(t, ke.ensurePVC(vmr, vr, vb), engine.ErrRetryLater)
			pvc, err := client.CoreV1().PersistentVolumeClaims(joblifecycle.Namespace).Get(context.Background(), ke.stagingName(vmr, vr), metav1.GetOptions{})
			require.NoError(t, err)
			sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "test-sc"}, VolumeBindingMode: &mode}
			_, createErr := client.StorageV1().StorageClasses().Create(context.Background(), sc, metav1.CreateOptions{})
			require.NoError(t, createErr)
			ke.scCache = fakeclients.StorageClassCache(client.StorageV1().StorageClasses)
			pvc.Spec.StorageClassName = &sc.Name
			pvc.Status.Phase = corev1.ClaimPending
			vb.PersistentVolumeClaim.Spec.StorageClassName = &sc.Name
			_, updateErr := client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Update(context.Background(), pvc, metav1.UpdateOptions{})
			require.NoError(t, updateErr)
			checkErr := ke.ensurePVC(vmr, vr, vb)
			if mode == storagev1.VolumeBindingWaitForFirstConsumer {
				require.NoError(t, checkErr)
			} else {
				require.ErrorIs(t, checkErr, engine.ErrRetryLater)
			}
		})
	}
}

func TestJobsUseValidatedCredentials(t *testing.T) {
	t.Setenv("HARVESTER_IMAGE", "harvester:test")
	for _, operation := range []string{"check", "restore"} {
		t.Run(operation, func(t *testing.T) {
			ke, vmb, vmr, client := credentialFixture(t)
			// The source becomes unavailable after validation and Job creation.
			jobCreated := false
			client.PrependReactor("create", "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
				jobCreated = true
				return false, nil, nil
			})
			client.PrependReactor("get", "secrets", func(action clienttesting.Action) (bool, runtime.Object, error) {
				if jobCreated && action.GetNamespace() == util.LonghornSystemNamespaceName &&
					action.(clienttesting.GetAction).GetName() == util.BackupTargetSecretName {
					return true, nil, errors.New("shared credentials unavailable")
				}
				return false, nil, nil
			})
			vb := &vmb.Status.VolumeBackups[0]
			var err error
			switch operation {
			case "check":
				err = ke.createCheckJob(vmr, vmb, vb, "captured")
			case "restore":
				err = ke.createRestoreJob(
					vmr, vmb, &vmr.Status.VolumeRestores[0], vb, "captured",
				)
			}
			require.NoError(t, err)
			job, err := ke.jobClient.Get(joblifecycle.Namespace, "captured", metav1.GetOptions{})
			require.NoError(t, err)
			require.False(t, *job.Spec.Suspend)
			secret, err := ke.secretClient.Get(job.Namespace, jobcredentials.Name(job.Name), metav1.GetOptions{})
			require.NoError(t, err)
			require.True(t, metav1.IsControlledBy(secret, job))
			require.Equal(t, map[string][]byte{
				util.AWSAccessKey: []byte("access"), util.AWSSecretKey: []byte("secret"),
				backuputil.KopiaPasswordKey: []byte("stored-password"),
			}, secret.Data)
		})
	}
}

func TestInvalidCredentialsCreateNoJobs(t *testing.T) {
	t.Setenv("HARVESTER_IMAGE", "harvester:test")
	for _, key := range []string{util.AWSAccessKey, util.AWSSecretKey, backuputil.RepositoryIDKey, "KOPIA_PASSWORD"} {
		t.Run(key, func(t *testing.T) {
			ke, vmb, vmr, client := credentialFixture(t)
			source, err := client.CoreV1().Secrets(util.LonghornSystemNamespaceName).Get(context.Background(), util.BackupTargetSecretName, metav1.GetOptions{})
			require.NoError(t, err)
			delete(source.Data, key)
			_, err = client.CoreV1().Secrets(source.Namespace).Update(context.Background(), source, metav1.UpdateOptions{})
			require.NoError(t, err)
			vb, vr := &vmb.Status.VolumeBackups[0], &vmr.Status.VolumeRestores[0]
			require.Error(t, ke.createCheckJob(vmr, vmb, vb, ke.checkJobName(vmr, vr)))
			require.Error(t, ke.createRestoreJob(vmr, vmb, vr, vb, ke.jobName(vmr, vr)))
			jobs, err := client.BatchV1().Jobs(joblifecycle.Namespace).List(context.Background(), metav1.ListOptions{})
			require.NoError(t, err)
			require.Empty(t, jobs.Items)
		})
	}
}
