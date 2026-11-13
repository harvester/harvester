package kopia

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
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
	"github.com/harvester/harvester/pkg/util/joblifecycle"
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
				var deletes []clienttesting.DeleteAction
				for _, action := range clientset.Actions() {
					if action.GetVerb() == "delete" {
						deletes = append(deletes, action.(clienttesting.DeleteAction))
					}
				}
				require.Len(t, deletes, 1)
				require.Equal(t, job.UID, *deletes[0].GetDeleteOptions().Preconditions.UID)
				require.Equal(t, metav1.DeletePropagationBackground, *deletes[0].GetDeleteOptions().PropagationPolicy)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestForgetJobDeletionFailureIsRetried(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "forget", Namespace: "default", UID: "forget-uid"},
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
		ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "default", UID: "backup-uid"},
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
	vmbo := backupcommon.NewVMBackupOperatorBuilder().Build()
	vsHelper := backupcommon.NewVolumeSnapshotHelper(nil, nil, nil, nil, vmbo, nil, nil)
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: "kopia-backup-volume", Namespace: vmb.Namespace,
	}}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "backup-source-kopia-clone", Namespace: vmb.Namespace, UID: "clone-uid",
			OwnerReferences: []metav1.OwnerReference{vsHelper.BuildOwnerReference(vmb)},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			DataSource: &corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(snapshotv1.GroupName), Kind: "VolumeSnapshot", Name: "volume",
			},
		},
	}
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

	ke := &KopiaEngine{
		vmbo:      vmbo,
		vsHelper:  vsHelper,
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

func forgetFixture(t *testing.T) (*KopiaEngine, *harvesterv1.VirtualMachineBackup, *fake.Clientset) {
	t.Helper()
	t.Setenv(kopiautil.ImageEnvVar, "harvester:test")
	for _, setting := range []settings.Setting{
		settings.BackupTargetSet, settings.ThirdPartyBackupConfigSet,
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
	configuredTarget := `{"type":"s3","endpoint":"http://s3.test:9000",` +
		`"bucketName":"test","bucketRegion":"test-region"}`
	require.NoError(t, settings.BackupTargetSet.Set(configuredTarget))
	vmb := &harvesterv1.VirtualMachineBackup{
		TypeMeta: metav1.TypeMeta{Kind: "VirtualMachineBackup"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "backup", Namespace: "default", UID: "backup-uid",
			DeletionTimestamp: ptr.To(metav1.Now()),
		},
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
			util.AWSAccessKey:     []byte("access"),
			util.AWSSecretKey:     []byte("secret"),
			kopiautil.PasswordKey: []byte("stored-kopia-password"),
		},
	}
	credentials.Data[backuputil.RepositoryIDKey] = []byte(vmb.Status.BackupTarget.RepositoryID)
	clientset := fake.NewSimpleClientset(credentials)
	clientset.PrependReactor("create", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		job := action.(clienttesting.CreateAction).GetObject().(*batchv1.Job)
		job.UID = uuid.NewUUID()
		return false, nil, nil
	})
	vmbo := backupcommon.NewVMBackupOperatorBuilder().Build()
	ke := &KopiaEngine{
		vmbo:         vmbo,
		vsHelper:     backupcommon.NewVolumeSnapshotHelper(nil, nil, nil, nil, vmbo, nil, nil),
		secretClient: fakeclients.SecretClient(clientset.CoreV1().Secrets),
		jobClient:    fakeclients.JobClient(clientset.BatchV1().Jobs),
		jobCache:     fakeclients.JobCache(clientset.BatchV1().Jobs),
		clientset: kubefake.NewSimpleClientset(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "default", UID: "namespace-uid"},
		}),
	}

	return ke, vmb, clientset
}

func TestForgetJobRetainsCompletion(t *testing.T) {
	ke, vmb, clientset := forgetFixture(t)
	jobName, err := ke.forgetJobName(&vmb.Status.VolumeBackups[0])
	require.NoError(t, err)
	require.ErrorIs(t, ke.forgetVolume(vmb, 0), engine.ErrRetryLater)
	job, err := clientset.BatchV1().Jobs(vmb.Namespace).Get(
		context.Background(), jobName, metav1.GetOptions{},
	)
	require.NoError(t, err)
	require.Equal(t, int32(3), *job.Spec.BackoffLimit)
	require.Nil(t, job.Spec.TTLSecondsAfterFinished)
	require.Contains(t, job.Finalizers, joblifecycle.CompletionFinalizer)
	require.Empty(t, job.OwnerReferences)
	require.Equal(t, string(vmb.UID), job.Labels[joblifecycle.LabelParentUID])
	require.NotContains(t, job.Labels, "harvesterhci.io/vm-backup-name")
	require.NotContains(t, job.Labels, "harvesterhci.io/vm-backup-namespace")

	job.Status.Succeeded = 1
	job.Status.Failed = 1
	_, err = clientset.BatchV1().Jobs(vmb.Namespace).UpdateStatus(
		context.Background(), job, metav1.UpdateOptions{},
	)
	require.NoError(t, err)
	clientset.ClearActions()
	require.NoError(t, ke.forgetVolume(vmb, 0))
	for _, action := range clientset.Actions() {
		require.NotEqual(t, "create", action.GetVerb(), "completed volume cleanup must not be recreated")
		require.NotEqual(t, "delete", action.GetVerb(), "completion checkpoint must survive until VMBackup deletion")
	}
}

func TestJobsRejectInvalidCredentials(t *testing.T) {
	for _, field := range []string{util.AWSAccessKey, util.AWSSecretKey, kopiautil.PasswordKey,
		backuputil.RepositoryIDKey, "wrong-id", "changed-target"} {
		t.Run(field, func(t *testing.T) {
			ke, vmb, client := forgetFixture(t)
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
			vb := &vmb.Status.VolumeBackups[0]
			require.Error(t, ke.createBackupJob(vmb, vb, "clone", "backup"))
			require.Error(t, ke.createCheckJob(vmb, vb, "check"))
			require.Error(t, ke.createForgetJob(vmb, vb, "forget"))
			jobs, err := client.BatchV1().Jobs(vmb.Namespace).List(context.Background(), metav1.ListOptions{})
			require.NoError(t, err)
			require.Empty(t, jobs.Items)
		})
	}
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

func TestExistingForgetDoesNotReadCredentials(t *testing.T) {
	ke, vmb, client := forgetFixture(t)
	require.ErrorIs(t, ke.forgetVolume(vmb, 0), engine.ErrRetryLater)
	jobName, err := ke.forgetJobName(&vmb.Status.VolumeBackups[0])
	require.NoError(t, err)
	require.NoError(t, client.CoreV1().Secrets(util.LonghornSystemNamespaceName).Delete(
		context.Background(), util.BackupTargetSecretName, metav1.DeleteOptions{},
	))
	// Running and completed Jobs no longer depend on the source Secret.
	require.ErrorIs(t, ke.forgetVolume(vmb, 0), engine.ErrRetryLater)
	jobs := client.BatchV1().Jobs(vmb.Namespace)
	job, err := jobs.Get(context.Background(), jobName, metav1.GetOptions{})
	require.NoError(t, err)
	job.Status.Succeeded = 1
	_, err = jobs.UpdateStatus(context.Background(), job, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.NoError(t, ke.forgetVolume(vmb, 0))
}

func TestJobsHideCredentials(t *testing.T) {
	ke, vmb, client := forgetFixture(t)
	vmb.DeletionTimestamp = nil
	vb := &vmb.Status.VolumeBackups[0]
	for _, suffix := range []string{"", "-rotated"} {
		if suffix != "" {
			secrets := client.CoreV1().Secrets(util.LonghornSystemNamespaceName)
			source, err := secrets.Get(context.Background(), util.BackupTargetSecretName, metav1.GetOptions{})
			require.NoError(t, err)
			source.Data[util.AWSAccessKey] = []byte("access-rotated")
			source.Data[util.AWSSecretKey] = []byte("secret-rotated")
			_, err = secrets.Update(context.Background(), source, metav1.UpdateOptions{})
			require.NoError(t, err)
		}
		require.NoError(t, ke.createBackupJob(vmb, vb, "clone", "backup"+suffix))
		require.NoError(t, ke.createCheckJob(vmb, vb, "check"+suffix))
		require.NoError(t, ke.createForgetJob(vmb, vb, "forget"+suffix))
	}
	for _, suffix := range []string{"", "-rotated"} {
		for _, name := range []string{"backup", "check", "forget"} {
			job, err := client.BatchV1().Jobs(vmb.Namespace).Get(
				context.Background(), name+suffix, metav1.GetOptions{},
			)
			require.NoError(t, err)
			require.False(t, ptr.Deref(job.Spec.Suspend, false))
			assertJobCredentials(t, job, vmb.Status.BackupTarget.RepositoryID)
		}
	}
	secrets, err := client.CoreV1().Secrets(vmb.Namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, secrets.Items)
	for _, action := range client.Actions() {
		require.False(t, action.Matches("create", "secrets"))
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
		"--engine", "kopia", "--", "/bin/sh", "-c",
	}, container.Command)
	for _, env := range container.Env {
		require.NotContains(t, []string{
			util.AWSAccessKey, util.AWSSecretKey, "RESTIC_PASSWORD", "KOPIA_PASSWORD",
		}, env.Name)
	}
	manifest, err := json.Marshal(job)
	require.NoError(t, err)
	for _, value := range []string{`"access"`, `"secret"`, "access-rotated", "secret-rotated", "stored-kopia-password"} {
		require.NotContains(t, string(manifest), value)
	}
}

func TestCleanupDecisionSurvivesRestart(t *testing.T) {
	for _, remoteDeleted := range []bool{true, false} {
		annotation := backupcommon.SkipAnnotation
		if remoteDeleted {
			annotation = backupcommon.CompleteAnnotation
		}
		t.Run(annotation, func(t *testing.T) {
			ke, vmb, client := forgetFixture(t)
			ctx := context.Background()
			backups := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace)
			vmb.ResourceVersion = "1"
			_, err := backups.Create(ctx, vmb, metav1.CreateOptions{})
			require.NoError(t, err)
			ke.vmbo = backupcommon.NewVMBackupOperatorBuilder().
				WithClient(fakeclients.VMBackupClient(client.HarvesterhciV1beta1().VirtualMachineBackups)).Build()
			require.ErrorIs(t, ke.forgetVolume(vmb, 0), engine.ErrRetryLater)
			jobs := client.BatchV1().Jobs(vmb.Namespace)
			jobName, err := ke.forgetJobName(&vmb.Status.VolumeBackups[0])
			require.NoError(t, err)
			job, err := jobs.Get(ctx, jobName, metav1.GetOptions{})
			require.NoError(t, err)
			job.Status.Succeeded = 1
			_, err = jobs.UpdateStatus(ctx, job, metav1.UpdateOptions{})
			require.NoError(t, err)

			persistErr := errors.New("checkpoint update failed")
			failUpdate := true
			client.PrependReactor("update", "virtualmachinebackups",
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					if failUpdate {
						return true, nil, persistErr
					}
					obj := action.(clienttesting.UpdateAction).GetObject()
					obj.(metav1.Object).SetResourceVersion("2")
					return false, nil, nil
				})
			client.ClearActions()
			_, err = ke.Remove(vmb, remoteDeleted)
			require.ErrorIs(t, err, persistErr)
			for _, action := range client.Actions() {
				require.NotEqual(t, "delete", action.GetVerb(), "retain Jobs until the checkpoint is durable")
				if action.GetResource().Resource == "jobs" {
					require.NotEqual(t, "update", action.GetVerb(), "retain completion protection until the checkpoint is durable")
				}
			}
			require.False(t, backupcommon.Ready(ke.vmbo, vmb), "leave the informer object unchanged")

			failUpdate = false
			deleteErr := errors.New("Job deletion failed")
			failDelete := true
			client.PrependReactor("delete", "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
				if failDelete {
					return true, nil, deleteErr
				}
				return false, nil, nil
			})
			updated, err := ke.Remove(vmb, remoteDeleted)
			require.ErrorIs(t, err, deleteErr)
			require.Equal(t, "2", updated.ResourceVersion)
			stored, err := backups.Get(ctx, vmb.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, "true", stored.Annotations[annotation])

			// A restarted engine resumes local deletion without recreating remote work.
			restarted := &KopiaEngine{vmbo: ke.vmbo, jobClient: ke.jobClient}
			client.ClearActions()
			failDelete = false
			updated, err = restarted.Remove(stored, !remoteDeleted)
			require.NoError(t, err)
			require.Equal(t, stored.Annotations, updated.Annotations, "preserve the original decision")
			for _, action := range client.Actions() {
				require.NotEqual(t, "create", action.GetVerb(), "do not recreate completed remote work")
			}
			// Missing Jobs do not prevent another finalizer-removal attempt.
			_, err = restarted.Remove(stored, !remoteDeleted)
			require.NoError(t, err)
		})
	}
}

func TestBackupJobFailurePrecedesRemoteCheck(t *testing.T) {
	vmb := &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "default"},
		Status: harvesterv1.VirtualMachineBackupStatus{
			VolumeBackups: []harvesterv1.VolumeBackup{{Name: ptr.To("volume")}},
		},
	}
	vb := &vmb.Status.VolumeBackups[0]
	vmbo := backupcommon.NewVMBackupOperatorBuilder().Build()
	ke := &KopiaEngine{vmbo: vmbo}
	jobName, err := ke.jobName(vb)
	require.NoError(t, err)
	checkName, err := ke.checkJobName(vb)
	require.NoError(t, err)
	clientset := fake.NewSimpleClientset(
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: vmb.Namespace},
			Status: batchv1.JobStatus{Failed: 1, Conditions: []batchv1.JobCondition{{
				Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
			}}}},
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: checkName, Namespace: vmb.Namespace},
			Status: batchv1.JobStatus{Succeeded: 1}},
	)
	ke.jobCache = fakeclients.JobCache(clientset.BatchV1().Jobs)
	err = ke.reconcileBackupJob(vmb, vb)
	require.ErrorContains(t, err, "failed")
	require.False(t, vmbo.GetVolBackupReadyToUse(vb))
}

// removeFixture holds two independent per-volume cleanup results.
func removeFixture(t *testing.T) (*KopiaEngine, *harvesterv1.VirtualMachineBackup, *fake.Clientset) {
	t.Helper()
	ke, vmb, client := forgetFixture(t)
	vmb.Status.VolumeBackups = append(vmb.Status.VolumeBackups, harvesterv1.VolumeBackup{Name: ptr.To("data")})
	_, err := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).Create(
		context.Background(), vmb, metav1.CreateOptions{},
	)
	require.NoError(t, err)
	ke.vmbo = backupcommon.NewVMBackupOperatorBuilder().
		WithClient(fakeclients.VMBackupClient(client.HarvesterhciV1beta1().VirtualMachineBackups)).Build()
	for i := range vmb.Status.VolumeBackups {
		require.ErrorIs(t, ke.forgetVolume(vmb, i), engine.ErrRetryLater)
	}
	return ke, vmb, client
}

func completeForgetJob(t *testing.T, client *fake.Clientset, namespace, name string) *batchv1.Job {
	t.Helper()
	jobs := client.BatchV1().Jobs(namespace)
	job, err := jobs.Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)
	job.Status.Succeeded = 1
	job, err = jobs.UpdateStatus(context.Background(), job, metav1.UpdateOptions{})
	require.NoError(t, err)
	return job
}

func TestRemoveWaitsForAllVolumes(t *testing.T) {
	ke, vmb, client := removeFixture(t)
	first, err := ke.forgetJobName(&vmb.Status.VolumeBackups[0])
	require.NoError(t, err)
	second, err := ke.forgetJobName(&vmb.Status.VolumeBackups[1])
	require.NoError(t, err)
	job := completeForgetJob(t, client, vmb.Namespace, first)
	// An external deletion must not erase this volume's completed result.
	job.DeletionTimestamp = ptr.To(metav1.Now())
	_, err = client.BatchV1().Jobs(vmb.Namespace).Update(context.Background(), job, metav1.UpdateOptions{})
	require.NoError(t, err)
	client.ClearActions()
	_, err = ke.Remove(vmb, true)
	require.ErrorIs(t, err, engine.ErrRetryLater)
	for _, action := range client.Actions() {
		require.NotEqual(t, "delete", action.GetVerb(), "retain the first result while another volume is pending")
		require.NotEqual(t, "update", action.GetVerb(), "do not release the first result early")
	}
	completeForgetJob(t, client, vmb.Namespace, second)
	updated, err := ke.Remove(vmb, true)
	require.NoError(t, err)
	require.Equal(t, "true", updated.Annotations[backupcommon.CompleteAnnotation])
	jobs, err := client.BatchV1().Jobs(vmb.Namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, jobs.Items)
}

func TestRemoveRetriesPartialDeletion(t *testing.T) {
	ke, vmb, client := removeFixture(t)
	for i := range vmb.Status.VolumeBackups {
		name, err := ke.forgetJobName(&vmb.Status.VolumeBackups[i])
		require.NoError(t, err)
		completeForgetJob(t, client, vmb.Namespace, name)
	}
	second, err := ke.forgetJobName(&vmb.Status.VolumeBackups[1])
	require.NoError(t, err)
	deleteErr := errors.New("second Job deletion failed")
	failDelete := true
	client.PrependReactor("delete", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if failDelete && action.(clienttesting.DeleteAction).GetName() == second {
			return true, nil, deleteErr
		}
		return false, nil, nil
	})
	updated, err := ke.Remove(vmb, true)
	require.ErrorIs(t, err, deleteErr)
	require.Equal(t, "true", updated.Annotations[backupcommon.CompleteAnnotation])
	remaining, err := client.BatchV1().Jobs(vmb.Namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, remaining.Items, 1)
	stored, err := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).Get(
		context.Background(), vmb.Name, metav1.GetOptions{},
	)
	require.NoError(t, err)
	restarted := &KopiaEngine{vmbo: ke.vmbo, jobClient: ke.jobClient}
	failDelete = false
	client.ClearActions()
	_, err = restarted.Remove(stored, true)
	require.NoError(t, err)
	_, err = restarted.Remove(stored, true)
	require.NoError(t, err, "a failed VMBackup finalizer update must be safe to retry after Jobs are gone")
	for _, action := range client.Actions() {
		require.NotEqual(t, "create", action.GetVerb())
	}
}

func TestRemoveCancelsPendingJobs(t *testing.T) {
	ke, vmb, client := removeFixture(t)
	// Disconnected cleanup must not need remote credentials or start more work.
	restarted := &KopiaEngine{vmbo: ke.vmbo, jobClient: ke.jobClient}
	client.ClearActions()
	updated, err := restarted.Remove(vmb, false)
	require.NoError(t, err)
	require.Equal(t, "true", updated.Annotations[backupcommon.SkipAnnotation])
	for _, action := range client.Actions() {
		require.NotEqual(t, "create", action.GetVerb())
		require.NotEqual(t, "secrets", action.GetResource().Resource)
	}
	jobs, err := client.BatchV1().Jobs(vmb.Namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, jobs.Items)
}
