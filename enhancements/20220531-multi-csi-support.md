# Multi CSI support

## Summary

### Related Issues

harvester/harvester#2295

## Motivation

### Goals

If users install other csi drivers and create their storageClasses and volumeSnapshotClasses in Harvester by manually and configure setting `csi-driver-config`, users can use other csi drivers to create `data volumes`.

- if users want to expand volume, the csi driver need to support pvc expand.
- if users want to clone volume, the csi driver need to support pvc clone.
- if users want to backup or snapshot virtual machine, the csi driver need to support pvc snapshot.
- if users want to live migrate virtual machine, the csi driver need to support RWX mode.

### Non-goals [optional]

Use other csi driver to create `image volumes`

## Proposal

### User Stories

User already has other storage providers in their environments, they want to reuse them in Harvester by other csi driver.

### User Experience In Detail

1. Install other csi driver and create their storageClasses and volumeSnapshotClasses in Harvester by manually.
2. Edit setting `csi-driver-config`, add a section for the new added csi driver, configure `volumeSnapshotClassName`, `backupVolumeSnapshotClassName`, `supportClone`, `supportExpand` according the csi driver's capability.
3. Show different actions for volumes according the setting `csi-driver-config` when managing volumes.
4. Select storageClasses when creating empty volume (src=New).
5. Select storageClasses when adding a new empty volume to virtual machine.

### API changes

Add a new pvc action `expand` instead of editing pvc size directly, because not all csi drivers support pvc expand and pvc size decrease is not allowed.

## Design

### Implementation Overview

The default content of setting `csi-driver-config`:
```json
{
  "driver.longhorn.io": {
    "volumeSnapshotClassName": "longhorn-snapshot",
    "backupVolumeSnapshotClassName": "longhorn",
    "supportClone": true,
    "supportExpand": true
  }
}
```

An example setting `csi-driver-config` if user want to use https://github.com/dell/csi-powerflex:
```json
{
  "driver.longhorn.io": {
    "volumeSnapshotClassName": "longhorn-snapshot",
    "backupVolumeSnapshotClassName": "longhorn",
    "supportClone": true,
    "supportExpand": true
  },
  "csi-vxflexos.dellemc.com": {
    "volumeSnapshotClassName": "vxflexos-snapclass",
    "backupVolumeSnapshotClassName": "",
    "supportClone": true,
    "supportExpand": true
  }
}
```

### Test plan

### Upgrade strategy

## Note [optional]
