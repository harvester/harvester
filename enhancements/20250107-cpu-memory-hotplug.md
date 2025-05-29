# CPU / Memory Hotplug

## Summary

This enhancement focues on leveraging KubeVirt's CPU and Memory hotplug feature to enable users to dynamically adjust the CPU and Memory resources of a VM.

### Related Issues

https://github.com/harvester/harvester/issues/5835

## Motivation

### Goals

- Users can adjust the CPU and Memory resources of a VM dynamically.
- Users can set maximum CPU and Memory resources for a VM. The dynamic adjustment of CPU and Memory resources will be limited by the maximum resources.

### Non-goals

N/A

## Proposal

The cpu / memory hotplug feature will be enabled by default. When creating a VM, users can set 4 fields: initial cpu / memory and maximum cpu / memory. The initial cpu / memory will be the initial resources allocated to the VM. The maximum cpu / memory will be the maximum resources that the VM can be adjusted to.

When the VM is running, users can adjust the CPU and Memory resources of the VM. The adjustment will be limited by the maximum resources set when creating the VM. However, users cannot adjust the CPU and Memory resources to be less than the current resources, because it's not allowed in KubeVirt.

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

#### MacAddress Patch

To keep the MacAddress on a VM, the `VMController.SetDefaultManagementNetworkMacAddress` updates VM after VMI is running. This makes KubeVirt controller addes a condition `RestartRequired` to the VM, so the VM can't do cpu / memory hotplug ([ref](https://github.com/kubevirt/kubevirt/blob/4366d2810a6b59d1b9d4c3fdcc052cf78855a385/pkg/virt-controller/watch/vm/vm.go#L3334-L3354)).

To fix this, the controller will not directly update the VM `spec` field when VM is running, but update the information to VM annotations. The controller will detect whether the VMI is shutdown, and then update the information to VM `spec` field. This controller should be able to cover all cases like soft reboot, restart, running strategy change, etc.

#### Initial CPU and Memory

Currently, when creating a VM by UI, the initial CPU and Memory resources are set to `resources.limits`. However, these fields are immutable, so we need to set the initial CPU to `template.spec.domain.cpu.sockets` and the initial Memory to `template.spec.domain.memory.guest`.

There are 3 fields in `template.spec.domain.cpu`: `cores`, `sockets`, and `threads`. Set CPU to `sockets`, but not `cores`, is because KubeVirt only detects the `sockets` field when doing CPU hotplug ([ref](https://github.com/kubevirt/kubevirt/blob/4366d2810a6b59d1b9d4c3fdcc052cf78855a385/pkg/virt-controller/watch/vm/vm.go#L690-L706)).

#### Maximum CPU and Memory

The maximum CPU and Memory resources are set to `template.spec.domain.cpu.maxSockets` and `template.spec.domain.memory.maxGuest` respectively. Same as above, we will not set these values to `requests.limits` because they are immutable.

#### CPU and Memory Hotplug Action

The webhook server should deny following requests:
* new `sockets` is bigger than `maxSockets`
* new `sockets` is smaller than current `sockets`
* new `memory` is bigger than `maxGuest`
* new `memory` is smaller is bigger than current `memory`

The KubeVirt webhook doesn't handle these cases. It allows these requests but setting `RestartRequired` condition on the VM. This will make the VM can't do next CPU and Memory Hotplug, so we should handle these cases in the Harvester webhook server.

### Test plan

Integration test plan.

### Upgrade strategy

The CPU and Memory hotplug feature will be enabled for new VMs only, because the initial CPU and Memory resources were set to `resources.limits` in the old system.

## Note

N/A
