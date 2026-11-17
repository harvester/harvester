package kopia

import (
	"context"
	"errors"
	"strings"
	"testing"

	snapshotv6 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/backup/engine"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/datamover"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/util/jobcredentials"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
	kopiautil "github.com/harvester/harvester/pkg/util/kopia"
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
	ke, vmb, pvc, client := cloneFixture()
	vb := &vmb.Status.VolumeBackups[0]
	vb.ReadyToUse = ptr.To(true)
	failure := errors.New("API unavailable")
	fail := true
	client.PrependReactor("delete", "persistentvolumeclaims", func(clienttesting.Action) (bool, runtime.Object, error) {
		if fail {
			return true, nil, failure
		}
		return false, nil, nil
	})
	require.ErrorIs(t, ke.Reconcile(vmb, 0, nil), failure)
	_, err := client.SnapshotV1().VolumeSnapshots(vmb.Namespace).Get(context.Background(), backupcommon.SnapshotName(vmb, vb), metav1.GetOptions{})
	require.NoError(t, err, "source snapshot remains until clone cleanup succeeds")
	fail = false
	for i := 0; i < 8; i++ {
		err = ke.Reconcile(vmb, 0, nil)
		if err == nil {
			break
		}
		require.ErrorIs(t, err, engine.ErrRetryLater)
	}
	require.NoError(t, err)
	_, err = client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(context.Background(), pvc.Name, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
}

func forgetFixture(t *testing.T) (*KopiaEngine, *harvesterv1.VirtualMachineBackup, *fake.Clientset) {
	t.Helper()
	t.Setenv(kopiautil.ImageEnvVar, "harvester:test")
	for _, setting := range []settings.Setting{
		settings.ThirdPartyBackupConfigSet,
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
	old := settings.BackupTargetSet.Get()
	t.Cleanup(func() { require.NoError(t, settings.BackupTargetSet.Set(old)) })
	require.NoError(t, settings.BackupTargetSet.Set(`{"type":"s3","endpoint":"http://s3.test:9000","bucketName":"test","bucketRegion":"test-region"}`))

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
			util.AWSAccessKey: []byte("access"),
			util.AWSSecretKey: []byte("secret"),
		},
	}
	credentials.Data[backuputil.RepositoryIDKey] = []byte(vmb.Status.BackupTarget.RepositoryID)
	credentials.Data[kopiautil.PasswordKey] = []byte("stored-password")
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
		clientset:    fakeclients.KubernetesClient(clientset),
	}

	scheme := runtime.NewScheme()
	_ = snapshotv6.AddToScheme(scheme)
	ke.imports = datamover.Snapshots{Client: dynamicfake.NewSimpleDynamicClient(scheme)}
	ke.vsClient = fakeclients.VolumeSnapshotClient(clientset.SnapshotV1().VolumeSnapshots)

	return ke, vmb, clientset
}

func TestForgetJobRetainsCompletion(t *testing.T) {
	ke, vmb, clientset := forgetFixture(t)
	jobName, err := ke.forgetJobName(vmb, &vmb.Status.VolumeBackups[0])
	require.NoError(t, err)
	require.ErrorIs(t, ke.forgetVolume(vmb, 0), engine.ErrRetryLater)
	job, err := clientset.BatchV1().Jobs(joblifecycle.Namespace).Get(
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
	_, err = clientset.BatchV1().Jobs(joblifecycle.Namespace).UpdateStatus(
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

func TestJobSecretUsesStoredPassword(t *testing.T) {
	ke, vmb, client := forgetFixture(t)
	vb := &vmb.Status.VolumeBackups[0]
	name, err := ke.checkJobName(vmb, vb)
	require.NoError(t, err)
	require.NoError(t, ke.createCheckJob(vmb, vb, name))
	secret, err := client.CoreV1().Secrets(joblifecycle.Namespace).Get(context.Background(), jobcredentials.Name(name), metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, []byte("stored-password"), secret.Data[kopiautil.PasswordKey])
	require.Len(t, secret.Data, 3)
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

	jobName, err := ke.jobName(vmb, vb)
	require.NoError(t, err)
	forgetName, err := ke.forgetJobName(vmb, vb)
	require.NoError(t, err)
	checkName, err := ke.checkJobName(vmb, vb)
	require.NoError(t, err)

	for _, generatedName := range []string{
		jobcredentials.Name(longName),
		ke.clonePVCName(vmb, vb),
		jobName,
		forgetName,
		checkName,
	} {
		require.LessOrEqual(t, len(generatedName), 63)
	}

	other := vb.DeepCopy()
	other.Name = ptr.To(longName + "-volume-b")
	otherJobName, err := ke.jobName(vmb, other)
	require.NoError(t, err)
	require.NotEqual(t, jobName, otherJobName)
}

func TestForgetRepairsCredentials(t *testing.T) {
	ke, vmb, client := forgetFixture(t)
	require.ErrorIs(t, ke.forgetVolume(vmb, 0), engine.ErrRetryLater)
	secrets := client.CoreV1().Secrets(joblifecycle.Namespace)
	jobName, err := ke.forgetJobName(vmb, &vmb.Status.VolumeBackups[0])
	require.NoError(t, err)
	name := jobcredentials.Name(jobName)
	original, err := secrets.Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, original.OwnerReferences, 1)
	require.Equal(t, "Job", original.OwnerReferences[0].Kind)
	require.NoError(t, secrets.Delete(context.Background(), name, metav1.DeleteOptions{}))

	// The existing pending Job must not bypass dependency repair.
	require.ErrorIs(t, ke.forgetVolume(vmb, 0), engine.ErrRetryLater)
	repaired, err := secrets.Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, original.Data, repaired.Data)
	require.Equal(t, original.OwnerReferences, repaired.OwnerReferences)

	// Completion must remain usable even after source credentials disappear.
	jobs := client.BatchV1().Jobs(joblifecycle.Namespace)
	job, err := jobs.Get(context.Background(), jobName, metav1.GetOptions{})
	require.NoError(t, err)
	job.Status.Succeeded = 1
	_, err = jobs.UpdateStatus(context.Background(), job, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.NoError(t, client.CoreV1().Secrets(util.LonghornSystemNamespaceName).Delete(
		context.Background(), util.BackupTargetSecretName, metav1.DeleteOptions{},
	))
	require.NoError(t, ke.forgetVolume(vmb, 0))
}

func TestJobsOwnCredentials(t *testing.T) {
	ke, vmb, client := forgetFixture(t)
	vmb.DeletionTimestamp = nil
	vb := &vmb.Status.VolumeBackups[0]
	backupName, err := ke.jobName(vmb, vb)
	require.NoError(t, err)
	checkName, err := ke.checkJobName(vmb, vb)
	require.NoError(t, err)
	forgetName, err := ke.forgetJobName(vmb, vb)
	require.NoError(t, err)
	require.NoError(t, ke.createBackupJob(vmb, vb, "clone", backupName))
	require.NoError(t, ke.createCheckJob(vmb, vb, checkName))
	require.NoError(t, ke.createForgetJob(vmb, vb, forgetName))
	secrets := client.CoreV1().Secrets(joblifecycle.Namespace)
	for _, name := range []string{backupName, checkName, forgetName} {
		job, err := client.BatchV1().Jobs(joblifecycle.Namespace).Get(context.Background(), name, metav1.GetOptions{})
		require.NoError(t, err)
		require.False(t, *job.Spec.Suspend)
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
	// Repeated reconciliation repairs dependencies for every Job type.
	require.ErrorIs(t, ke.reconcileBackupJob(vmb, vb), engine.ErrRetryLater)
	_, err = ke.checkRemoteSnapshot(vmb, vb)
	require.ErrorIs(t, err, engine.ErrRetryLater)
	require.ErrorIs(t, ke.forgetVolume(vmb, 0), engine.ErrRetryLater)
	list, err := secrets.List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, list.Items, 3)
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
			jobs := client.BatchV1().Jobs(joblifecycle.Namespace)
			jobName, err := ke.forgetJobName(vmb, &vmb.Status.VolumeBackups[0])
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
			require.False(t, backupcommon.CleanupRecorded(ke.vmbo, vmb), "leave the informer object unchanged")

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
			restarted := *ke
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
	ke, vmb, client := forgetFixture(t)
	vb := &vmb.Status.VolumeBackups[0]
	for _, prefix := range []string{jobNamePrefix, checkJobNamePrefix} {
		job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: joblifecycle.Name(prefix, vmb, *vb.Name), Namespace: joblifecycle.Namespace}}
		joblifecycle.Track(&job.ObjectMeta, vmb, vmBackupKind)
		job.Status.Succeeded = 1
		if prefix == jobNamePrefix {
			job.Status.Succeeded = 0
			job.Status.Failed = 1
			job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}
		}
		_, err := client.BatchV1().Jobs(job.Namespace).Create(context.Background(), job, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	require.ErrorContains(t, ke.reconcileBackupJob(vmb, vb), "failed")
	require.False(t, ke.vmbo.GetVolBackupReadyToUse(vb))
}

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
	first, err := ke.forgetJobName(vmb, &vmb.Status.VolumeBackups[0])
	require.NoError(t, err)
	second, err := ke.forgetJobName(vmb, &vmb.Status.VolumeBackups[1])
	require.NoError(t, err)
	job := completeForgetJob(t, client, joblifecycle.Namespace, first)
	// An external deletion must not erase this volume's completed result.
	job.DeletionTimestamp = ptr.To(metav1.Now())
	_, err = client.BatchV1().Jobs(joblifecycle.Namespace).Update(context.Background(), job, metav1.UpdateOptions{})
	require.NoError(t, err)
	client.ClearActions()
	_, err = ke.Remove(vmb, true)
	require.ErrorIs(t, err, engine.ErrRetryLater)
	for _, action := range client.Actions() {
		require.NotEqual(t, "delete", action.GetVerb(), "retain the first result while another volume is pending")
		require.NotEqual(t, "update", action.GetVerb(), "do not release the first result early")
	}
	completeForgetJob(t, client, joblifecycle.Namespace, second)
	updated, err := ke.Remove(vmb, true)
	require.NoError(t, err)
	require.Equal(t, "true", updated.Annotations[backupcommon.CompleteAnnotation])
	jobs, err := client.BatchV1().Jobs(joblifecycle.Namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, jobs.Items)
}

func TestRemoveRetriesPartialDeletion(t *testing.T) {
	ke, vmb, client := removeFixture(t)
	for i := range vmb.Status.VolumeBackups {
		name, err := ke.forgetJobName(vmb, &vmb.Status.VolumeBackups[i])
		require.NoError(t, err)
		completeForgetJob(t, client, joblifecycle.Namespace, name)
	}
	second, err := ke.forgetJobName(vmb, &vmb.Status.VolumeBackups[1])
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
	remaining, err := client.BatchV1().Jobs(joblifecycle.Namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, remaining.Items, 1)
	stored, err := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).Get(
		context.Background(), vmb.Name, metav1.GetOptions{},
	)
	require.NoError(t, err)
	restarted := *ke
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
	for _, target := range []string{`{}`, `{"type":"s3","bucketName":"another-repository"}`} {
		t.Run(target, func(t *testing.T) {
			ke, vmb, client := removeFixture(t)
			require.NoError(t, settings.BackupTargetSet.Set(target))
			require.NoError(t, client.CoreV1().Secrets(util.LonghornSystemNamespaceName).Delete(
				context.Background(), util.BackupTargetSecretName, metav1.DeleteOptions{},
			))
			// Changed or disconnected targets must not need credentials or start remote work.
			restarted := *ke
			client.ClearActions()
			updated, err := restarted.Remove(vmb, false)
			require.NoError(t, err)
			require.Equal(t, "true", updated.Annotations[backupcommon.SkipAnnotation])
			for _, action := range client.Actions() {
				require.NotEqual(t, "create", action.GetVerb())
				require.NotEqual(t, "secrets", action.GetResource().Resource)
			}
			jobs, err := client.BatchV1().Jobs(joblifecycle.Namespace).List(context.Background(), metav1.ListOptions{})
			require.NoError(t, err)
			require.Empty(t, jobs.Items)
		})
	}
}

func TestInvalidCredentialsCreateNoJobs(t *testing.T) {
	for _, operation := range []string{"backup", "check", "forget"} {
		for _, key := range []string{util.AWSAccessKey, util.AWSSecretKey, kopiautil.PasswordKey, backuputil.RepositoryIDKey} {
			t.Run(operation+"/"+key, func(t *testing.T) {
				ke, vmb, client := forgetFixture(t)
				source, err := client.CoreV1().Secrets(util.LonghornSystemNamespaceName).Get(context.Background(), util.BackupTargetSecretName, metav1.GetOptions{})
				require.NoError(t, err)
				delete(source.Data, key)
				_, err = client.CoreV1().Secrets(source.Namespace).Update(context.Background(), source, metav1.UpdateOptions{})
				require.NoError(t, err)
				vb := &vmb.Status.VolumeBackups[0]
				switch operation {
				case "backup":
					err = ke.createBackupJob(vmb, vb, "clone", "invalid")
				case "check":
					err = ke.createCheckJob(vmb, vb, "invalid")
				case "forget":
					err = ke.createForgetJob(vmb, vb, "invalid")
				}
				require.Error(t, err)
				jobs, err := client.BatchV1().Jobs(joblifecycle.Namespace).List(context.Background(), metav1.ListOptions{})
				require.NoError(t, err)
				require.Empty(t, jobs.Items)
			})
		}
	}
}

func TestJobsCaptureCredentialsAcrossRotation(t *testing.T) {
	ke, vmb, client := forgetFixture(t)
	vb := &vmb.Status.VolumeBackups[0]
	require.NoError(t, ke.createCheckJob(vmb, vb, "before"))
	old, err := client.CoreV1().Secrets(joblifecycle.Namespace).Get(context.Background(), jobcredentials.Name("before"), metav1.GetOptions{})
	require.NoError(t, err)
	source, err := client.CoreV1().Secrets(util.LonghornSystemNamespaceName).Get(context.Background(), util.BackupTargetSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	source.Data[util.AWSAccessKey] = []byte("rotated-access")
	source.Data[util.AWSSecretKey] = []byte("rotated-secret")
	_, err = client.CoreV1().Secrets(source.Namespace).Update(context.Background(), source, metav1.UpdateOptions{})
	require.NoError(t, err)
	job, err := ke.jobClient.Get(joblifecycle.Namespace, "before", metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, ke.ensureSecret(vmb, job))
	kept, err := client.CoreV1().Secrets(joblifecycle.Namespace).Get(context.Background(), old.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, old.Data, kept.Data)
	require.NoError(t, ke.createCheckJob(vmb, vb, "after"))
	current, err := client.CoreV1().Secrets(joblifecycle.Namespace).Get(context.Background(), jobcredentials.Name("after"), metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, []byte("rotated-access"), current.Data[util.AWSAccessKey])
	require.Equal(t, old.Data[kopiautil.PasswordKey], current.Data[kopiautil.PasswordKey])
	require.False(t, *job.Spec.Template.Spec.AutomountServiceAccountToken)
	for _, env := range job.Spec.Template.Spec.Containers[0].Env {
		if env.Name == util.AWSAccessKey || env.Name == util.AWSSecretKey || env.Name == kopiautil.PasswordKey {
			require.Empty(t, env.Value)
			require.NotNil(t, env.ValueFrom.SecretKeyRef)
		}
	}
}
