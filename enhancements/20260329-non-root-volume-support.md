# Non-Root Volume Support for 3rd-Party Storage

## Summary

Harvester's CDI (Containerized Data Importer) settings and StorageProfile configurations currently only apply to VM image and root disk volumes backed by CDI DataVolumes. Non-root volumes created directly from the UI as standard PVCs bypass CDI entirely, which prevents 3rd-party CSI storage solutions from leveraging CDI StorageProfiles for access modes and volume modes.

This enhancement addresses this limitation by:
1. Providing an optional CDI DataVolume creation path alongside the existing PVC creation path, allowing users to opt in to CDI StorageProfile support when creating non-root volumes.
2. Exposing advanced volume creation options (access mode, volume mode) in the UI.
3. Adding a **Data Migration** action on the Volume page to allow offline storage migration between StorageClasses via CDI.
4. Removing the legacy `volumeForVirtualMachine` annotation mechanism and its associated filesystem blank source workaround, which are no longer needed.

### Related Issues

https://github.com/harvester/harvester/issues/9412

## Motivation

The StorageProfile of CDI provide a centralized way to configure storage attributes (access modes, volume modes, clone strategies, snapshot classes) per StorageClass. However, these profiles only apply to CDI DataVolumes, not to standard PVCs. When a user creates a non-root volume from the Harvester UI, it is created as a plain PVC. This means:

- Users cannot leverage CDI's intelligent defaults (e.g., automatically selecting the right access mode based on the StorageClass profile).
- The previous workaround using `volumeForVirtualMachine` annotation and `VolumeImportSource` filesystem blank source was fragile and only partially addressed the issue for filesystem-mode volumes.

Additionally, users lack a way to migrate data between volumes of different StorageClasses without manual intervention. This is needed when transitioning workloads between storage backends or when rebalancing storage across different tiers.

### Goals

- Provide an optional DataVolume-based creation path for non-root volumes, so users who need CDI StorageProfile support can opt in
- Preserve the existing PVC creation path as the default for backward compatibility
- Expose advanced options (access mode, volume mode) in the UI for volume creation
- Provide a Data Migration API action on volumes to clone data (offline) between StorageClasses using CDI
- Remove the legacy `volumeForVirtualMachine` annotation and filesystem blank source mechanism

### Non-goals

None. This enhancement focuses solely on non-root volume creation and data migration. It does not change the root disk creation process or the underlying CDI StorageProfile implementation.

## Proposal

### User Stories

#### Story 1: Create Non-Root Volume with 3rd-Party Storage via DataVolume

Before this enhancement, a user creating a non-root volume from the UI with a 3rd-party CSI StorageClass would get a plain PVC that ignores CDI StorageProfile settings. The user had no way to ensure correct access modes or clone strategies were applied.

After this enhancement, the user can choose to create the volume through a CDI DataVolume interface instead of a plain PVC. When the DataVolume path is selected, StorageProfile for the chosen StorageClass is automatically applied. The traditional PVC creation path remains available as the default for users who do not need CDI StorageProfile integration.

#### Story 2: Volume Creation with Advanced Options

Before this enhancement, the UI only supported basic volume creation with name, size, and StorageClass. Access mode was hardcoded.

After this enhancement, users can toggle "Show Advanced Options" to configure access mode (ReadWriteOnce, ReadWriteMany) and volume mode (Filesystem, Block) during volume creation.

#### Story 3: Migrate Volume Data to a Different StorageClass

Before this enhancement, migrating data from a volume on one StorageClass to another required manual steps: creating a new volume, attaching both to a VM, and copying data.

After this enhancement, the user can select "Data Migration" from the volume actions menu. They specify a target volume name and target StorageClass, and the system creates a CDI DataVolume that clones the data automatically. The action is only available when the volume is not attached to any running VM.

### User Experience In Detail

#### Volume Creation

On the "Create Volume" page, users see the standard fields (name, size, StorageClass, image source). Two new capabilities are available:

**1. Creation Interface Selection**

Users can choose between two creation interfaces:
- **PVC** (default): Creates a standard PersistentVolumeClaim directly. This is the existing behavior.
- **DataVolume**: Creates a CDI DataVolume CR, which in turn provisions the PVC. StorageProfile for the selected StorageClass is applied, ensuring correct access modes, volume modes, and clone strategies for 3rd-party CSI solutions.

**2. Advanced Options**

A "Show Advanced Options" link reveals additional fields:
- **Access Mode**: dropdown with `ReadWriteOnce`, `ReadWriteMany`, `ReadOnlyMany` and `ReadWriteOncePod` options
- **Volume Mode**: dropdown with `Filesystem` and `Block` options

When using the DataVolume interface and these fields are left unset, StorageProfile for the selected StorageClass determines the defaults automatically.

#### Data Migration

On the Volumes list page, each **Bound** volume that is **not attached to any VM** displays a "Data Migration" action in its action menu. Clicking it opens a dialog with:

- **Target Volume Name**: the name for the new volume
- **Target StorageClass**: dropdown to select the destination StorageClass

Upon confirmation, the system:
1. Validates the target name does not conflict with existing PVCs or DataVolumes
2. Validates the target StorageClass exists
3. Validates the source PVC is not in use by any Pod
4. Creates a CDI DataVolume with `source.pvc` referencing the original volume

The new DataVolume appears in the Volumes list. CDI handles the actual data cloning. Progress can be monitored via the DataVolume status.

### API changes

#### New Volume Action: `dataMigration`

**Endpoint:** `POST /v1/harvester/persistentvolumeclaims/{namespace}/{name}?action=dataMigration`

**Input type:**
```go
type DataMigrationInput struct {
	TargetVolumeName       string `json:"targetVolumeName"`
	TargetStorageClassName string `json:"targetStorageClassName"`
}
```

**Created DataVolume spec:**
```yaml
apiVersion: cdi.kubevirt.io/v1beta1
kind: DataVolume
metadata:
  name: <targetVolumeName>
  namespace: <source PVC namespace>
spec:
  source:
    pvc:
      name: <source PVC name>
      namespace: <source PVC namespace>
  storage:
    storageClassName: <targetStorageClassName>
    resources:
      requests:
        storage: <same as source PVC>
```

Note: `accessModes` and `volumeMode` are intentionally omitted from the DataVolume spec. CDI's StorageProfile for the target StorageClass determines these values automatically.

## Design

### Implementation Overview

#### Backend Changes

**1. Volume API (`pkg/api/volume/`)**

- **`types.go`**: Add `DataMigrationInput` struct with `TargetVolumeName` and `TargetStorageClassName` fields.
- **`formatter.go`**: Add `actionDataMigration` constant. Add `vmCache` to `volFormatter` to check VM attachment. Show the `dataMigration` action only when the PVC is Bound and not attached to any VM (via `VMByPVCIndex`).
- **`schema.go`**: Register `DataMigrationInput` schema, add `dataMigration` to `ResourceActions` and `ActionHandlers`, wire `DataVolumeClient` and `VirtualMachineCache` from scaled factories.
- **`handler.go`**: Add `dataVolumes` field (`ctlcdiv1.DataVolumeClient`) to `ActionHandler`. Implement `dataMigration()` method that validates inputs, checks for conflicts, verifies the PVC is not in use, and creates the CDI DataVolume CR.

**2. PVC Controller (`pkg/controller/master/pvc/`)**

- Remove `createFilesystemBlankSource` handler and all `VolumeImportSource` logic. The `pvcHandler` struct is simplified to only hold `dataVolumeClient` for DataVolume cleanup on PVC deletion.
- Remove `volImportSourceClient`, `pvcClient`, `pvcController` fields.

**3. VM Controller (`pkg/controller/master/virtualmachine/`)**

- Remove the `volumeForVirtualMachine` annotation injection when creating PVCs from VM annotations. The filesystem blank source workaround is no longer needed since non-root volumes are now created via DataVolumes.

**4. PVC Webhook Mutator (`pkg/webhook/resources/persistentvolumeclaim/`)**

- Remove the `patchDataSource` method that injected `VolumeImportSource` references into PVCs annotated with `volumeForVirtualMachine`. The mutator now only handles golden image annotation patching.

**5. Constants (`pkg/util/constants.go`)**

- Remove `AnnotationVolForVM`, `KindVolumeImportSource`, and `ImportSourceFSBlank` constants.

#### Frontend Changes (harvester-ui-extension)

- **Volume creation page**: Add a creation interface selector (PVC / DataVolume) allowing users to opt in to CDI DataVolume-based creation. Add "Show Advanced Options" link that reveals access mode and volume mode dropdowns.
- **Volume list page**: Add "Data Migration" action with a dialog for target volume name and target StorageClass selection.
- **Feature flags**: Add feature flags for the DataVolume creation option and data migration action.

### Test plan

#### Volume Creation

1. Create a Harvester cluster with at least one 3rd-party StorageClass configured
2. Navigate to Volumes page and create a new volume using the default PVC interface
3. Verify a standard PVC is created (backward compatibility)
4. Create a new volume using the DataVolume interface option
5. Verify a DataVolume CR is created and the resulting PVC has correct StorageProfile settings
6. Create a volume with advanced options (specific access mode and volume mode) via the DataVolume interface
7. Verify the DataVolume and resulting PVC reflect the specified settings

#### Data Migration

1. Create a volume on StorageClass A, write test data
2. Ensure the volume is not attached to any VM
3. Call the `dataMigration` action with a target name and StorageClass B
4. Verify a DataVolume CR is created with the correct `source.pvc` and `storage` spec
5. Wait for the DataVolume to complete (status: `Succeeded`)
6. Verify the new PVC exists with the correct StorageClass and data
7. Attach the new volume to a VM and verify data integrity

#### Data Migration Validation

1. Attempt data migration on a volume attached to a running VM - should fail
2. Attempt data migration with a target name that already exists as a PVC - should fail
3. Attempt data migration with a non-existent target StorageClass - should fail

#### Legacy Mechanism Removal

1. Verify volumes created with filesystem mode work correctly without the `volumeForVirtualMachine` annotation
2. Verify no `VolumeImportSource` CRs are created for new filesystem volumes
3. Verify existing volumes (created before upgrade) continue to function normally

### Upgrade strategy

Upgrade would not affected with this change. The new DataVolume creation path and data migration action are additive features that do not interfere with existing PVC creation or VM workflows. The removal of the legacy `volumeForVirtualMachine` annotation and filesystem blank source mechanism is safe.

## Note

None.