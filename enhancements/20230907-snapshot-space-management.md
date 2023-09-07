# Snapshot Space Management

## Summary

This proposal leverages [LH Snapshot Space Management](https://github.com/longhorn/longhorn/issues/6563) to control snapshot space usage in the system. LH introduces two new attributes `snapshotMaxCount` and `snapshotMaxSize` in Volume. Harvester can use these parameters to set upper bound for snapshot space usage.

### Related Issues

https://github.com/harvester/harvester/issues/4478

## Motivation

### Goals

- Set snapshot count and size limitations for each volume.

## Proposal

Use a PVC controller to watch PVCs. If there is `harvesterhci.io/snapshotMaxCount` or `harvesterhci.io/snapshotMaxSize` annotation on a PVC, the system uses the value to update the related LH Volume.

### User Stories

With snapshot space management, snapshot space usage is controllable. Users can set snapshot limitation and evaluate maximum space usage.

### User Experience In Detail

Before snapshot space management:
The default maximum snapshot count for each volume is `250`. This is a constants value in the [source code](https://github.com/longhorn/longhorn-engine/blob/8b4c80ab174b4f454a992ff998b6cb1041faf63d/pkg/replica/replica.go#L33) and users don't have a way to control it. If a volume size is 1G, the maximum snapshot space usage will be 250G.

After snapshot space management:
There is a configurable default maximum snapshot count setting. Users can update it to overwrite the fixed value in the system.
Users can set different maximum snapshot count and size on each volume. A more important volume can have more snapshot space usage.

### API changes

No harvester API changes are needed. The changes will only be in the pvc controller logic.

## Design

### Implementation Overview

#### Settings

Add a harvester setting `snapshot-max-count`. The range of the value is same as LH setting `snapshot-max-count`. It should be between `2` to `250`. The [Setting controller](https://github.com/harvester/harvester/blob/ea814927a14f99b0f0ef9fe74c37dbea3e6e95df/pkg/controller/master/setting/register.go#L66-L80) will overwrite the value to LH setting.

#### PVC webhook

Add annotations value check to [PVC webhook](https://github.com/harvester/harvester/blob/master/pkg/webhook/resources/persistentvolumeclaim/validator.go).

* `harvesterhci.io/snapshotMaxCount`: The default value is from the harvester setting `snapshot-max-count`. Users can overwrite the default value by setting the annotation.
* `harvesterhci.io/snapshotMaxSize`: The default value is `0` which means no limitation for snapshot max size. Without `0`, the minimum value is `PVC size * 2`. The minimum snapshot count is `2`, so the system needs at least twice of the PVC size.

#### PVC controller

Introduce a new PVC controller to watch PVCs. If there is `harvesterhci.io/snapshotMaxCount` or `harvesterhci.io/snapshotMaxSize` annotation on a PVC, the system uses the value to update `snapshotMaxCount` or `snapshotMaxSize` fields in the related LH Volume.

#### VMBackup webhook

From LH perspective, it only checks whether one volume can take a new snapshot. However, a VM may has multiple volumes. If Harvester doesn't do pre-checks, users may get a VMBackup with partial failed snapshots.

Introduce snapshot limitation checks in [VMBackup webhook](https://github.com/harvester/harvester/blob/master/pkg/webhook/resources/virtualmachinebackup/validator.go). The webhook checks related LH [EngineStatus](https://github.com/longhorn/longhorn-manager/blob/421da081cf870659e390dd6e08a5abad1f392af0/k8s/pkg/apis/longhorn/v1beta2/engine.go#L176) CRD. It will sum up all snapshots count and size for each Volume and check whether the total count or size is over the `snapshotMaxCount` or `snapshotMaxSize` limitation.

From Harvester perspective, it can't freeze the disk before taking snapshots, so a VMBackup request may pass the webhook check but fail to take snapshots.

### Test plan

1. `snapshot-max-size` setting.
    - Validate the value should be between `2` to `250`. After the value is updated in Harvester, the related LH setting should be updated.
    - Create a VM with PVC with empty `harvesterhci.io/snapshotMaxCount` and mutator should replace the value with `snapshot-max-count` setting value.
    - Create a VM with PVC with nonempty `harvesterhci.io/snapshotMaxCount` and mutator shouldn't update the value.
2. A VM with two `20G` PVC with `2` snapshot max count and `0` snapshot max size.
    - Create the first VMBackup. It should be successful.
    - Create the second VMBackup. It should be successful.
    - Create the third VMBackup. It should be failed.
    - Delete a snapshot and create a new snapshot. It should be successful.
3. A VM with two `20G` PVC with `250` snapshot max count and `45G` snapshot max size. Assumed OS installation takes `5G` on the first PVC.
    - Write `5G` data to the second PVC and create the first snapshot. It should be successful.
    - Write `20G` data to each PVC and create the second snapshot. It should be successful.
    - Write `20G` data to each PVC and create the third snapshot. It should be successful.
    - Write `20G` data to each PVC and create the fourth snapshot. It should be failed.
    - Delete the second or third snapshot and create a new snapshot. It should be successful.
4. Partial failed snapshot case. A VM with two `10G` PVC with `250` snapshot max count and `20G` snapshot max size. Assumed OS installation takes `5G` on the first PVC.
    - Write `10G` data to the second PVC and create the first snapshot. It should be successful.
    - Write `10G` data to the second PVC and create the second snapshot. It should be successful.
    - Write `10G` data to the second PVC and create the third snapshot. It should be failed.
    - Delete the first or second snapshot and create a new snapshot. It should be successful.

### Upgrade strategy

None

## Note [optional]

None
