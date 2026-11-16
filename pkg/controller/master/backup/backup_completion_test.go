package backup

import (
	"context"
	"testing"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/backup/engine"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

type completionEngine struct {
	preparingEngine
	reconcile    func(*harvesterv1.VirtualMachineBackup, int) error
	cleanupCalls int
}

func (e *completionEngine) Reconcile(
	vmb *harvesterv1.VirtualMachineBackup, index int,
	_ map[string]snapshotv1.VolumeSnapshotClass,
) error {
	return e.reconcile(vmb, index)
}

func (e *completionEngine) Cleanup(*harvesterv1.VirtualMachineBackup) error {
	e.cleanupCalls++
	return nil
}

func (e *completionEngine) UpdateProgress(
	_ *harvesterv1.VirtualMachineBackup, vb *harvesterv1.VolumeBackup,
) (int64, error) {
	return int64(vb.Progress), nil
}

type completionOperator struct{ common.VMBackupOperator }

func (o completionOperator) BuildCSIDriverMap(*harvesterv1.VirtualMachineBackup) (
	map[string]string, map[string]snapshotv1.VolumeSnapshotClass, error,
) {
	return nil, nil, nil
}

func completionFixture(t *testing.T, backupType harvesterv1.BackupType) (
	*Handler, *completionEngine, *harvesterv1.VirtualMachineBackup, *fake.Clientset,
) {
	t.Helper()
	vmb := &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "default", UID: "backup-uid"},
		Spec:       harvesterv1.VirtualMachineBackupSpec{Type: backupType},
		Status: harvesterv1.VirtualMachineBackupStatus{
			SourceSpec:                        &harvesterv1.VirtualMachineSourceSpec{},
			BackupTarget:                      &harvesterv1.BackupTarget{Type: "s3"},
			CSIDriverVolumeSnapshotClassNames: map[string]string{"driver": "snapshots"},
			VolumeBackups: []harvesterv1.VolumeBackup{
				{Name: ptr.To("disk"), VolumeSize: 100},
			},
		},
	}
	client := fake.NewSimpleClientset(vmb)
	operator := common.NewVMBackupOperatorBuilder().
		WithClient(fakeclients.VMBackupClient(client.HarvesterhciV1beta1().VirtualMachineBackups)).Build()
	backupEngine := &completionEngine{preparingEngine: preparingEngine{prepared: true}}
	handler := &Handler{
		vmbo: completionOperator{operator}, vmbController: &requeueBackupController{},
		engines: map[harvesterv1.BackupType]engine.BackupEngine{backupType: backupEngine},
	}
	return handler, backupEngine, vmb, client
}

func TestBackupWaitsForCleanup(t *testing.T) {
	for _, backupType := range []harvesterv1.BackupType{harvesterv1.Restic, harvesterv1.Kopia} {
		t.Run(string(backupType), func(t *testing.T) {
			h, e, vmb, client := completionFixture(t, backupType)
			cleanupPending := true
			e.reconcile = func(vmb *harvesterv1.VirtualMachineBackup, index int) error {
				vb := &vmb.Status.VolumeBackups[index]
				if !ptr.Deref(vb.ReadyToUse, false) {
					vb.ReadyToUse = ptr.To(true)
					vb.Progress = 100
					return nil
				}
				if cleanupPending {
					return engine.ErrRetryLater
				}
				return nil
			}
			// Persist the upload checkpoint, then retry asynchronous cleanup.
			for pass := 0; pass < 3; pass++ {
				before := vmb.DeepCopy()
				_, err := h.OnBackupChange("", vmb)
				require.NoError(t, err)
				require.Equal(t, before, vmb, "informer objects must not be mutated")
				vmb, err = client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).
					Get(context.Background(), vmb.Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.True(t, *vmb.Status.VolumeBackups[0].ReadyToUse)
				require.False(t, *vmb.Status.ReadyToUse)
				require.Equal(t, 100, vmb.Status.Progress)
			}
			cleanupPending = false
			_, err := h.OnBackupChange("", vmb)
			require.NoError(t, err)
			vmb, err = client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).
				Get(context.Background(), vmb.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.True(t, *vmb.Status.ReadyToUse)
			require.Len(t, h.vmbController.(*requeueBackupController).enqueues, 3)
		})
	}
}

func TestBackupPersistsCheckpointWhileAnotherVolumeWaits(t *testing.T) {
	for _, backupType := range []harvesterv1.BackupType{harvesterv1.Restic, harvesterv1.Kopia} {
		t.Run(string(backupType), func(t *testing.T) {
			h, e, vmb, client := completionFixture(t, backupType)
			vmb.Status.VolumeBackups = append(vmb.Status.VolumeBackups,
				harvesterv1.VolumeBackup{Name: ptr.To("data"), VolumeSize: 100})
			e.reconcile = func(vmb *harvesterv1.VirtualMachineBackup, index int) error {
				if index == 1 {
					vmb.Status.VolumeBackups[index].Progress = 50
					return engine.ErrRetryLater
				}
				vmb.Status.VolumeBackups[index].ReadyToUse = ptr.To(true)
				vmb.Status.VolumeBackups[index].Progress = 100
				return nil
			}
			_, err := h.OnBackupChange("", vmb)
			require.NoError(t, err)
			current, err := client.HarvesterhciV1beta1().VirtualMachineBackups(vmb.Namespace).
				Get(context.Background(), vmb.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.True(t, *current.Status.VolumeBackups[0].ReadyToUse)
			require.False(t, *current.Status.ReadyToUse)
			require.Equal(t, 75, current.Status.Progress)
		})
	}
}

func TestReadyBackupSkipsCleanup(t *testing.T) {
	oldTarget := settings.BackupTargetSet.Get()
	t.Cleanup(func() { require.NoError(t, settings.BackupTargetSet.Set(oldTarget)) })
	require.NoError(t, settings.BackupTargetSet.Set(settings.BackupTargetSet.GetDefault()))
	for _, backupType := range []harvesterv1.BackupType{harvesterv1.Restic, harvesterv1.Kopia} {
		t.Run(string(backupType), func(t *testing.T) {
			h, e, vmb, _ := completionFixture(t, backupType)
			vmb.Status.ReadyToUse = ptr.To(true)
			vmb.Status.VolumeBackups[0].ReadyToUse = ptr.To(true)
			e.reconcile = func(*harvesterv1.VirtualMachineBackup, int) error {
				t.Fatal("a ready backup must not restart an upload")
				return nil
			}
			_, err := h.OnBackupChange("", vmb)
			require.NoError(t, err)
			require.Zero(t, e.cleanupCalls)
			require.Empty(t, h.vmbController.(*requeueBackupController).enqueues)
		})
	}
}
