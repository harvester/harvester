package restic

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	"github.com/harvester/harvester/pkg/restore/engine"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

func TestCompletionWaitsForPersistence(t *testing.T) {
	re, _, vmr, client := credentialFixture(t)
	current := vmr.DeepCopy()
	vr := &current.Status.VolumeRestores[0]
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: re.jobName(vmr, vr), Namespace: vmr.Namespace},
		Status:     batchv1.JobStatus{Succeeded: 1},
	}
	require.ErrorIs(t, re.syncFromJob(vr, job), engine.ErrRetryLater)
	require.Equal(t, 100, vr.Progress)
	require.Empty(t, client.Actions(), "the controller must persist progress before release")
	require.Zero(t, vmr.Status.VolumeRestores[0].Progress, "the informer object remains unchanged")
}

func TestRunningJobWaitsWithoutChangingProgress(t *testing.T) {
	vmro := restorecommon.NewVMRestoreOperatorBuilder().Build()
	re := &ResticRestoreEngine{vmro: vmro}
	vr := &harvesterv1.VolumeRestore{Progress: 30}
	job := &batchv1.Job{Status: batchv1.JobStatus{Active: 1}}

	require.ErrorIs(t, re.syncFromJob(vr, job), engine.ErrRetryLater)
	require.Equal(t, 30, vr.Progress)
}

func TestPersistedCompletionReleasesJob(t *testing.T) {
	vmro := restorecommon.NewVMRestoreOperatorBuilder().Build()
	vmr := &harvesterv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "default", UID: "restore-uid"},
		Status: harvesterv1.VirtualMachineRestoreStatus{
			VolumeRestores: []harvesterv1.VolumeRestore{{VolumeName: "volume", Progress: 100}},
		},
	}
	re := &ResticRestoreEngine{vmro: vmro}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:       re.jobName(vmr, &vmr.Status.VolumeRestores[0]),
			Namespace:  vmr.Namespace,
			Finalizers: []string{joblifecycle.CompletionFinalizer},
			UID:        "job-uid",
		},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	joblifecycle.Track(&job.ObjectMeta, vmr, vmRestoreKind)
	clientset := fake.NewSimpleClientset(job)
	re.jobClient = fakeclients.JobClient(clientset.BatchV1().Jobs)

	require.NoError(t, re.Reconcile(vmr, nil, 0))
	updated, err := clientset.BatchV1().Jobs(job.Namespace).Get(
		context.Background(),
		job.Name,
		metav1.GetOptions{},
	)
	require.NoError(t, err)
	require.Empty(t, updated.Finalizers)
}

func TestCompletionRetriesAPIErrors(t *testing.T) {
	for _, verb := range []string{"get", "update"} {
		t.Run(verb, func(t *testing.T) {
			vmro := restorecommon.NewVMRestoreOperatorBuilder().Build()
			vmr := &harvesterv1.VirtualMachineRestore{
				ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "default", UID: "restore-uid"},
				Status: harvesterv1.VirtualMachineRestoreStatus{
					VolumeRestores: []harvesterv1.VolumeRestore{{VolumeName: "volume", Progress: 100}},
				},
			}
			re := &ResticRestoreEngine{vmro: vmro}
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:       re.jobName(vmr, &vmr.Status.VolumeRestores[0]),
					Namespace:  vmr.Namespace,
					Finalizers: []string{joblifecycle.CompletionFinalizer},
					UID:        "job-uid",
				},
				Status: batchv1.JobStatus{Succeeded: 1},
			}
			joblifecycle.Track(&job.ObjectMeta, vmr, vmRestoreKind)
			clientset := fake.NewSimpleClientset(job)
			apiErr := errors.New("API unavailable")
			clientset.PrependReactor(verb, "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, apiErr
			})
			re.jobClient = fakeclients.JobClient(clientset.BatchV1().Jobs)

			require.ErrorIs(t, re.Reconcile(vmr, nil, 0), engine.ErrRetryLater)
			obj, err := clientset.Tracker().Get(batchv1.SchemeGroupVersion.WithResource("jobs"), job.Namespace, job.Name)
			require.NoError(t, err)
			require.Equal(t, job.Finalizers, obj.(*batchv1.Job).Finalizers)
		})
	}
}

func TestRestoredPVCHasNoOwnerReferences(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	pvcs := clientset.CoreV1().PersistentVolumeClaims
	re := &ResticRestoreEngine{
		vmbo:      backupcommon.NewVMBackupOperatorBuilder().Build(),
		vmro:      restorecommon.NewVMRestoreOperatorBuilder().Build(),
		pvcCache:  fakeclients.PersistentVolumeClaimCache(pvcs),
		pvcClient: fakeclients.PersistentVolumeClaimClient(pvcs),
	}
	vmr := &harvesterv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "default"},
	}
	vr := &harvesterv1.VolumeRestore{
		PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
			ObjectMeta: metav1.ObjectMeta{Name: "restored-volume"},
		},
	}
	vb := &harvesterv1.VolumeBackup{
		PersistentVolumeClaim: harvesterv1.PersistentVolumeClaimSourceSpec{
			Spec: corev1.PersistentVolumeClaimSpec{},
		},
	}

	require.NoError(t, re.ensurePVC(vmr, vr, vb))
	pvc, err := pvcs(vmr.Namespace).Get(
		context.Background(),
		vr.PersistentVolumeClaim.ObjectMeta.Name,
		metav1.GetOptions{},
	)
	require.NoError(t, err)
	require.Empty(t, pvc.OwnerReferences)
}

func TestGeneratedNamesAreBounded(t *testing.T) {
	longName := strings.Repeat("restore", 20)
	vmr := &harvesterv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{Name: longName, Namespace: "default"},
	}
	vr := &harvesterv1.VolumeRestore{VolumeName: "volume-a"}
	re := &ResticRestoreEngine{
		vmro: restorecommon.NewVMRestoreOperatorBuilder().Build(),
	}

	for _, generatedName := range []string{
		re.jobName(vmr, vr),
		re.checkJobName(vmr, vr),
	} {
		require.LessOrEqual(t, len(generatedName), 63)
	}

	other := &harvesterv1.VolumeRestore{VolumeName: "volume-b"}
	require.NotEqual(t, re.jobName(vmr, vr), re.jobName(vmr, other))
}

func TestFailedJobDoesNotCompleteRestore(t *testing.T) {
	vmro := restorecommon.NewVMRestoreOperatorBuilder().Build()
	re := &ResticRestoreEngine{vmro: vmro}
	vr := &harvesterv1.VolumeRestore{Progress: 30}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "restore", Namespace: "default", UID: "job-uid",
			Finalizers: []string{joblifecycle.CompletionFinalizer},
		},
		Status: batchv1.JobStatus{Failed: 1},
	}
	clientset := fake.NewSimpleClientset(job)
	re.jobClient = fakeclients.JobClient(clientset.BatchV1().Jobs)
	require.ErrorContains(t, re.syncFromJob(vr, job), "failed")
	require.Equal(t, 30, vr.Progress)
	require.Empty(t, clientset.Actions(), "failure must be persisted before releasing the Job")
	require.Contains(t, job.Finalizers, joblifecycle.CompletionFinalizer)
}

func TestJobIdentity(t *testing.T) {
	for _, operation := range []string{"reuse", "check", "release"} {
		for _, valid := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/valid=%t", operation, valid), func(t *testing.T) {
				re, vmb, vmr, client := credentialFixture(t)
				vr, vb := &vmr.Status.VolumeRestores[0], &vmb.Status.VolumeBackups[0]
				name := re.jobName(vmr, vr)
				if operation == "check" {
					name = re.checkJobName(vmr, vr)
				}
				job := &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name: name, Namespace: vmr.Namespace,
						Finalizers: []string{joblifecycle.CompletionFinalizer},
					},
					Status: batchv1.JobStatus{Succeeded: 1},
				}
				joblifecycle.Track(&job.ObjectMeta, vmr, vmRestoreKind)
				if !valid {
					job.Labels[joblifecycle.LabelParentUID] = "previous-restore-uid"
				}
				_, err := re.jobClient.Create(job)
				require.NoError(t, err)
				client.ClearActions()
				switch operation {
				case "reuse":
					_, err = re.ensureRestoreJob(vmr, vmb, vr, vb)
				case "check":
					err = re.checkRemoteSnapshot(vmr, vmb, vr, vb)
				case "release":
					vr.Progress = 100
					err = re.Reconcile(vmr, nil, 0)
				}
				if valid {
					require.NoError(t, err)
					return
				}
				require.Error(t, err)
				for _, action := range client.Actions() {
					require.Equal(t, "get", action.GetVerb(), "a foreign Job must not be changed")
				}
			})
		}
	}
}

func TestJobLookupRetriesAPIErrors(t *testing.T) {
	re, vmb, vmr, client := credentialFixture(t)
	apiErr := errors.New("API unavailable")
	client.PrependReactor("get", "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apiErr
	})
	_, err := re.ensureRestoreJob(vmr, vmb, &vmr.Status.VolumeRestores[0], &vmb.Status.VolumeBackups[0])
	require.ErrorIs(t, err, engine.ErrRetryLater)
	require.ErrorIs(t, err, apiErr)
	require.Len(t, client.Actions(), 1, "retry without creating another Job")
}

func TestDeleteJobs(t *testing.T) {
	for _, typed := range []bool{false, true} {
		t.Run(fmt.Sprintf("typed=%t", typed), func(t *testing.T) {
			re, _, vmr, client := credentialFixture(t)
			if typed {
				vmr.Annotations = map[string]string{restorecommon.BackupTypeAnnotation: "restic"}
			}
			for _, name := range []string{"check", "running", "finished", "foreign"} {
				job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
					Name: name, Namespace: vmr.Namespace, Finalizers: []string{joblifecycle.CompletionFinalizer},
				}}
				joblifecycle.Track(&job.ObjectMeta, vmr, vmRestoreKind)
				if name == "foreign" {
					job.Labels[joblifecycle.LabelParentUID] = "previous-restore-uid"
				}
				_, err := re.jobClient.Create(job)
				require.NoError(t, err)
			}
			// Cleanup uses the saved engine and UID even without source metadata or volume status.
			vmr.Status = harvesterv1.VirtualMachineRestoreStatus{}
			require.NoError(t, re.Delete(vmr))
			require.NoError(t, re.Delete(vmr))
			jobs, err := client.BatchV1().Jobs(vmr.Namespace).List(context.Background(), metav1.ListOptions{})
			require.NoError(t, err)
			if !typed {
				require.Len(t, jobs.Items, 4, "legacy restores must not invoke a new engine's cleanup")
				return
			}
			require.Len(t, jobs.Items, 1)
			require.Equal(t, "foreign", jobs.Items[0].Name)
		})
	}
}

func TestCompletionAfterRestart(t *testing.T) {
	re, _, vmr, client := credentialFixture(t)
	vr := &vmr.Status.VolumeRestores[0]
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: re.jobName(vmr, vr), Namespace: vmr.Namespace,
			Finalizers: []string{joblifecycle.CompletionFinalizer},
		},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
	joblifecycle.Track(&job.ObjectMeta, vmr, vmRestoreKind)
	job, err := re.jobClient.Create(job)
	require.NoError(t, err)
	current := vmr.DeepCopy()
	require.ErrorIs(t, re.syncFromJob(&current.Status.VolumeRestores[0], job), engine.ErrRetryLater)
	// The controller persists completion before a later reconcile releases the Job.
	stored, err := client.HarvesterhciV1beta1().VirtualMachineRestores(vmr.Namespace).Update(
		context.Background(), current, metav1.UpdateOptions{},
	)
	require.NoError(t, err)
	restarted := &ResticRestoreEngine{vmro: re.vmro, jobClient: re.jobClient}
	require.NoError(t, restarted.Reconcile(stored, nil, 0))
	released, err := re.jobClient.Get(job.Namespace, job.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Empty(t, released.Finalizers)
	// Simulate TTL collection: a subsequent retry must not recreate the Job.
	require.NoError(t, re.jobClient.Delete(job.Namespace, job.Name, &metav1.DeleteOptions{}))
	client.ClearActions()
	require.NoError(t, restarted.Reconcile(stored, nil, 0))
	for _, action := range client.Actions() {
		require.Equal(t, "get", action.GetVerb())
	}
}

func TestDeletionRetries(t *testing.T) {
	re, _, vmr, client := credentialFixture(t)
	vmr.Annotations = map[string]string{restorecommon.BackupTypeAnnotation: "restic"}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: "running", Namespace: vmr.Namespace, Finalizers: []string{joblifecycle.CompletionFinalizer},
	}}
	joblifecycle.Track(&job.ObjectMeta, vmr, vmRestoreKind)
	_, err := re.jobClient.Create(job)
	require.NoError(t, err)
	deleteErr := errors.New("Job deletion failed")
	fail := true
	client.PrependReactor("delete", "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
		if fail {
			return true, nil, deleteErr
		}
		return false, nil, nil
	})
	client.ClearActions()
	require.ErrorIs(t, re.Delete(vmr), deleteErr)
	restarted := &ResticRestoreEngine{vmro: re.vmro, jobClient: re.jobClient}
	fail = false
	require.NoError(t, restarted.Delete(vmr))
	for _, action := range client.Actions() {
		require.NotEqual(t, "virtualmachinerestores", action.GetResource().Resource)
		require.NotEqual(t, "create", action.GetVerb())
	}
}
