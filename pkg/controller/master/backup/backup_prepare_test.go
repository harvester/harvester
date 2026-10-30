package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/backup/common"
	"github.com/harvester/harvester/pkg/backup/engine"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCreateVolumeBackupsWaitsForPreparation(t *testing.T) {
	tests := []struct {
		name           string
		backupType     harvesterv1.BackupType
		prepared       bool
		prepareErr     error
		wantReconciles int
	}{
		{name: "preparing snapshots", backupType: harvesterv1.Restic},
		{name: "snapshots ready", backupType: harvesterv1.Restic, prepared: true, wantReconciles: 1},
		{name: "preparation retry", backupType: harvesterv1.Kopia, prepareErr: engine.ErrRetryLater},
		{name: "preparation failed", backupType: harvesterv1.Kopia, prepareErr: errors.New("prepare failed")},
		{name: "longhorn ready", backupType: harvesterv1.Backup, prepared: true, wantReconciles: 1},
		{name: "snapshot ready", backupType: harvesterv1.Snapshot, prepared: true, wantReconciles: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backupEngine := &preparingEngine{
				prepared:   tt.prepared,
				prepareErr: tt.prepareErr,
			}
			controller := &requeueBackupController{}
			vmbo := common.NewVMBackupOperatorBuilder().Build()
			handler := &Handler{
				vmbController: controller,
				vmbo:          vmbo,
				engines: map[harvesterv1.BackupType]engine.BackupEngine{
					tt.backupType: backupEngine,
				},
			}
			vmb := &harvesterv1.VirtualMachineBackup{
				ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "default"},
				Spec:       harvesterv1.VirtualMachineBackupSpec{Type: tt.backupType},
				Status: harvesterv1.VirtualMachineBackupStatus{
					VolumeBackups: []harvesterv1.VolumeBackup{{Name: ptr.To("volume")}},
				},
			}

			err := handler.CreateVolumeBackups(
				vmb,
				map[string]snapshotv1.VolumeSnapshotClass{},
			)
			require.Equal(t, 1, backupEngine.prepareCalls)
			require.Equal(t, tt.wantReconciles, backupEngine.reconcileCalls)
			if tt.prepareErr != nil && tt.prepareErr != engine.ErrRetryLater {
				require.ErrorIs(t, err, tt.prepareErr)
				require.Empty(t, controller.enqueues)
				return
			}
			require.NoError(t, err)
			require.Len(t, controller.enqueues, 1)
			require.Equal(t, engineRetryDelay, controller.enqueues[0].after)
		})
	}
}

type preparingEngine struct {
	prepared       bool
	prepareErr     error
	prepareCalls   int
	reconcileCalls int
}

func (e *preparingEngine) Prepare(
	_ *harvesterv1.VirtualMachineBackup,
	_ map[string]snapshotv1.VolumeSnapshotClass,
) (bool, error) {
	e.prepareCalls++
	return e.prepared, e.prepareErr
}

func (e *preparingEngine) Reconcile(
	_ *harvesterv1.VirtualMachineBackup,
	_ int,
	_ map[string]snapshotv1.VolumeSnapshotClass,
) error {
	e.reconcileCalls++
	return engine.ErrRetryLater
}

func (e *preparingEngine) UpdateProgress(*harvesterv1.VolumeBackup) (int64, error) {
	return 0, nil
}

func (e *preparingEngine) Remove(
	vmb *harvesterv1.VirtualMachineBackup,
	_ bool,
) (*harvesterv1.VirtualMachineBackup, error) {
	return vmb, nil
}

func (e *preparingEngine) RegisterWatchers(context.Context, func(string, string)) {}

type enqueueCall struct {
	namespace string
	name      string
	after     time.Duration
}

type requeueBackupController struct {
	fakeclients.VMBackupClient
	enqueues []enqueueCall
}

func (c *requeueBackupController) Informer() cache.SharedIndexInformer {
	return nil
}

func (c *requeueBackupController) GroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{}
}

func (c *requeueBackupController) AddGenericHandler(context.Context, string, generic.Handler) {}

func (c *requeueBackupController) AddGenericRemoveHandler(context.Context, string, generic.Handler) {}

func (c *requeueBackupController) Updater() generic.Updater {
	return nil
}

func (c *requeueBackupController) OnChange(
	context.Context,
	string,
	generic.ObjectHandler[*harvesterv1.VirtualMachineBackup],
) {
}

func (c *requeueBackupController) OnRemove(
	context.Context,
	string,
	generic.ObjectHandler[*harvesterv1.VirtualMachineBackup],
) {
}

func (c *requeueBackupController) Enqueue(string, string) {}

func (c *requeueBackupController) EnqueueAfter(namespace, name string, after time.Duration) {
	c.enqueues = append(c.enqueues, enqueueCall{
		namespace: namespace,
		name:      name,
		after:     after,
	})
}

func (c *requeueBackupController) Cache() generic.CacheInterface[*harvesterv1.VirtualMachineBackup] {
	return nil
}
