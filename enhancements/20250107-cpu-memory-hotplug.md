# CPU / Memory Hotplug

## Summary

This enhancement focuses on leveraging KubeVirt's CPU and Memory hotplug feature to enable users to dynamically adjust the CPU and Memory resources of a VM.

### Related Issues

https://github.com/harvester/harvester/issues/5835

## Motivation

### Goals

- Users can adjust the CPU and Memory resources of a VM dynamically.
- Users can set maximum CPU and Memory resources for a VM. The dynamic adjustment of CPU and Memory resources will be limited by the maximum resources.
- Old VMs will not be affected by this change.

### Non-goals

N/A

## Proposal

The cpu / memory hotplug feature will be enabled by adding annotation `harvesterhci.io/enableCPUAndMemoryHotplug`. When creating a VM, users can set 4 fields: initial cpu / memory and maximum cpu / memory. The initial cpu / memory will be set to `domain.cpu.sockets` and `domain.memory.guest`. The maximum cpu / memory will be set to `domain.cpu.maxSockets`, `resources.limits.cpu` and `domain.memory.maxGuest`, `resources.limits.memory`.

When the VM is running, users can adjust the CPU and Memory resources of the VM. The adjustment will be limited by the maximum resources.

### User Stories

#### Story 1 - Vertical Scaling a VM without downtime

There is a legacy workload which is not actively developed. The workload was not designed as a distributed system, so it's hard to scale out. Recently, there are some users using this application, and the workload is getting heavier. The current resources allocated to the VM are not enough to handle the workload. The administration wants to adjust the CPU and Memory resources of the VM without downtime.

### User Experience In Detail

1. Prepare an Image.

2. On the VM creation dashboard, click the `Enable CPU / Memory Hotplug` button and the dashboard will show another two fields: Maximum CPU and Maximum Memory.

3. Fulfill the VM creation form and click the `Create` button.

4. After the VM is created, administrators can adjust the CPU and Memory resources by clicking `CPU / Memory Hotplug` on the VM details page.

### API changes

#### Update CPU and Memomory resources of a VM [POST]

Endpoint:  `/v1/harvester/vm/<vm>?action=cpuAndMemoryHotplug`

Request (application/json):
```json
{
  "sockets": 2,
  "memory": "4Gi"
}
```

## Design

### Implementation Overview

#### Initial CPU and Memory

Currently, when creating a VM by UI, the initial CPU and Memory resources are set to `resources.limits`. If there is no annotation `harvesterhci.io/enableCPUAndMemoryHotplug`, the system should follow old pattern to set values. If there is annotation, the system sets the initial CPU to `domain.cpu.sockets` and the initial Memory to `domain.memory.guest`.

There are 3 fields in `template.spec.domain.cpu`: `cores`, `sockets`, and `threads`. Set CPU to `sockets` instead of `cores`, is because KubeVirt only detects the `sockets` field when doing CPU hotplug ([ref](https://github.com/kubevirt/kubevirt/blob/4366d2810a6b59d1b9d4c3fdcc052cf78855a385/pkg/virt-controller/watch/vm/vm.go#L690-L706)).

#### Maximum CPU and Memory

The maximum CPU and Memory resources are set to `domain.cpu.maxSockets`, `resources.limits.cpu` and `domain.memory.maxGuest`, `resources.limits.memory` respectively. These fields are immutable. Adjust the maximum CPU or Memory needs to restart the VM.

#### CPU and Memory Hotplug Action

The API server sends a patch to adjust the CPU and Memory resources of a VM.

Following cases makes a VM cannot do CPU and Memory hotplug. The VM will be marked with `RestartRequired` condition, which means the VM needs to be restarted to apply the changes.

* new `sockets` is smaller than current `sockets`
* new `memory` is smaller is bigger than current `memory`
* modify `maxSockets` or `maxGuest`

### Test plan

* Use `cpuAndMemoryHotplug` action to adjust the CPU and Memory resources of a VM.
  * If `sockets` is bigger than current `sockets` and smaller than `maxSockets`, the VM should be able to adjust the CPU resources dynamically.
  * If `memory` is bigger than current `memory` and smaller than `maxGuest`, the VM should be able to adjust the Memory resources dynamically.
  * If `sockets` is smaller than current `sockets`, the VM should be marked with `RestartRequired` condition.
  * If `memory` is smaller than current `memory`, the VM should be marked with `RestartRequired` condition.
* Modify `maxSockets` or `maxGuest` marks the VM with `RestartRequired` condition.

### Upgrade strategy

The CPU and Memory hotplug feature will be enabled for new VMs only, because the initial CPU and Memory resources were set to `resources.limits` in the old system.

## Note

N/A
