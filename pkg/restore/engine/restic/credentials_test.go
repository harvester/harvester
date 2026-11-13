package restic

import (
	"context"
	"encoding/json"
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
	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

func TestJobsHideCredentials(t *testing.T) {
	t.Setenv("HARVESTER_IMAGE", "harvester:test")
	re, vmb, vmr, client := credentialFixture(t)
	vb, vr := &vmb.Status.VolumeBackups[0], &vmr.Status.VolumeRestores[0]
	checkName, restoreName := re.checkJobName(vmr, vr), re.jobName(vmr, vr)
	secrets, jobs := client.CoreV1().Secrets(util.LonghornSystemNamespaceName), client.BatchV1().Jobs(vmr.Namespace)
	for _, suffix := range []string{"", "-rotated"} {
		if suffix != "" {
			source, err := secrets.Get(context.Background(), util.BackupTargetSecretName, metav1.GetOptions{})
			require.NoError(t, err)
			source.Data[util.AWSAccessKey] = []byte("access-rotated")
			source.Data[util.AWSSecretKey] = []byte("secret-rotated")
			_, err = secrets.Update(context.Background(), source, metav1.UpdateOptions{})
			require.NoError(t, err)
		}
		require.NoError(t, re.createCheckJob(vmr, vmb, vb, checkName+suffix))
		require.NoError(t, re.createRestoreJob(vmr, vmb, vr, vb, restoreName+suffix))
	}
	for _, suffix := range []string{"", "-rotated"} {
		for _, name := range []string{checkName, restoreName} {
			job, err := jobs.Get(context.Background(), name+suffix, metav1.GetOptions{})
			require.NoError(t, err)
			require.False(t, ptr.Deref(job.Spec.Suspend, false))
			require.Empty(t, job.OwnerReferences)
			require.NoError(t, joblifecycle.ValidateJob(vmr, vmRestoreKind, job))
			require.Equal(t, int32(60), *job.Spec.TTLSecondsAfterFinished)
			if name == restoreName {
				require.Contains(t, job.Finalizers, joblifecycle.CompletionFinalizer)
			}
			assertJobCredentials(t, job, vmb.Status.BackupTarget.RepositoryID)
		}
	}
	list, err := client.CoreV1().Secrets(vmr.Namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, list.Items)
	for _, action := range client.Actions() {
		require.False(t, action.Matches("create", "secrets"))
	}
}

func TestExistingRestoreDoesNotReadCredentials(t *testing.T) {
	t.Setenv("HARVESTER_IMAGE", "harvester:test")
	re, vmb, vmr, client := credentialFixture(t)
	vb, vr := &vmb.Status.VolumeBackups[0], &vmr.Status.VolumeRestores[0]
	checkName, restoreName := re.checkJobName(vmr, vr), re.jobName(vmr, vr)
	require.NoError(t, re.createCheckJob(vmr, vmb, vb, checkName))
	require.NoError(t, re.createRestoreJob(vmr, vmb, vr, vb, restoreName))
	require.NoError(t, client.CoreV1().Secrets(util.LonghornSystemNamespaceName).Delete(
		context.Background(), util.BackupTargetSecretName, metav1.DeleteOptions{},
	))
	require.ErrorIs(t, re.checkRemoteSnapshot(vmr, vmb, vr, vb), engine.ErrRetryLater)
	jobs := client.BatchV1().Jobs(vmr.Namespace)
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
	require.ErrorIs(t, re.Reconcile(vmr, vmb, 0), engine.ErrRetryLater)
	restore, err := jobs.Get(context.Background(), restoreName, metav1.GetOptions{})
	require.NoError(t, err)
	restore.Status.Succeeded = 1
	_, err = jobs.UpdateStatus(context.Background(), restore, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.ErrorIs(t, re.Reconcile(vmr, vmb, 0), engine.ErrRetryLater)
	require.EqualValues(t, 100, re.vmro.GetVolRestoreProgress(vr))
	require.NoError(t, re.Reconcile(vmr, vmb, 0))
}

func credentialFixture(t *testing.T) (
	*ResticRestoreEngine, *harvesterv1.VirtualMachineBackup, *harvesterv1.VirtualMachineRestore, *fake.Clientset,
) {
	t.Helper()
	oldTarget := settings.BackupTargetSet.Get()
	t.Cleanup(func() { require.NoError(t, settings.BackupTargetSet.Set(oldTarget)) })
	configuredTarget := `{"type":"s3","endpoint":"https://s3.example.com",` +
		`"bucketName":"backups"}`
	require.NoError(t, settings.BackupTargetSet.Set(configuredTarget))
	source := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: util.BackupTargetSecretName, Namespace: util.LonghornSystemNamespaceName},
		Data: map[string][]byte{
			util.AWSAccessKey: []byte("access"),
			util.AWSSecretKey: []byte("secret"),
			"RESTIC_PASSWORD": []byte("stored-restic-password"),
		},
	}
	target, err := settings.DecodeBackupTarget(configuredTarget)
	require.NoError(t, err)
	source.Data[backuputil.RepositoryIDKey] = []byte(backuputil.RepositoryID(target))
	client := fake.NewSimpleClientset(source)
	client.PrependReactor("create", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		job := action.(clienttesting.CreateAction).GetObject().(*batchv1.Job)
		job.UID = uuid.NewUUID()
		return false, nil, nil
	})
	re := &ResticRestoreEngine{
		vmbo:         backupcommon.NewVMBackupOperatorBuilder().Build(),
		vmro:         restorecommon.NewVMRestoreOperatorBuilder().Build(),
		jobClient:    fakeclients.JobClient(client.BatchV1().Jobs),
		secretClient: fakeclients.SecretClient(client.CoreV1().Secrets),
		pvcCache:     fakeclients.PersistentVolumeClaimCache(client.CoreV1().PersistentVolumeClaims),
		clientset: kubefake.NewSimpleClientset(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "default", UID: "namespace-uid"},
		}),
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
	_, err = client.HarvesterhciV1beta1().VirtualMachineRestores(vmr.Namespace).Create(
		context.Background(), vmr, metav1.CreateOptions{},
	)
	require.NoError(t, err)
	client.ClearActions()
	return re, vmb, vmr, client
}

func TestCredentialFailureRetriesRestore(t *testing.T) {
	t.Setenv("HARVESTER_IMAGE", "harvester:test")
	re, vmb, vmr, client := credentialFixture(t)
	apiErr := errors.New("API unavailable")
	fail := true
	client.PrependReactor("get", "secrets", func(clienttesting.Action) (bool, runtime.Object, error) {
		if fail {
			return true, nil, apiErr
		}
		return false, nil, nil
	})
	vb, vr := &vmb.Status.VolumeBackups[0], &vmr.Status.VolumeRestores[0]
	name := re.checkJobName(vmr, vr)
	err := re.createCheckJob(vmr, vmb, vb, name)
	require.ErrorIs(t, err, engine.ErrRetryLater)
	require.ErrorIs(t, err, apiErr)
	err = re.createRestoreJob(vmr, vmb, vr, vb, re.jobName(vmr, vr))
	require.ErrorIs(t, err, engine.ErrRetryLater)
	require.ErrorIs(t, err, apiErr)
	jobs, err := client.BatchV1().Jobs(vmr.Namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, jobs.Items)
	fail = false
	require.ErrorIs(t, re.checkRemoteSnapshot(vmr, vmb, vr, vb), engine.ErrRetryLater)
	job, err := re.jobClient.Get(vmr.Namespace, name, metav1.GetOptions{})
	require.NoError(t, err)
	require.False(t, ptr.Deref(job.Spec.Suspend, false))
}

func TestJobsRejectInvalidCredentials(t *testing.T) {
	t.Setenv("HARVESTER_IMAGE", "harvester:test")
	for _, field := range []string{util.AWSAccessKey, util.AWSSecretKey, "RESTIC_PASSWORD",
		backuputil.RepositoryIDKey, "wrong-id", "changed-target"} {
		t.Run(field, func(t *testing.T) {
			re, vmb, vmr, client := credentialFixture(t)
			secrets := client.CoreV1().Secrets(util.LonghornSystemNamespaceName)
			source, err := secrets.Get(context.Background(), util.BackupTargetSecretName, metav1.GetOptions{})
			require.NoError(t, err)
			delete(source.Data, field)
			if field == "wrong-id" {
				source.Data[backuputil.RepositoryIDKey] = []byte("other")
			}
			if field == "changed-target" {
				require.NoError(t, settings.BackupTargetSet.Set(`{"type":"s3","bucketName":"other"}`))
			}
			_, err = secrets.Update(context.Background(), source, metav1.UpdateOptions{})
			require.NoError(t, err)
			vb, vr := &vmb.Status.VolumeBackups[0], &vmr.Status.VolumeRestores[0]
			require.ErrorIs(t, re.createCheckJob(vmr, vmb, vb, "check"), engine.ErrRetryLater)
			require.ErrorIs(t, re.createRestoreJob(vmr, vmb, vr, vb, "restore"), engine.ErrRetryLater)
			jobs, err := client.BatchV1().Jobs(vmr.Namespace).List(context.Background(), metav1.ListOptions{})
			require.NoError(t, err)
			require.Empty(t, jobs.Items)
		})
	}
}

func assertJobCredentials(t *testing.T, job *batchv1.Job, repositoryID string) {
	t.Helper()
	pod := job.Spec.Template.Spec
	require.Equal(t, backuputil.JobServiceAccount, pod.ServiceAccountName)
	require.True(t, ptr.Deref(pod.AutomountServiceAccountToken, false))
	container := pod.Containers[0]
	require.Equal(t, []string{
		"/usr/bin/harvester", "repository-exec", "--repository-id", repositoryID,
		"--engine", "restic", "--", "/bin/sh", "-c",
	}, container.Command)
	for _, env := range container.Env {
		require.NotContains(t, []string{
			util.AWSAccessKey, util.AWSSecretKey, "RESTIC_PASSWORD", "KOPIA_PASSWORD",
		}, env.Name)
	}
	manifest, err := json.Marshal(job)
	require.NoError(t, err)
	for _, value := range []string{`"access"`, `"secret"`, "access-rotated", "secret-rotated", "stored-restic-password"} {
		require.NotContains(t, string(manifest), value)
	}
}

func TestRestoreAccessUsesDestinationNamespace(t *testing.T) {
	t.Setenv("HARVESTER_IMAGE", "harvester:test")
	re, vmb, vmr, client := credentialFixture(t)
	vmr.Namespace = "destination"
	_, err := re.clientset.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: vmr.Namespace, UID: "destination-uid"},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	vb, vr := &vmb.Status.VolumeBackups[0], &vmr.Status.VolumeRestores[0]
	require.NoError(t, re.createCheckJob(vmr, vmb, vb, "check"))
	require.NoError(t, re.createRestoreJob(vmr, vmb, vr, vb, "restore"))
	bindings, err := re.clientset.RbacV1().RoleBindings(util.LonghornSystemNamespaceName).List(
		context.Background(), metav1.ListOptions{},
	)
	require.NoError(t, err)
	require.Len(t, bindings.Items, 1)
	require.Equal(t, vmr.Namespace, bindings.Items[0].Subjects[0].Namespace)
	for _, name := range []string{"check", "restore"} {
		job, err := client.BatchV1().Jobs(vmr.Namespace).Get(context.Background(), name, metav1.GetOptions{})
		require.NoError(t, err)
		assertJobCredentials(t, job, vmb.Status.BackupTarget.RepositoryID)
	}
}

func TestRestoreRetriesAccessFailure(t *testing.T) {
	t.Setenv("HARVESTER_IMAGE", "harvester:test")
	re, vmb, vmr, client := credentialFixture(t)
	apiErr := errors.New("RBAC API unavailable")
	re.clientset.(*kubefake.Clientset).PrependReactor("create", "rolebindings",
		func(clienttesting.Action) (bool, runtime.Object, error) { return true, nil, apiErr })
	vb, vr := &vmb.Status.VolumeBackups[0], &vmr.Status.VolumeRestores[0]
	for _, err := range []error{
		re.createCheckJob(vmr, vmb, vb, "check"),
		re.createRestoreJob(vmr, vmb, vr, vb, "restore"),
	} {
		require.ErrorIs(t, err, engine.ErrRetryLater)
		require.ErrorIs(t, err, apiErr)
	}
	jobs, err := client.BatchV1().Jobs(vmr.Namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, jobs.Items)
}
