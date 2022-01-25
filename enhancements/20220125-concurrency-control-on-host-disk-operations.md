# Concurrency Control on Host Disk Operations

## Summary

Some operations, such as formatting, might take a while for operating system to
finish. It causes problems if the controller of host disk manager re-enters
these criticial sections. To prevent re-entrance, additional disk management of
Harvester needs a robust concurrency control on host disk operations.

### Related Issues

- Original bug report: https://github.com/harvester/harvester/issues/1718
- Previous PR: https://github.com/harvester/node-disk-manager/pull/23

## Motivation

### Goals

- Prevent criticial disk operations from re-entrances.
- Control maximum count of concurrent disk operations within a time period.
- Refactor the logic of blockdevice reconcilation to a more deterministic,
  readable, and maintainable state.

### Non-goals

- Auto-recovering from disk operation failures is not in the scope. Node disk
  manager does not know how to deal with this kind of failures (might due to 
  hardware or OS failures). User should come in and tackle the issue manually
  before re-triggering any futher operation.

## Proposal

This proposal is originally for tackling [issue #1718]. During the investigation
of that issue, we found several potential flaws in the current implementation:

1. After introduced [feature auto-provision], user is able to format and
   provision a bunch of raw block device at once. And that leads to several 
   race conditions between each format or provision operation, which might fail
   eventually and make disks result in a wrong state.
2. Operations like formatting a disk may take time. When `OnBlockDeviceChange`
   has been called the first time and the disk started formatting, and later it
   was called the second time but the former formatting has not yet finished,
   this kind of re-entrance problem confuses both operating system and our 
   custom blockdeivce controller and finally goes stuck in a incorrect state.
3. All the logic of formatting/partitioning/mounting/provisioning are crammed 
   into the single `OnBlockDeviceChange` function. That makes debugging and
   unit-testing nearly impossible. The maintenance cost are still stacking high.

For the first two flaws above, I propose two general concurrency control to
resolve: lock mechanism and job queue. For the last one, we plan to refactor
the controller logic to a state-machine-like pattern to enhance maintainability.

[issue #1718]: https://github.com/harvester/harvester/issues/1718
[feature auto-provision]: https://github.com/harvester/node-disk-manager/pull/17

### User Stories

#### `auto-provision` lots of raw block devices with large volume size

Before this enhancement, user might encounter undefined behavior due to race
condition of disk formatting re-entrances. Node disk manager sometimes auto 
heals from this situation but often it fails and leaves blockdevice custom
resoures in incorrect states.

After this enhancement, the blockdevice custom resource first enters "verb-ing"
phase. further subsequent changes won't take effect until the disk operations
completes. The lock and job queue mechanism is nearly unawared from the user
perspective.

#### Easier troubleshooting and retriggering

Before this enhancement, the status of block device custom resource is entangled
and difficult for users to interpret. Even developers cannot have a clear view
on their statuses. It's also hard to retrigger disk operations since one cannot
simply translate the actual state of the disk from the status of the blockdevice.

After this enhancement, a state-machine-like phase transition helps developers
see a deterministic path for each phase-to-phase change. Each phase is also a
single source of true that tells user in what phase the blockdevice are. It can
also help retrigger disk operations by deleting the blockdevice resource and it
will run transitions phase-by-phase to the desired phase.

### User Experience In Detail

Basically, user shouldn't aware of this enhancement because this is more like a
internal refactor than a user-facing feature. However, this enhancement plan to
1) deprecate `spec.fileSystem.mountPoint` and 2) allow to format raw block
device directly, so users previous depends on these functionalities will find
they cannot after this enhancement. Thankfully, these changes won't affect any
real user experience on disk provisions, since node disk manager won't persist
mounts for extra disks between boots, and it always re-mounts all extra disk
after reboot.

### API changes

- :pencil2: Add several new phases to `blockdevice.status.provisionPhase`
    - :new: `Formatting`
    - :new: `Formatted`
    - :new: `Mounting`
    - :new: `Mounted`
    - :new: `Unmounting`
    - :new: `Provisioning`
    - :new: `Failed`
- :pencil2:  Changes on `blockdevice.status.conditions` for logging timestamp w
  when performing each type of disk operations:
    - :new: `Formatted` indicates the device has been formatted.
    - :new: `Failed` indicates the disk operations failed.
    - :new: `AutoProvisionDetected` indicates the disk is added by 
      auto-provision mechanism
    - :pencil2: Rename `AddedToNode` to `Provisioned`: indicates the device is
      served as a disk on a longhorn node.
- :x: `blockdevice.spec.fileSystem.mountPoint` is deprecated. Setting this won't 
  affect anything.
- :x: `blockdevice.status.fileSystem.lastFormattedAt` is removed
- Now node disk manager can format and provision raw block device directly
  without any partitions. It won't create a single partition for raw block
  provisioning anymore.

## Design

### Implementation Overview

As aforementioned, this enhancement introduces three important parts:

1. Lock mechanism: Prevent re-entrance to criticial disk operations.
2. Job queue: Control maximun count of concurrent disk operation.
3. State machine pattern: Consolidate the transitions between disk states/phases.

We'll explain each parts in that order.

#### Lock mechanism

This is the most important part of the enhancement. Consider the following
state transition of a block device:

![](https://user-images.githubusercontent.com/14314532/150992612-afd4c454-5440-4c60-814b-071a3c05f6ae.png)

1. Gorountine 1 starts formatting the disk first.
2. During Goroutine 1 formatting, something triggers Goroutine 2 start formatting.
3. Goroutine 1 thinks it finishes formatting, so perform a mount.
4. Goroutine 1 fails to mount (still formatting by Goroutine 2)
5. Goroutine 2 fails to update the state because the blockdevice is in failed state.

Thankfully, we can leverage Kubernetes optimistic locking mechanism consistency
model [`resourceVersion`] field in each resource object. This mechanism is
usually guaranteed [etcd's consistency model]. Therefore, before a blockdevice
enters criticial section of disk operation like formatting, the controller can
patch the resource with a `Verb-ing` state to deliberately increase the resource
version as an optimistic lock.

![](https://user-images.githubusercontent.com/14314532/150992469-62b92e85-d565-40c0-b6f4-29c645df3352.png)

In conclusion, the enhancement propose to add the following state as optimistic
lock, since they interact with the operating system or other components:

- `Formatting`
- `Mounting`
- `Umounting`
- `Provisioning`
- `Unprovisioning`

[`resourceVersion`]: https://kubernetes.io/docs/reference/using-api/api-concepts/#resourceversion-in-metadata
[etcd's consistency model]: https://etcd.io/docs/v3.5/learning/api_guarantees/#isolation-level-and-consistency-of-replicas

#### Job queue

The lock mechanism seems pretty great, but actually it has a flaw: if the whole
process crashes, the blockdevice will be stuck in `Verb-ing` state indefinitely.
User can only edit it manually to resolve it. 

To deal with this situation, we could use [workqueue] package, provided by
Kubernetes client-go library, to build a queue to store items processed.

The basic flow is like the following:

1. The queue is created only once during the initialization of blockdevice controller.
    - The queue will enqueue all blockdevice resources in `Verb-ing` to 
      re-process them. This can solve the problem that the blockdevice is stuck
      in `Verb-ing` when the host reboot.
2. Run a goroutine for workqueue and block it to wait items to be processed.
3. Blockdevice controller: When trying to perform disk operation
    1. Patch the resource with `Verb-ing` state as the optimistic lock
    2. Enqueue the resource to workqueue
4. Workqueue goroutine
    1. Actually performs the disk operation
    2. Report the result back by patching the resource. If the operation timeout,
       report it as `Failed`.

Note that for the first step, there are two reasons

The workqueue package also provides a convenience method [`WithChunkSize`].
The method allows to set chunks of work items instead of processing them one
by one. This can resolve the "maximum count of concurrent disk operations" 
issue as we mentioned earlier.

[workqueue]: https://pkg.go.dev/k8s.io/client-go/util/workqueue
[`WithChunkSize`]: https://pkg.go.dev/k8s.io/client-go/util/workqueue#WithChunkSize

#### State machine pattern

The state machine pattern is primarily a rewrite of the messy contrller logic.
It contains only one major component: transition table.

Transition stable is struct that maps each state to a transition method which
produces the next state and disk operation to be perform. The controller can
leverage this transition table to get the next state and then patch the resource
accordingly. The disk operation returned by the transition method is then added
to workqueue along with the patched resource to be processed by workqueue.

```go
// The integration of transition table and controller
func (c Ctrl) onChange(bd *Blockdevice) {
   nextPhase, operation, err := c.transitionTable.next(bd)
   bd.phase :=  nextPhase
   // Update the resource to increase resourceVersion as an optimistic locking.
   newBd := c.updateBlockDevice(bd)
   // Enqeue to workqueue if there is any operation to perform.
   if operation != nil {
      item := itemToProcess{
        bd: newBd,
        op: operation,
      }
      c.workqueue.add(item)
   }
}
```

```go
type transitionTable struct {}

func (t transitionTable) phaseUnprovisioned(bd *Blockdevice) (Phase, Operation, error) {
  if needForceFormatted {
    return PhaseFormatting, OperationFormatDisk, nil
  }
  // No need to change. Return current phase.
  return bd.phase, nil, nil
}

func (t transitionTable) phaseFormatting(bd *Blockdevice) (Phase, Operation, error) {
  // This phase acts as optimisitic lock. No need to change.
  // OperationFormatDisk will update the phase after its execution.
  return PhaseFormatting, nil, nil
}

func (t transitionTable) phaseFormatted(bd *Blockdevice) (Phase, Operation, error) {
  if needProvisioned {
    return PhaseMounting, OperationMountFileSystem, nil
  }
  // No need to change. Return current phase.
  return bd.phase, nil, nil
}
```

By implementing the state machine pattern, the phase transitions is easiler to 
track and unit-testable to some extent.

The full state machine diagram might be like the following but is subject to change:

![](https://user-images.githubusercontent.com/14314532/148892960-d772d18a-d1d6-49db-9947-674b9d31c5f6.png)

### Test plan

TODO: Integration test plan.

### Upgrade strategy

TODO: Anything that requires if user want to upgrade to this enhancement

## Note [optional]

Additional nodes.
