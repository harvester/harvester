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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	"github.com/harvester/harvester/pkg/restore/engine"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/datamover"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/util/joblifecycle"
)

func TestCompletionWaitsForHandoff(t *testing.T) {
	re, vmb, vmr, client := credentialFixture(t)
	vr, vb := &vmr.Status.VolumeRestores[0], &vmb.Status.VolumeBackups[0]
	job := completedTransfer(t, re, vmb, vmr, client)
	require.ErrorIs(t, re.syncFromJob(vmr, vr, vb, job), datamover.ErrPending)
	require.Zero(t, vr.Progress)
	pv, err := client.CoreV1().PersistentVolumes().Get(context.Background(), "restore-pv", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, corev1.PersistentVolumeReclaimRetain, pv.Spec.PersistentVolumeReclaimPolicy)
	require.Equal(t, "copied", pv.Annotations[datamover.TransferState])
}

func TestRunningJobWaitsWithoutChangingProgress(t *testing.T) {
	vmro := restorecommon.NewVMRestoreOperatorBuilder().Build()
	re := &ResticRestoreEngine{vmro: vmro}
	vr := &harvesterv1.VolumeRestore{Progress: 30}
	job := &batchv1.Job{Status: batchv1.JobStatus{Active: 1}}

	require.ErrorIs(t, re.syncFromJob(nil, vr, nil, job), engine.ErrRetryLater)
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
			Namespace:  joblifecycle.Namespace,
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

func TestCompletionPreservesAPIErrors(t *testing.T) {
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
					Namespace:  joblifecycle.Namespace,
					Finalizers: []string{joblifecycle.CompletionFinalizer},
					UID:        "job-uid",
				},
				Status: batchv1.JobStatus{Succeeded: 1},
			}
			joblifecycle.Track(&job.ObjectMeta, vmr, vmRestoreKind)
			clientset := fake.NewSimpleClientset(job)
			apiErr := apierrors.NewServiceUnavailable("API unavailable")
			clientset.PrependReactor(verb, "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
				return true, nil, apiErr
			})
			re.jobClient = fakeclients.JobClient(clientset.BatchV1().Jobs)

			require.ErrorIs(t, re.Reconcile(vmr, nil, 0), apiErr)
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

	require.ErrorIs(t, re.ensurePVC(vmr, vr, vb), engine.ErrRetryLater)
	pvc, err := pvcs(joblifecycle.Namespace).Get(
		context.Background(),
		re.stagingName(vmr, vr),
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
	require.ErrorContains(t, re.syncFromJob(nil, vr, nil, job), "failed")
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
						Name: name, Namespace: joblifecycle.Namespace,
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
				require.NotErrorIs(t, err, engine.ErrRetryLater)
				for _, action := range client.Actions() {
					require.Equal(t, "get", action.GetVerb(), "a foreign Job must not be changed")
				}
			})
		}
	}
}

func TestJobLookupPreservesAPIErrors(t *testing.T) {
	re, vmb, vmr, client := credentialFixture(t)
	apiErr := apierrors.NewTooManyRequests("API throttled", 1)
	client.PrependReactor("get", "jobs", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apiErr
	})
	_, err := re.ensureRestoreJob(vmr, vmb, &vmr.Status.VolumeRestores[0], &vmb.Status.VolumeBackups[0])
	require.ErrorIs(t, err, apiErr)
	require.NotErrorIs(t, err, engine.ErrRetryLater)
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
					Name: name, Namespace: joblifecycle.Namespace, Finalizers: []string{joblifecycle.CompletionFinalizer},
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
			jobs, err := client.BatchV1().Jobs(joblifecycle.Namespace).List(context.Background(), metav1.ListOptions{})
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
	re, vmb, vmr, client := credentialFixture(t)
	vr, vb := &vmr.Status.VolumeRestores[0], &vmb.Status.VolumeBackups[0]
	job := completedTransfer(t, re, vmb, vmr, client)
	require.ErrorIs(t, re.syncFromJob(vmr, vr, vb, job), datamover.ErrPending)
	// Each subsequent reconcile can reconstruct the handoff without Job results or credentials.
	require.NoError(t, client.CoreV1().Secrets(util.LonghornSystemNamespaceName).Delete(context.Background(), util.BackupTargetSecretName, metav1.DeleteOptions{}))
	for i := 0; i < 10 && vr.Progress != 100; i++ {
		err := re.Reconcile(vmr, vmb, 0)
		if err != nil {
			require.True(t, errors.Is(err, engine.ErrRetryLater) || errors.Is(err, datamover.ErrPending))
		}
		pv, err := client.CoreV1().PersistentVolumes().Get(context.Background(), "restore-pv", metav1.GetOptions{})
		require.NoError(t, err)
		if pv.Spec.ClaimRef.Namespace == vmr.Namespace {
			pvc, err := client.CoreV1().PersistentVolumeClaims(vmr.Namespace).Get(context.Background(), vr.PersistentVolumeClaim.ObjectMeta.Name, metav1.GetOptions{})
			require.NoError(t, err)
			pvc.Status.Phase = corev1.ClaimBound
			_, err = client.CoreV1().PersistentVolumeClaims(vmr.Namespace).UpdateStatus(context.Background(), pvc, metav1.UpdateOptions{})
			require.NoError(t, err)
		}
	}
	require.Equal(t, 100, vr.Progress)
	require.NoError(t, re.Reconcile(vmr, nil, 0))
}

func TestDeletionRetries(t *testing.T) {
	re, _, vmr, client := credentialFixture(t)
	vmr.Annotations = map[string]string{restorecommon.BackupTypeAnnotation: "restic"}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: "running", Namespace: joblifecycle.Namespace, Finalizers: []string{joblifecycle.CompletionFinalizer},
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
	restarted := &ResticRestoreEngine{vmro: re.vmro, jobClient: re.jobClient, clientset: re.clientset}
	fail = false
	require.NoError(t, restarted.Delete(vmr))
	for _, action := range client.Actions() {
		require.NotEqual(t, "virtualmachinerestores", action.GetResource().Resource)
		require.NotEqual(t, "create", action.GetVerb())
	}
}

func completedTransfer(t *testing.T, e *ResticRestoreEngine, vmb *harvesterv1.VirtualMachineBackup, vmr *harvesterv1.VirtualMachineRestore, client *fake.Clientset) *batchv1.Job {
	t.Helper()
	vr := &vmr.Status.VolumeRestores[0]
	stage := e.destinationPVC(vmr, vr, &vmb.Status.VolumeBackups[0])
	stage.Namespace, stage.Name, stage.UID = joblifecycle.Namespace, e.stagingName(vmr, vr), "stage-uid"
	stage.Spec.VolumeName = "restore-pv"
	stage.Status.Phase = corev1.ClaimBound
	joblifecycle.Track(&stage.ObjectMeta, vmr, vmRestoreKind)
	_, err := client.CoreV1().PersistentVolumeClaims(stage.Namespace).Create(context.Background(), stage, metav1.CreateOptions{})
	require.NoError(t, err)
	pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "restore-pv", UID: "pv-uid"}, Spec: corev1.PersistentVolumeSpec{PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
		ClaimRef: &corev1.ObjectReference{Name: stage.Name, Namespace: stage.Namespace, UID: stage.UID},
	}, Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound}}
	_, err = client.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)
	client.PrependReactor("create", "persistentvolumeclaims", func(action clienttesting.Action) (bool, runtime.Object, error) {
		action.(clienttesting.CreateAction).GetObject().(*corev1.PersistentVolumeClaim).UID = "destination-uid"
		return false, nil, nil
	})
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: e.jobName(vmr, vr), Namespace: joblifecycle.Namespace}, Status: batchv1.JobStatus{Succeeded: 1}}
	joblifecycle.Track(&job.ObjectMeta, vmr, vmRestoreKind)
	job, err = e.jobClient.Create(job)
	require.NoError(t, err)
	return job
}
