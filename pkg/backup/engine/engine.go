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

// Restic and Kopia use the same Job lifecycle policy:
//
//	Job type             Restic               Kopia
//	Backup/create Job    TTL 60s, backoff 0   TTL 60s, backoff 0
//	Snapshot-check Job   TTL 60s, backoff 0   TTL 60s, backoff 0
//	Forget Job           No TTL, backoff 3    No TTL, backoff 3
//
// Forget Jobs remain until the owning VMBackup is finalized so completed
// per-volume cleanup is not repeated if later volumes take longer than a TTL.
type BackupEngine interface {
	// Reconcile ensures the per-volume backup at volIndex matches desired state:
	// it creates the volume snapshot if missing, otherwise refreshes status. Idempotent.
	Reconcile(vmb *harvesterv1.VirtualMachineBackup, volIndex int, vsClassMap map[string]snapshotv1.VolumeSnapshotClass) error
	// UpdateProgress returns the per-volume backup progress percentage, bounded
	// between 0 and 100.
	UpdateProgress(*harvesterv1.VolumeBackup) (int64, error)
	ForceDelete(vmb *harvesterv1.VirtualMachineBackup, volIndex int) error
	// RegisterWatchers gives the engine an opportunity to register its own
	// informer event handlers (e.g. on Jobs it creates) and call enqueue to
	// trigger reconcile on the owning VMBackup. Engines that don't need any
	// extra watchers should implement this as a no-op.
	RegisterWatchers(ctx context.Context, enqueue func(namespace, name string))
}

// BackupPreparer is implemented by engines that need all source snapshots to
// be ready before per-volume backup reconciliation can start.
type BackupPreparer interface {
	Prepare(
		vmb *harvesterv1.VirtualMachineBackup,
		vsClassMap map[string]snapshotv1.VolumeSnapshotClass,
	) (bool, error)
}
