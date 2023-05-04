# Freeze FS during VM backup creation

## Summary

This proposal introduces changes to Harvester to ensure the file system freeze on guests when a backup is performed.

Currently, when a user initiates a VM backup, no guest-level action is taken to ensure the file systems are synced, or I/O is paused. This can lead to an inconsistent backup if the backup is triggered during a high I/O event.

### Related Issues

[GH issue 1723 ](https://github.com/harvester/harvester/issues/1723)

### Goals

- Attempt to perform file system freeze if qemu-guest-agent is installed in the guest when a backup is triggered.

## Proposal

### User Stories

#### Need for consistent backups for VM workloads
#####  User Experience In Detail
A VM backup initiated during times of high IO, can be inconsistent. This can be due to the fact the backup controller doesn't attempt to pause fs io when a backup is performed.

### API changes
No harvester API changes are needed. The changes will only be in the backup controller logic.

## Design

### Implementation Overview

The changes need to be implemented in the harvester [backup controller](https://github.com/harvester/harvester/blob/master/pkg/controller/master/backup/backup.go)

Additional logic will be needed to check the following:
* check if qemu-guest-agent is available, this will be available via vmi status by checking if condition for GuestAgent connected is true
```yaml
  conditions:
  - lastProbeTime: null
    lastTransitionTime: "2023-05-04T00:41:03Z"
    status: "True"
    type: Ready
  - lastProbeTime: null
    lastTransitionTime: null
    status: "True"
    type: LiveMigratable
  - lastProbeTime: "2023-05-04T00:41:23Z"
    lastTransitionTime: null
    status: "True"
    type: AgentConnected
```

* if guest agent is present, then [reconcileVolumeSnapshots](https://github.com/harvester/harvester/blob/master/pkg/controller/master/backup/backup.go#L443) could trigger a fs freeze on the VM using [freeze/unfreeze subresource api](https://github.com/kubevirt/kubevirt/blob/main/docs/freeze.md) with a small unfreezeTimeout.

* volume backups proceed as expected.
### Test plan

* Run VM backups for VM's with and without guest agent. There should be no impact to observed VM backup functionality

* Launch VM, and ensure qemu-guest-agent install instructions are removed from Harvester generated cloud-config. Once VM is running, simulate I/O while triggering vm backup. Restore VM backup, it is likely to be inconsistent.

* Ensure qemu-guest-agent is connected. Run I/O during while triggering VM backup. Restore VM backup and it should be consistent.

### Upgrade strategy

No further action needed. The upgrade will rolled to clusters as part of the harvester upgrade.

## Note [optional]

None
