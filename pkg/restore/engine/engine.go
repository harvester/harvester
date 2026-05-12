package engine

import (
	"context"
	"errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

var (
	ErrRetryLater = errors.New("retry later error")
)

// RestoreEngine defines the interface for restoring VM volumes
type RestoreEngine interface {
	// Reconcile ensures the per-volume restore at volIndex matches desired state:
	// it creates the restored PVC if missing, otherwise checks status. Idempotent.
	Reconcile(vmr *harvesterv1.VirtualMachineRestore, vmb *harvesterv1.VirtualMachineBackup, volIndex int) error

	// UpdateProgress updates the restore progress for a specific volume
	UpdateProgress(vr *harvesterv1.VolumeRestore) (int64, error)

	// Delete handles cleanup of restore resources for a specific volume
	Delete(vmr *harvesterv1.VirtualMachineRestore, volIndex int) error

	// RegisterWatchers gives the engine an opportunity to register its own
	// informer event handlers (e.g. on Jobs it creates) and call enqueue to
	// trigger reconcile on the owning VMRestore. Engines that don't need any
	// extra watchers should implement this as a no-op.
	RegisterWatchers(ctx context.Context, enqueue func(namespace, name string))
}
