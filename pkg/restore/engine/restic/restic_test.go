package restic

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	"github.com/harvester/harvester/pkg/restore/engine"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCompletionWaitsForPersistence(t *testing.T) {
	vmro := restorecommon.NewVMRestoreOperatorBuilder().Build()
	re := &ResticRestoreEngine{vmro: vmro}
	vr := &harvesterv1.VolumeRestore{}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "default"},
		Status:     batchv1.JobStatus{Succeeded: 1},
	}

	err := re.syncFromJob(vr, job)
	require.ErrorIs(t, err, engine.ErrRetryLater)
	require.Equal(t, 100, vr.Progress)
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
		ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "default"},
		Status: harvesterv1.VirtualMachineRestoreStatus{
			VolumeRestores: []harvesterv1.VolumeRestore{{VolumeName: "volume", Progress: 100}},
		},
	}
	re := &ResticRestoreEngine{vmro: vmro}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:       re.jobName(vmr, &vmr.Status.VolumeRestores[0]),
			Namespace:  vmr.Namespace,
			Finalizers: []string{engine.RestoreJobCompletionFinalizer},
		},
		Status: batchv1.JobStatus{Succeeded: 1},
	}
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

func TestPersistedProgressDoesNotCompleteRunningJob(t *testing.T) {
	vmro := restorecommon.NewVMRestoreOperatorBuilder().Build()
	vmr := &harvesterv1.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{Name: "restore", Namespace: "default"},
		Status: harvesterv1.VirtualMachineRestoreStatus{
			VolumeRestores: []harvesterv1.VolumeRestore{{VolumeName: "volume", Progress: 100}},
		},
	}
	re := &ResticRestoreEngine{vmro: vmro}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:       re.jobName(vmr, &vmr.Status.VolumeRestores[0]),
			Namespace:  vmr.Namespace,
			Finalizers: []string{engine.RestoreJobCompletionFinalizer},
		},
		Status: batchv1.JobStatus{Active: 1},
	}
	clientset := fake.NewSimpleClientset(job)
	re.jobClient = fakeclients.JobClient(clientset.BatchV1().Jobs)

	require.ErrorIs(t, re.Reconcile(vmr, nil, 0), engine.ErrRetryLater)
	updated, err := clientset.BatchV1().Jobs(job.Namespace).Get(context.Background(), job.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, job.Finalizers, updated.Finalizers)
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
		re.credentialsName(vmr),
		re.jobName(vmr, vr),
		re.checkJobName(vmr, vr),
	} {
		require.LessOrEqual(t, len(generatedName), 63)
	}

	other := &harvesterv1.VolumeRestore{VolumeName: "volume-b"}
	require.NotEqual(t, re.jobName(vmr, vr), re.jobName(vmr, other))
}
