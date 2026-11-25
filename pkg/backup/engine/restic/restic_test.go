package restic

import (
	"context"
	"encoding/json"
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
	resticutil "github.com/harvester/harvester/pkg/util/restic"
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
			re := &ResticEngine{jobClient: fakeclients.JobClient(clientset.BatchV1().Jobs)}
			err := re.syncFromForgetJob(job)
			if tt.wantRetry {
				require.ErrorIs(t, err, engine.ErrRetryLater)
			} else if tt.wantFailed {
				require.Error(t, err)
				require.NotErrorIs(t, err, engine.ErrRetryLater)
			} else {
				require.NoError(t, err)
			}
			_, err = clientset.BatchV1().Jobs(job.Namespace).Get(context.Background(), job.Name, metav1.GetOptions{})
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
	re := &ResticEngine{jobClient: fakeclients.JobClient(clientset.BatchV1().Jobs)}
	require.ErrorIs(t, re.syncFromForgetJob(job), deleteErr)
}

func TestReconcileRetriesTemporaryCleanup(t *testing.T) {
	re, vmb, pvc, client := cloneFixture()
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
	require.ErrorIs(t, re.Reconcile(vmb, 0, nil), failure)
	_, err := client.SnapshotV1().VolumeSnapshots(vmb.Namespace).Get(context.Background(), backupcommon.SnapshotName(vmb, vb), metav1.GetOptions{})
	require.NoError(t, err, "source snapshot remains until clone cleanup succeeds")
	fail = false
	for i := 0; i < 8; i++ {
		err = re.Reconcile(vmb, 0, nil)
		if err == nil {
			break
		}
		require.True(t, errors.Is(err, engine.ErrRetryLater) || errors.Is(err, datamover.ErrPending))
	}
	require.NoError(t, err)
	_, err = client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(context.Background(), pvc.Name, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
}

func TestJobSecretUsesStoredPassword(t *testing.T) {
	re, vmb, client := forgetFixture(t)
	vb := &vmb.Status.VolumeBackups[0]
	name, err := re.checkJobName(vmb, vb)
	require.NoError(t, err)
	require.NoError(t, re.createCheckJob(vmb, vb, name))
	secret, err := client.CoreV1().Secrets(joblifecycle.Namespace).Get(context.Background(), jobcredentials.Name(name), metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, []byte("stored-password"), secret.Data[resticutil.PasswordKey])
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
	re := &ResticEngine{vmbo: backupcommon.NewVMBackupOperatorBuilder().Build()}

	jobName, err := re.jobName(vmb, vb)
	require.NoError(t, err)
	forgetName, err := re.forgetJobName(vmb, vb)
	require.NoError(t, err)
	checkName, err := re.checkJobName(vmb, vb)
	require.NoError(t, err)

	for _, generatedName := range []string{
		jobcredentials.Name(longName),
		re.clonePVCName(vmb, vb),
		jobName,
		forgetName,
		checkName,
	} {
		require.LessOrEqual(t, len(generatedName), 63)
	}

	other := vb.DeepCopy()
	other.Name = ptr.To(longName + "-volume-b")
	otherJobName, err := re.jobName(vmb, other)
	require.NoError(t, err)
	require.NotEqual(t, jobName, otherJobName)
}

func forgetFixture(t *testing.T) (*ResticEngine, *harvesterv1.VirtualMachineBackup, *fake.Clientset) {
	t.Helper()
	t.Setenv(resticutil.ImageEnvVar, "harvester:test")
	for _, setting := range []settings.Setting{
		settings.BackupTargetSet, settings.ThirdPartyBackupConfigSet,
	} {
		old := setting.Get()
		t.Cleanup(func() { require.NoError(t, setting.Set(old)) })
		require.NoError(t, setting.Set(setting.GetDefault()))
	}
	target := `{"type":"s3","endpoint":"http://s3.test:9000",` +
		`"bucketName":"test","bucketRegion":"test-region"}`
	require.NoError(t, settings.BackupTargetSet.Set(target))
	backupTarget, err := settings.DecodeBackupTarget(target)
	require.NoError(t, err)
	vmb := &harvesterv1.VirtualMachineBackup{
		TypeMeta: metav1.TypeMeta{Kind: "VirtualMachineBackup"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "backup", Namespace: "default", UID: "backup-uid",
			DeletionTimestamp: ptr.To(metav1.Now()),
		},
		Status: harvesterv1.VirtualMachineBackupStatus{
			VolumeBackups: []harvesterv1.VolumeBackup{{Name: ptr.To("volume")}},
			BackupTarget:  backuputil.BackupTargetStatus(backupTarget),
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
	credentials.Data[resticutil.PasswordKey] = []byte("stored-password")
	backupTarget.ThirdPartyBackupEnabled = true
	credentials.UID, credentials.ResourceVersion = "source-uid", "1"
	value, err := json.Marshal(backupTarget)
	require.NoError(t, err)
	record, err := backuputil.RepoReadyRecord(backupTarget, credentials)
	require.NoError(t, err)
	setting := &harvesterv1.Setting{ObjectMeta: metav1.ObjectMeta{
		Name:        settings.BackupTargetSettingName,
		Annotations: map[string]string{backuputil.ReadyAnnotation: record},
	}, Value: string(value)}
	harvesterv1.SettingConfigured.True(setting)
	clientset := fake.NewSimpleClientset(credentials, setting)
	clientset.PrependReactor("create", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		job := action.(clienttesting.CreateAction).GetObject().(*batchv1.Job)
		job.UID = uuid.NewUUID()
		return false, nil, nil
	})
	vmbo := backupcommon.NewVMBackupOperatorBuilder().Build()
	re := &ResticEngine{
		vmbo:         vmbo,
		vsHelper:     backupcommon.NewVolumeSnapshotHelper(nil, nil, nil, nil, vmbo, nil, nil),
		secretClient: fakeclients.SecretClient(clientset.CoreV1().Secrets),
		targets:      backuputil.TargetClient{Settings: fakeclients.HarvesterSettingClient(clientset.HarvesterhciV1beta1().Settings), Secrets: fakeclients.SecretClient(clientset.CoreV1().Secrets)},
		jobClient:    fakeclients.JobClient(clientset.BatchV1().Jobs),
		clientset:    fakeclients.KubernetesClient(clientset),
	}
	scheme := runtime.NewScheme()
	_ = snapshotv6.AddToScheme(scheme)
	re.imports = datamover.Snapshots{Client: dynamicfake.NewSimpleDynamicClient(scheme)}
	re.vsClient = fakeclients.VolumeSnapshotClient(clientset.SnapshotV1().VolumeSnapshots)

	return re, vmb, clientset
}

func forgetJob(t *testing.T, clientset *fake.Clientset, namespace, name string) *batchv1.Job {
	t.Helper()
	job, err := clientset.BatchV1().Jobs(namespace).Get(
		context.Background(), name, metav1.GetOptions{},
	)
	require.NoError(t, err)
	return job
}

func TestForgetJobRetainsCompletion(t *testing.T) {
	re, vmb, clientset := forgetFixture(t)
	jobName, err := re.forgetJobName(vmb, &vmb.Status.VolumeBackups[0])
	require.NoError(t, err)
	require.ErrorIs(t, re.forgetVolume(vmb, 0), engine.ErrRetryLater)
	job := forgetJob(t, clientset, joblifecycle.Namespace, jobName)
	require.Equal(t, int32(3), *job.Spec.BackoffLimit)
	require.Nil(t, job.Spec.TTLSecondsAfterFinished)
	require.Contains(t, job.Finalizers, joblifecycle.CompletionFinalizer)
	require.Empty(t, job.OwnerReferences)
	require.Equal(t, string(vmb.UID), job.Labels[joblifecycle.LabelParentUID])
	require.NotContains(t, job.Labels, "harvesterhci.io/vm-backup-name")
	require.NotContains(t, job.Labels, "harvesterhci.io/vm-backup-namespace")
	require.Equal(t, resticutil.ForgetCommand(`restic -o s3.region="$RESTIC_S3_REGION"`, []string{
		"ns=default", "vmb=backup", "sn=volume",
	}), job.Spec.Template.Spec.Containers[0].Args[0])

	job.Status.Succeeded = 1
	job.Status.Failed = 1
	_, err = clientset.BatchV1().Jobs(joblifecycle.Namespace).UpdateStatus(context.Background(), job, metav1.UpdateOptions{})
	require.NoError(t, err)
	clientset.ClearActions()
	require.NoError(t, re.forgetVolume(vmb, 0))
	for _, action := range clientset.Actions() {
		require.NotEqual(t, "create", action.GetVerb(), "completed volume cleanup must not be recreated")
		require.NotEqual(t, "delete", action.GetVerb(), "completion checkpoint must survive until VMBackup deletion")
	}

}

func TestForgetJobRetriesNextVolume(t *testing.T) {
	re, vmb, clientset := forgetFixture(t)
	vmb.Status.VolumeBackups = append(vmb.Status.VolumeBackups, harvesterv1.VolumeBackup{
		Name: ptr.To("data-volume"),
	})
	firstName, err := re.forgetJobName(vmb, &vmb.Status.VolumeBackups[0])
	require.NoError(t, err)
	dataName, err := re.forgetJobName(vmb, &vmb.Status.VolumeBackups[1])
	require.NoError(t, err)
	require.ErrorIs(t, re.forgetVolume(vmb, 0), engine.ErrRetryLater)
	first := forgetJob(t, clientset, joblifecycle.Namespace, firstName)
	first.Status.Succeeded = 1
	_, err = clientset.BatchV1().Jobs(joblifecycle.Namespace).UpdateStatus(
		context.Background(), first, metav1.UpdateOptions{},
	)
	require.NoError(t, err)
	require.ErrorIs(t, re.forgetVolume(vmb, 1), engine.ErrRetryLater)
	data := forgetJob(t, clientset, joblifecycle.Namespace, dataName)
	data.Status = batchv1.JobStatus{
		Failed: 4,
		Conditions: []batchv1.JobCondition{{
			Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
		}},
	}
	_, err = clientset.BatchV1().Jobs(joblifecycle.Namespace).UpdateStatus(
		context.Background(), data, metav1.UpdateOptions{},
	)
	require.NoError(t, err)
	err = re.forgetVolume(vmb, 1)
	require.Error(t, err)
	require.NotErrorIs(t, err, engine.ErrRetryLater)
	_, err = clientset.BatchV1().Jobs(joblifecycle.Namespace).Get(
		context.Background(), dataName, metav1.GetOptions{},
	)
	require.True(t, apierrors.IsNotFound(err))
	// Fake clients do not run garbage collection for the deleted Job.
	require.NoError(t, clientset.CoreV1().Secrets(joblifecycle.Namespace).Delete(
		context.Background(), jobcredentials.Name(dataName), metav1.DeleteOptions{},
	))
	require.ErrorIs(t, re.forgetVolume(vmb, 1), engine.ErrRetryLater)
	require.Zero(t, forgetJob(t, clientset, joblifecycle.Namespace, dataName).Status.Failed)
	require.NoError(t, re.forgetVolume(vmb, 0))
}

func TestForgetRepairsCredentials(t *testing.T) {
	re, vmb, client := forgetFixture(t)
	require.ErrorIs(t, re.forgetVolume(vmb, 0), engine.ErrRetryLater)
	secrets := client.CoreV1().Secrets(joblifecycle.Namespace)
	jobName, err := re.forgetJobName(vmb, &vmb.Status.VolumeBackups[0])
	require.NoError(t, err)
	name := jobcredentials.Name(jobName)
	original, err := secrets.Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, original.OwnerReferences, 1)
	require.Equal(t, "Job", original.OwnerReferences[0].Kind)
	require.NoError(t, secrets.Delete(context.Background(), name, metav1.DeleteOptions{}))

	// The existing pending Job must not bypass dependency repair.
	require.ErrorIs(t, re.forgetVolume(vmb, 0), engine.ErrRetryLater)
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
	require.NoError(t, re.forgetVolume(vmb, 0))
}

func TestJobsOwnCredentials(t *testing.T) {
	re, vmb, client := forgetFixture(t)
	vmb.DeletionTimestamp = nil
	vb := &vmb.Status.VolumeBackups[0]
	backupName, err := re.jobName(vmb, vb)
	require.NoError(t, err)
	checkName, err := re.checkJobName(vmb, vb)
	require.NoError(t, err)
	forgetName, err := re.forgetJobName(vmb, vb)
	require.NoError(t, err)
	require.NoError(t, re.createBackupJob(vmb, vb, "clone", backupName))
	require.NoError(t, re.createCheckJob(vmb, vb, checkName))
	require.NoError(t, re.createForgetJob(vmb, vb, forgetName))
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
		require.Len(t, refs, 3)
		for _, ref := range refs {
			require.Equal(t, secretName, ref)
		}
		secret, err := secrets.Get(context.Background(), secretName, metav1.GetOptions{})
		require.NoError(t, err)
		require.True(t, metav1.IsControlledBy(secret, job))
		require.NoError(t, secrets.Delete(context.Background(), secretName, metav1.DeleteOptions{}))
	}
	// Repeated reconciliation repairs dependencies for every Job type.
	require.ErrorIs(t, re.reconcileBackupJob(vmb, vb), engine.ErrRetryLater)
	_, err = re.checkRemoteSnapshot(vmb, vb)
	require.ErrorIs(t, err, engine.ErrRetryLater)
	require.ErrorIs(t, re.forgetVolume(vmb, 0), engine.ErrRetryLater)
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
			re, vmb, client := forgetFixture(t)
			ctx := context.Background()
			backups := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace)
			vmb.ResourceVersion = "1"
			_, err := backups.Create(ctx, vmb, metav1.CreateOptions{})
			require.NoError(t, err)
			re.vmbo = backupcommon.NewVMBackupOperatorBuilder().
				WithClient(fakeclients.VMBackupClient(client.HarvesterhciV1beta1().VirtualMachineBackups)).Build()
			require.ErrorIs(t, re.forgetVolume(vmb, 0), engine.ErrRetryLater)
			jobs := client.BatchV1().Jobs(joblifecycle.Namespace)
			jobName, err := re.forgetJobName(vmb, &vmb.Status.VolumeBackups[0])
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
			_, err = re.Remove(vmb, remoteDeleted)
			require.ErrorIs(t, err, persistErr)
			for _, action := range client.Actions() {
				require.NotEqual(t, "delete", action.GetVerb(), "retain Jobs until the checkpoint is durable")
				if action.GetResource().Resource == "jobs" {
					require.NotEqual(t, "update", action.GetVerb(), "retain completion protection until the checkpoint is durable")
				}
			}
			require.False(t, backupcommon.CleanupRecorded(re.vmbo, vmb), "leave the informer object unchanged")

			failUpdate = false
			deleteErr := errors.New("Job deletion failed")
			failDelete := true
			client.PrependReactor("delete", "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
				if failDelete {
					return true, nil, deleteErr
				}
				return false, nil, nil
			})
			updated, err := re.Remove(vmb, remoteDeleted)
			require.ErrorIs(t, err, deleteErr)
			require.Equal(t, "2", updated.ResourceVersion)
			stored, err := backups.Get(ctx, vmb.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, "true", stored.Annotations[annotation])

			// A restarted engine resumes local deletion without recreating remote work.
			restarted := *re
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
	re, vmb, client := forgetFixture(t)
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
	require.ErrorContains(t, re.reconcileBackupJob(vmb, vb), "failed")
	require.False(t, re.vmbo.GetVolBackupReadyToUse(vb))
}

func removeFixture(t *testing.T) (*ResticEngine, *harvesterv1.VirtualMachineBackup, *fake.Clientset) {
	t.Helper()
	re, vmb, client := forgetFixture(t)
	vmb.Status.VolumeBackups = append(vmb.Status.VolumeBackups, harvesterv1.VolumeBackup{Name: ptr.To("data")})
	_, err := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).Create(
		context.Background(), vmb, metav1.CreateOptions{},
	)
	require.NoError(t, err)
	re.vmbo = backupcommon.NewVMBackupOperatorBuilder().
		WithClient(fakeclients.VMBackupClient(client.HarvesterhciV1beta1().VirtualMachineBackups)).Build()
	for i := range vmb.Status.VolumeBackups {
		require.ErrorIs(t, re.forgetVolume(vmb, i), engine.ErrRetryLater)
	}
	return re, vmb, client
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
	re, vmb, client := removeFixture(t)
	first, err := re.forgetJobName(vmb, &vmb.Status.VolumeBackups[0])
	require.NoError(t, err)
	second, err := re.forgetJobName(vmb, &vmb.Status.VolumeBackups[1])
	require.NoError(t, err)
	job := completeForgetJob(t, client, joblifecycle.Namespace, first)
	// An external deletion must not erase this volume's completed result.
	job.DeletionTimestamp = ptr.To(metav1.Now())
	_, err = client.BatchV1().Jobs(joblifecycle.Namespace).Update(context.Background(), job, metav1.UpdateOptions{})
	require.NoError(t, err)
	client.ClearActions()
	_, err = re.Remove(vmb, true)
	require.ErrorIs(t, err, engine.ErrRetryLater)
	for _, action := range client.Actions() {
		require.NotEqual(t, "delete", action.GetVerb(), "retain the first result while another volume is pending")
		require.NotEqual(t, "update", action.GetVerb(), "do not release the first result early")
	}
	completeForgetJob(t, client, joblifecycle.Namespace, second)
	updated, err := re.Remove(vmb, true)
	require.NoError(t, err)
	require.Equal(t, "true", updated.Annotations[backupcommon.CompleteAnnotation])
	jobs, err := client.BatchV1().Jobs(joblifecycle.Namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, jobs.Items)
}

func TestRemoveRetriesPartialDeletion(t *testing.T) {
	re, vmb, client := removeFixture(t)
	for i := range vmb.Status.VolumeBackups {
		name, err := re.forgetJobName(vmb, &vmb.Status.VolumeBackups[i])
		require.NoError(t, err)
		completeForgetJob(t, client, joblifecycle.Namespace, name)
	}
	second, err := re.forgetJobName(vmb, &vmb.Status.VolumeBackups[1])
	require.NoError(t, err)
	deleteErr := errors.New("second Job deletion failed")
	failDelete := true
	client.PrependReactor("delete", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if failDelete && action.(clienttesting.DeleteAction).GetName() == second {
			return true, nil, deleteErr
		}
		return false, nil, nil
	})
	updated, err := re.Remove(vmb, true)
	require.ErrorIs(t, err, deleteErr)
	require.Equal(t, "true", updated.Annotations[backupcommon.CompleteAnnotation])
	remaining, err := client.BatchV1().Jobs(joblifecycle.Namespace).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, remaining.Items, 1)
	stored, err := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).Get(
		context.Background(), vmb.Name, metav1.GetOptions{},
	)
	require.NoError(t, err)
	restarted := *re
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
			re, vmb, client := removeFixture(t)
			require.NoError(t, settings.BackupTargetSet.Set(target))
			require.NoError(t, client.CoreV1().Secrets(util.LonghornSystemNamespaceName).Delete(
				context.Background(), util.BackupTargetSecretName, metav1.DeleteOptions{},
			))
			// Changed or disconnected targets must not need credentials or start remote work.
			restarted := *re
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

func TestJobsUseValidatedCredentials(t *testing.T) {
	t.Setenv("HARVESTER_IMAGE", "harvester:test")
	for _, operation := range []string{"backup", "check", "forget"} {
		t.Run(operation, func(t *testing.T) {
			re, vmb, client := forgetFixture(t)
			vmb.DeletionTimestamp = nil
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
			case "backup":
				err = re.createBackupJob(vmb, vb, "clone", "captured")
			case "check":
				err = re.createCheckJob(vmb, vb, "captured")
			case "forget":
				err = re.createForgetJob(vmb, vb, "captured")
			}
			require.NoError(t, err)
			job, err := re.jobClient.Get(joblifecycle.Namespace, "captured", metav1.GetOptions{})
			require.NoError(t, err)
			require.False(t, *job.Spec.Suspend)
			secret, err := re.secretClient.Get(job.Namespace, jobcredentials.Name(job.Name), metav1.GetOptions{})
			require.NoError(t, err)
			require.True(t, metav1.IsControlledBy(secret, job))
			require.Equal(t, map[string][]byte{
				util.AWSAccessKey: []byte("access"), util.AWSSecretKey: []byte("secret"),
				backuputil.ResticPasswordKey: []byte("stored-password"),
			}, secret.Data)
		})
	}
}

func TestInvalidCredentialsCreateNoJobs(t *testing.T) {
	t.Setenv("HARVESTER_IMAGE", "harvester:test")
	for _, operation := range []string{"backup", "check", "forget"} {
		for _, key := range []string{util.AWSAccessKey, util.AWSSecretKey, resticutil.PasswordKey, backuputil.RepositoryIDKey} {
			t.Run(operation+"/"+key, func(t *testing.T) {
				re, vmb, client := forgetFixture(t)
				source, err := client.CoreV1().Secrets(util.LonghornSystemNamespaceName).Get(context.Background(), util.BackupTargetSecretName, metav1.GetOptions{})
				require.NoError(t, err)
				delete(source.Data, key)
				_, err = client.CoreV1().Secrets(source.Namespace).Update(context.Background(), source, metav1.UpdateOptions{})
				require.NoError(t, err)
				vb := &vmb.Status.VolumeBackups[0]
				switch operation {
				case "backup":
					err = re.createBackupJob(vmb, vb, "clone", "invalid")
				case "check":
					err = re.createCheckJob(vmb, vb, "invalid")
				case "forget":
					err = re.createForgetJob(vmb, vb, "invalid")
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
	re, vmb, client := forgetFixture(t)
	vb := &vmb.Status.VolumeBackups[0]
	require.NoError(t, re.createCheckJob(vmb, vb, "before"))
	old, err := client.CoreV1().Secrets(joblifecycle.Namespace).Get(context.Background(), jobcredentials.Name("before"), metav1.GetOptions{})
	require.NoError(t, err)
	source, err := client.CoreV1().Secrets(util.LonghornSystemNamespaceName).Get(context.Background(), util.BackupTargetSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	source.Data[util.AWSAccessKey] = []byte("rotated-access")
	source.Data[util.AWSSecretKey] = []byte("rotated-secret")
	_, err = client.CoreV1().Secrets(source.Namespace).Update(context.Background(), source, metav1.UpdateOptions{})
	require.NoError(t, err)
	job, err := re.jobClient.Get(joblifecycle.Namespace, "before", metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, re.ensureSecret(vmb, job))
	kept, err := client.CoreV1().Secrets(joblifecycle.Namespace).Get(context.Background(), old.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, old.Data, kept.Data)
	require.NoError(t, re.createCheckJob(vmb, vb, "after"))
	current, err := client.CoreV1().Secrets(joblifecycle.Namespace).Get(context.Background(), jobcredentials.Name("after"), metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, []byte("rotated-access"), current.Data[util.AWSAccessKey])
	require.Equal(t, old.Data[resticutil.PasswordKey], current.Data[resticutil.PasswordKey])
	require.False(t, *job.Spec.Template.Spec.AutomountServiceAccountToken)
	for _, env := range job.Spec.Template.Spec.Containers[0].Env {
		if env.Name == util.AWSAccessKey || env.Name == util.AWSSecretKey || env.Name == resticutil.PasswordKey {
			require.Empty(t, env.Value)
			require.NotNil(t, env.ValueFrom.SecretKeyRef)
		}
	}
}
