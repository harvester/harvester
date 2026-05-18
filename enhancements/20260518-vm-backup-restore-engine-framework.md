# VM Backup and Restore Engine Framework

## Summary

This design document outlines the general backup and restore engine framework for Harvester `VirtualMachineBackup` and `VirtualMachineRestore`. The framework refactors the existing controller implementation into reusable operators, engine interfaces, and backend-specific implementations so that local CSI snapshots and remote Longhorn backups can share one orchestration model.

### Related Issues

- [Issue #9135](https://github.com/harvester/harvester/issues/9135)

## Motivation

### Goals

- **Unify VM Backup and Restore Flows**: Provide one controller workflow for VM backup and restore while allowing different storage backends to implement their own volume operations.
- **Support Multiple Backup Types**: Route `VirtualMachineBackup` operations by type, starting with `backup` for Longhorn remote backup and `snapshot` for local CSI snapshot.
- **Reduce Controller Complexity**: Move low-level VMBackup, VMRestore, VolumeSnapshot, PVC, secret, and status operations out of the main controllers.
- **Preserve Existing Behavior**: Maintain existing backup metadata sync, restore, finalizer, cleanup, progress, and webhook behavior while changing the internal structure.
- **Enable Future Engines**: Allow future storage engines to plug into the same controller lifecycle without rewriting the whole VM backup and restore controller.

## Proposal

### User Stories

**Story 1: As a cluster administrator**
I want VM backup and restore to support both remote backups and local snapshots so that I can choose the right protection mechanism for each VM.

**Story 2: As a Harvester maintainer**
I want backup and restore code to be split into clear operators and engines so that I can add or debug storage backends without changing unrelated controller logic.

**Story 3: As a storage integrator**
I want a small engine interface for per-volume backup and restore operations so that a storage backend can implement snapshot, progress, and cleanup behavior independently.

**Story 4: As a VM user**
I want restore behavior, secret restore, MAC address handling, deleted PVC cleanup, and halt-after-restore behavior to remain consistent across backup types.

## Design

### Architecture Overview

The framework separates VM backup and restore into three layers:

- **Controller Layer**: Watches `VirtualMachineBackup`, `VirtualMachineRestore`, `VolumeSnapshot`, Longhorn backup, PVC, and VM events. It owns high-level reconciliation order and requeue behavior.
- **Operator Layer**: Provides common operations on VM backup and restore objects, including status updates, condition management, source VM lookup, filesystem freeze support, metadata handling, secret restore, PVC deletion, and VM start.
- **Engine Layer**: Implements backend-specific per-volume backup and restore behavior for `backup` and `snapshot` types.

This structure keeps the controller lifecycle stable while allowing different volume workflows underneath it.

#### Why BackupEngine and RestoreEngine Abstractions?

The existing implementation mixed high-level VM lifecycle decisions with storage-specific operations. This made it hard to add a new backup type because controller code needed to understand each backend's details.

The engine abstractions solve this by defining a small contract:

```go
type BackupEngine interface {
    Reconcile(vmb *VirtualMachineBackup, volIndex int, vsClassMap map[string]VolumeSnapshotClass) error
    UpdateProgress(vb *VolumeBackup) (int, error)
    ForceDelete(vmb *VirtualMachineBackup, volIndex int) error
}

type RestoreEngine interface {
    Reconcile(vmr *VirtualMachineRestore, vmb *VirtualMachineBackup, volIndex int) error
    UpdateProgress(vr *VolumeRestore) (int, error)
    Delete(vmr *VirtualMachineRestore, volIndex int) error
}
```

The controller can now reconcile each volume by index and delegate backend-specific work to the selected engine. Engines remain idempotent, so normal Kubernetes requeues and watch events can safely retry partial progress.

### Component Architecture

#### 1. API Resources

The framework keeps the existing Harvester VM backup and restore resources:

- **`VirtualMachineBackup`**: Represents a VM backup operation and stores source VM spec, volume backup metadata, secret backup metadata, progress, backup target, and conditions.
- **`VirtualMachineRestore`**: Represents a VM restore operation and stores target VM information, volume restore metadata, deleted volume tracking, progress, and conditions.

`VirtualMachineBackupSpec` includes the backup type:

```go
type BackupType string

const (
    Backup   BackupType = "backup"
    Snapshot BackupType = "snapshot"
)

type VirtualMachineBackupSpec struct {
    Source corev1.TypedLocalObjectReference `json:"source"`
    // +kubebuilder:validation:Enum=backup;snapshot
    // +kubebuilder:default:=backup
    Type   BackupType `json:"type,omitempty"`
}
```

The default type is `backup`, preserving the remote Longhorn backup behavior for existing VMBackup resources.

#### 2. Controllers

The backup and restore controllers are responsible for orchestration rather than backend details.

Backup controller responsibilities:

- Initialize VMBackup status from the source VM.
- Configure backup target and CSI `VolumeSnapshotClass` mappings.
- Reconcile per-volume backup work through the selected `BackupEngine`.
- Update backup progress, ready state, and error conditions.
- Sync backup metadata to the configured backup target.
- Force-delete stuck snapshots and snapshot contents when backup targets change.

Restore controller responsibilities:

- Initialize VMRestore status and volume restore entries.
- Resolve and validate the referenced VMBackup.
- Create or update the target VM.
- Reconcile per-volume restore work through the selected `RestoreEngine`.
- Restore secrets and remap secret references for new VMs.
- Start the VM unless `haltAfterRestore` is requested.
- Delete old PVCs unless restoring a new VM or using retain policy.
- Mark restore completion and clean engine-created resources on deletion.

#### 3. Engines

The initial implementation includes four engines:

- **Backup Snapshot Engine**: Creates local CSI `VolumeSnapshot` objects from source PVCs and updates `VolumeBackup` status from snapshot status.
- **Backup Longhorn Engine**: Creates CSI snapshots for Longhorn-backed volumes, binds snapshots to existing Longhorn backups when syncing metadata from a remote target, tracks Longhorn backup progress, and force-deletes stuck snapshot resources.
- **Restore Snapshot Engine**: Restores PVCs from local CSI snapshots in the same namespace and validates snapshot readiness.
- **Restore Longhorn Engine**: Restores PVCs from Longhorn backup data, creates cross-namespace restore `VolumeSnapshot` and `VolumeSnapshotContent` resources when needed, tracks Longhorn engine restore progress, and deletes retained restore `VolumeSnapshotContent` resources.

### Backup Type Routing

`VirtualMachineBackup.spec.type` selects the engine and is fixed at creation time:

- **`type=backup`**: Uses the Longhorn backup engine. This supports remote backup metadata, Longhorn backup progress, backup target validation, and restore from Longhorn backup handles.
- **`type=snapshot`**: Uses the local CSI snapshot engine. This supports local snapshot creation and same-namespace restore from `VolumeSnapshot`.

`VirtualMachineBackup.spec.type` must be immutable after creation. The validating webhook for `VirtualMachineBackup` must reject updates that change `spec.type`, because this field determines which engine owns the backup artifacts and how existing status, `VolumeSnapshot`, and `VolumeSnapshotContent` resources are interpreted.

The restore controller selects its restore engine from the referenced `VirtualMachineBackup.spec.type`, ensuring the restore path matches the backup path and that restore/delete reconciliation always uses the same engine that created or recovered the backup artifacts.

### Metadata and Remote Sync

Backup metadata sync remains part of the VM backup controller flow. Metadata includes:

- VMBackup name and namespace
- VMBackup spec
- source VM spec
- volume backup metadata
- secret backup metadata

The framework preserves metadata-ready conditions and backup target consistency checks. When a backup is recovered from remote metadata, the Longhorn backup engine can rebuild CSI `VolumeSnapshot` and `VolumeSnapshotContent` objects that point at existing Longhorn backups instead of creating a fresh live snapshot.

---

## Implementation Details

### Part 1: VM Backup Framework

This section describes the changes in the backup controller and backup engine packages.

#### 1.1 Backup Operator

The backup operator centralizes common VMBackup operations in `pkg/backup/common/operator.go`.

Primary responsibilities:

- Read and mutate `VolumeBackup` fields.
- Read and mutate VMBackup status fields.
- Set processing, ready, error, and non-ready transition conditions.
- Initialize VMBackup status from the source VM.
- Resolve source VM and PVC metadata.
- Configure CSI snapshot class mappings.
- Configure backup target status.
- Persist VMBackup updates only when content changes.
- Freeze the VM filesystem through the KubeVirt subresource API when supported.

The controller uses this operator rather than directly editing backup status in many places.

#### 1.2 Volume Snapshot Helper

The shared snapshot helper in `pkg/backup/common/vshelper.go` wraps common CSI snapshot operations:

- Create `VolumeSnapshot` from PVC.
- Create or retrieve `VolumeSnapshotContent`.
- Build owner references.
- Update `VolumeBackup` status from `VolumeSnapshot` status.
- Check deletion states.
- Force-delete `VolumeSnapshot` and orphaned `VolumeSnapshotContent` resources.

Both backup engines reuse this helper.

#### 1.3 Backup Engine Interface

The backup engine interface lives in `pkg/backup/engine/engine.go`.

Backup engines are called once per volume backup entry. They must be idempotent and should return `ErrRetryLater` when the controller should requeue without marking the VMBackup as failed.

#### 1.4 Snapshot Backup Engine

The snapshot backup engine lives in `pkg/backup/snapshot/snapshot.go`.

Workflow:

1. Locate the `VolumeBackup` by index.
2. Check whether the corresponding `VolumeSnapshot` already exists.
3. If the snapshot exists, update `VolumeBackup` status from the snapshot.
4. If the snapshot does not exist, optionally freeze the VM filesystem and create a new `VolumeSnapshot` from the source PVC.

This engine is intended for local CSI snapshots.

#### 1.5 Longhorn Backup Engine

The Longhorn backup engine lives in `pkg/backup/longhorn/longhorn.go`.

Workflow:

1. Locate the `VolumeBackup` by index.
2. Reuse an existing `VolumeSnapshot` when present and update status from it.
3. When recovering from remote metadata, create a `VolumeSnapshotContent` with a Longhorn `bak://` snapshot handle and bind a `VolumeSnapshot` to it.
4. For fresh backups, freeze the source VM filesystem when possible and create a `VolumeSnapshot` from the source PVC.
5. Track Longhorn backup progress through the Longhorn backup resource.
6. Validate whether Longhorn backup data still exists in the configured backup target before moving a backup back to ready.

The engine also provides force-delete behavior for snapshots and snapshot contents that can become stuck after backup target changes.

### Part 2: VM Restore Framework

This section describes the changes in the restore controller and restore engine packages.

#### 2.1 Restore Operator

The restore operator centralizes common VMRestore operations in `pkg/restore/common/operator.go`.

Primary responsibilities:

- Read and mutate `VolumeRestore` fields.
- Set processing, ready, complete, and error conditions.
- Initialize VMRestore status.
- Build `VolumeRestore` entries from the referenced VMBackup.
- Track deleted PVCs for existing-VM restore.
- Map VM volumes to restored PVC names.
- Restore secrets and remap secret references for new VMs.
- Sanitize VM annotations and VM spec during new-VM restore.
- Start the VM through the KubeVirt subresource API.
- Delete old PVCs after a successful existing-VM restore.

#### 2.2 Restore Engine Interface

The restore engine interface lives in `pkg/restore/engine/engine.go`.

Restore engines are called once per volume restore entry. They are responsible for creating the restored PVC if needed, checking readiness, reporting progress, and cleaning engine-owned resources when the VMRestore is removed.

#### 2.3 PVC Helper

The PVC helper in `pkg/restore/pvchelper/pvchelper.go` provides shared PVC restore utilities:

- Build restore annotations while filtering annotations that should not be copied.
- Build a PVC from a snapshot data source.
- Check PVC readiness and error states.

Both restore engines use the helper to keep PVC construction consistent.

#### 2.4 Snapshot Restore Engine

The snapshot restore engine lives in `pkg/restore/snapshot/snapshot.go`.

Workflow:

1. Locate the `VolumeRestore` by index.
2. Check whether the restored PVC already exists.
3. If missing, validate the source `VolumeSnapshot`.
4. Create the restored PVC from the source `VolumeSnapshot`.
5. If present, validate PVC readiness.

Local snapshot restore is limited to same-namespace restore because a namespaced `VolumeSnapshot` cannot be used directly as a PVC data source from another namespace.

#### 2.5 Longhorn Restore Engine

The Longhorn restore engine lives in `pkg/restore/longhorn/longhorn.go`.

Workflow:

1. Locate the `VolumeRestore` and matching `VolumeBackup`.
2. If source and target namespaces differ, create restore-specific `VolumeSnapshotContent` and `VolumeSnapshot` resources.
3. Create the restored PVC from the appropriate snapshot data source.
4. Check PVC readiness and Longhorn volume restore status.
5. Track restore progress from Longhorn engine replica restore status.
6. Delete retained restore `VolumeSnapshotContent` resources when the VMRestore is deleted.

### Part 3: Controller Integration

#### 3.1 Backup Controller Refactor

`pkg/controller/master/backup/backup.go` now builds a controller dependency set, creates the backup operator, registers engines by `BackupType`, and wires event handlers.

The main backup reconciliation flow is:

1. Ignore deleted or ready backups except for ready-backup maintenance work.
2. Initialize missing status from the source VM.
3. Configure backup target and CSI snapshot classes.
4. Reconcile all volume backups through the selected engine.
5. Update status, conditions, progress, and metadata readiness.

#### 3.2 Restore Controller Refactor

`pkg/controller/master/backup/restore.go` now builds restore operators and engines and delegates backend-specific volume restore work.

The main restore reconciliation flow is:

1. Initialize VMRestore status when missing.
2. Fetch and validate the referenced VMBackup.
3. Initialize volume restore status from backup metadata.
4. Create or update the target VM.
5. Restore secrets.
6. Reconcile all volume restores through the selected engine.
7. Update progress.
8. Start the VM unless `haltAfterRestore` is true.
9. Delete old PVCs when appropriate.
10. Mark restore complete.

#### 3.3 Webhook Updates

Webhook validation and mutation are updated for the new backup type behavior:

- Default and validate `VirtualMachineBackup.spec.type`.
- Validate snapshot and backup settings used by backup and restore.
- Update VM and VMRestore validation paths for new restore behaviors.
- Maintain snapshot-related filtering utilities used by webhook logic.

### Part 4: Scheduled VM Backup Integration

Scheduled VM backup code is updated so that scheduled backups can create VMBackup resources with the expected backup type and can work with the refactored VMBackup controller flow.

## Upgrade Strategy

The framework is intended to be backward compatible:

- Existing `VirtualMachineBackup` resources default to `type=backup`.
- Existing backup metadata remains compatible with the Longhorn backup engine.
- Existing restore specs remain compatible because restore engine selection comes from the referenced VMBackup.
- Existing conditions and status fields are preserved.
- Existing backup target settings continue to be used by metadata sync and Longhorn backup validation.

After upgrade, controllers reconcile existing VMBackup and VMRestore resources through the new operator and engine paths.

## Limitations and Implementation

- The first engine set supports Longhorn remote backup and local CSI snapshot behavior.
- Local snapshot restore is same-namespace only.
- Remote backup restore depends on Longhorn backup metadata and the configured backup target.
- Filesystem freeze is best-effort and depends on a running VM with guest agent support.
- Engine selection is based on `VirtualMachineBackup.spec.type`; a VMRestore cannot override the backup type.
- The framework does not introduce a new public backup or restore CRD. It reorganizes implementation around the existing VM backup and restore APIs.

## Test Plan

### Environment Prerequisites

- A Harvester cluster with Longhorn enabled.
- A configured backup target for `type=backup` workflows.
- At least one VM with one or more PVC-backed disks.
- Optional guest agent support for filesystem freeze validation.

### VM Backup Validation

- Create a default `VirtualMachineBackup` and verify it uses `type=backup`.
- Create a `VirtualMachineBackup` with `type=backup`.
- Verify VMBackup status initialization includes source VM spec, source UID, volume backup metadata, and secret backup metadata.
- Verify per-volume progress is updated for Longhorn backup.
- Verify backup completion sets ready and metadata-ready conditions.
- Verify backup metadata is written to the configured backup target.
- Verify backup target changes can transition an affected backup to non-ready and force-delete stuck snapshot resources.

### VM Restore Validation

- Restore a VM from a Longhorn backup into the same namespace.
- Restore a VM from a Longhorn backup into a different namespace.
- Restore a VM from a local snapshot in the same namespace.
- Verify restore to a new VM remaps secret names and optionally removes MAC addresses.
- Verify restore to an existing VM updates volume PVC references and deletes old PVCs when deletion policy is `delete`.
- Verify deletion policy `retain` keeps old PVCs.
- Verify `haltAfterRestore` keeps the restored VM halted and still marks the restore complete.
- Verify restore progress is capped before VM start and reaches completion after the VM is ready.

### Webhook and Scheduled Backup Validation

- Verify VMBackup creation defaults and validates `spec.type`.
- Verify invalid backup type values are rejected.
- Verify VMRestore validation rejects unsupported or inconsistent restore requests.
- Verify scheduled VM backup creates VMBackup resources that reconcile through the new framework.
