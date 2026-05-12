package engine

import (
	"context"
	"errors"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

var (
	ErrRetryLater = errors.New("retry later error")
)

type BackupEngine interface {
	// Reconcile ensures the per-volume backup at volIndex matches desired state:
	// it creates the volume snapshot if missing, otherwise refreshes status. Idempotent.
	Reconcile(vmb *harvesterv1.VirtualMachineBackup, volIndex int, vsClassMap map[string]snapshotv1.VolumeSnapshotClass) error
	UpdateProgress(*harvesterv1.VolumeBackup) (int64, error)
	ForceDelete(vmb *harvesterv1.VirtualMachineBackup, volIndex int) error
	// RegisterWatchers gives the engine an opportunity to register its own
	// informer event handlers (e.g. on Jobs it creates) and call enqueue to
	// trigger reconcile on the owning VMBackup. Engines that don't need any
	// extra watchers should implement this as a no-op.
	RegisterWatchers(ctx context.Context, enqueue func(namespace, name string))
}
