package backup

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/backup/engine"
	"github.com/harvester/harvester/pkg/settings"
)

type cleaningEngine struct {
	preparingEngine
	removeCalls  int
	removeRemote bool
	removeErr    error
}

func (e *cleaningEngine) Remove(
	vmb *harvesterv1.VirtualMachineBackup,
	removeRemote bool,
) (*harvesterv1.VirtualMachineBackup, error) {
	e.removeCalls++
	e.removeRemote = removeRemote
	updated := vmb.DeepCopy()
	updated.ResourceVersion = "updated"
	return updated, e.removeErr
}

func cleanupFixture(
	t *testing.T,
	backupType harvesterv1.BackupType,
) (*Handler, *cleaningEngine, *harvesterv1.VirtualMachineBackup) {
	t.Helper()
	old := settings.BackupTargetSet.Get()
	t.Cleanup(func() { require.NoError(t, settings.BackupTargetSet.Set(old)) })
	require.NoError(t, settings.BackupTargetSet.Set(`{"type":"s3","bucketName":"new"}`))
	vmb := &harvesterv1.VirtualMachineBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "backup", Namespace: "default", UID: "backup-uid",
			DeletionTimestamp: ptr.To(metav1.Now()),
			Finalizers:        []string{"wrangler.cattle.io/harvester-vm-backup-controller"},
		},
		Spec: harvesterv1.VirtualMachineBackupSpec{Type: backupType},
		Status: harvesterv1.VirtualMachineBackupStatus{
			VolumeBackups: []harvesterv1.VolumeBackup{{Name: ptr.To("os")}, {Name: ptr.To("data")}},
			BackupTarget:  &harvesterv1.BackupTarget{Type: "s3", BucketName: "old"},
		},
	}
	cleaner := &cleaningEngine{}
	handler := &Handler{
		vmbo:    common.NewVMBackupOperatorBuilder().Build(),
		engines: map[harvesterv1.BackupType]engine.BackupEngine{backupType: cleaner},
	}
	return handler, cleaner, vmb
}

func TestRemoveDelegatesToEngine(t *testing.T) {
	for _, backupType := range []harvesterv1.BackupType{
		harvesterv1.Snapshot, harvesterv1.Backup, harvesterv1.Restic, harvesterv1.Kopia,
	} {
		t.Run(string(backupType), func(t *testing.T) {
			handler, remover, vmb := cleanupFixture(t, backupType)
			remover.removeErr = errors.New("removal failed")
			updated, err := handler.OnBackupRemove("", vmb)
			require.ErrorIs(t, err, remover.removeErr)
			require.Equal(t, 1, remover.removeCalls)
			require.True(t, remover.removeRemote, "the backup target changed")
			require.Equal(t, "updated", updated.ResourceVersion)
			require.NotEmpty(t, updated.Finalizers)
		})
	}
}

func TestRemoveWithoutTarget(t *testing.T) {
	for _, backupType := range []harvesterv1.BackupType{
		harvesterv1.Snapshot, harvesterv1.Backup, harvesterv1.Restic, harvesterv1.Kopia,
	} {
		t.Run(string(backupType), func(t *testing.T) {
			handler, remover, vmb := cleanupFixture(t, backupType)
			require.NoError(t, settings.BackupTargetSet.Set(settings.BackupTargetSet.GetDefault()))
			remover.removeErr = engine.ErrRetryLater
			_, err := handler.OnBackupRemove("", vmb)
			require.ErrorIs(t, err, engine.ErrRetryLater)
			require.False(t, remover.removeRemote)
			remover.removeErr = nil
			vmb.Status.BackupTarget = nil
			updated, err := handler.OnBackupRemove("", vmb)
			require.NoError(t, err)
			require.Equal(t, "updated", updated.ResourceVersion)
			require.Equal(t, 2, remover.removeCalls)
			require.False(t, remover.removeRemote)
		})
	}
}

func TestRemoveStopsOnTargetError(t *testing.T) {
	handler, remover, vmb := cleanupFixture(t, harvesterv1.Restic)
	require.NoError(t, settings.BackupTargetSet.Set("invalid"))
	_, err := handler.OnBackupRemove("", vmb)
	require.ErrorContains(t, err, "failed to decode backup target")
	require.Zero(t, remover.removeCalls)
}
