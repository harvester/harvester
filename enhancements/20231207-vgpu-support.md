# VGPU Support


## Summary

As of Harvester v1.1.x, support for Peripheral Component Interconnect (PCI) device passthrough to workload VMs has been introduced.  

Building on top of this functionality, we can now support for NVIDIA vGPUs.

### Related Issues

https://github.com/harvester/harvester/issues/2764

## Motivation

### Goals

- Enable deployment of NVIDIA Grid driver via an Addon

- Allow users to enable / disable vGPU on NVIDIA GPUs

- Configure vGPU using available profiles based on underlying GPU type

- Allow usage of vGPU in VM workloads

### Non-goals [optional]

- Shipping NVIDIA drivers. 
- License management for guests.

## Proposal

### User Stories

#### Configure and passthrough vGPU devices to VMs
A Harvester user wants to leverage vGPU capability on supported NVIDIA GPU devices and share vGPUs with VM workloads.

Currently configuring and passing through vGPU devices in a cluster is not possible as the user can only passthrough the entire GPU to the VM workload. This can result in less efficient resource utilisation of GPU instances.

Once this change is implemented, the user will be able to browse SR-IOV capable NVIDIA GPU devices on the cluster. The user can then enable / disable vGPUs on said GPU. The newly created vGPU devices will appear as a new CRD, with information about all the possible vGPU profiles supported by the vGPU. 

The user can now enable / disable vGPUs by defining the correct profile available from list of possible profiles. Once the vGPU is setup, it can be passed through to VM for use by workloads.

### API changes
No API changes will be introduced to core Harvester. The additional CRDs will be managed by the PCI devices controller, as a result the end users will need to enable the PCI devices controller addon to leverage this capability.

In addition we will also be shipping a `nvidia-driver-toolkit` addon. Users need to enable this addon, and provide info about NVIDIA driver location for driver to be installed at runtime.

## Design

The controller introduces two new CRDs:
- sriovgpudevices.devices.harvesterhci.io
- vgpudevices.devices.harvesterhci.io

### Implementation Overview

When deployed, the PCI devices controller addon runs a daemonset on the cluster.

One of the reconcile loops in the cluster scans the nodes for PCI devices at a fixed interval (30 seconds).

As part of this reconcile it will scan for GPU-  devices, and check the following:
* NVIDIA GPU devices which are capable of supporting SRIOV vGPUs are scanned and represented as `sriovgpudevices.devices.harvesterhci.io` objects

A sample device is as follows:

```yaml
apiVersion: devices.harvesterhci.io/v1beta1
kind: SRIOVGPUDevice
metadata:
  creationTimestamp: "2023-11-03T03:47:42Z"
  generation: 8
  labels:
    nodename: harvester-82cd7
  name: harvester-82cd7-000008000
  resourceVersion: "29445307"
  uid: 18e89637-73d9-4bed-9f5c-68739c336b27
spec:
  address: "0000:08:00.0"
  enabled: false
  nodeName: harvester-82cd7
```

Users can now edit the `SRIOVGPUDevice` and `enable` vGPUs. This will trigger the execution of NVIDIA specific tooling via the `nvidia-driver-toolkit` to enable vGPUs on underlying GPU device.

The `VGPUDevice` name and VF addresses are recorded in the status of the `SRIOVGPUDevice` when the operation is successful.

```yaml
apiVersion: devices.harvesterhci.io/v1beta1
kind: SRIOVGPUDevice
metadata:
  creationTimestamp: "2023-11-03T03:47:42Z"
  generation: 8
  labels:
    nodename: harvester-82cd7
  name: harvester-82cd7-000008000
  resourceVersion: "29445307"
  uid: 18e89637-73d9-4bed-9f5c-68739c336b27
spec:
  address: "0000:08:00.0"
  enabled: false
  nodeName: harvester-82cd7
status:
  vGPUDevices:
  - harvester-82cd7-000008004
  - harvester-82cd7-000008005
  - harvester-82cd7-000008016
  - harvester-82cd7-000008017
  - harvester-82cd7-000008020
  - harvester-82cd7-000008021
  - harvester-82cd7-000008022
  - harvester-82cd7-000008023
  - harvester-82cd7-000008006
  - harvester-82cd7-000008007
  - harvester-82cd7-000008010
  - harvester-82cd7-000008011
  - harvester-82cd7-000008012
  - harvester-82cd7-000008013
  - harvester-82cd7-000008014
  - harvester-82cd7-000008015
  vfAddresses:
  - "0000:08:00.4"
  - "0000:08:00.5"
  - "0000:08:01.6"
  - "0000:08:01.7"
  - "0000:08:02.0"
  - "0000:08:02.1"
  - "0000:08:02.2"
  - "0000:08:02.3"
  - "0000:08:00.6"
  - "0000:08:00.7"
  - "0000:08:01.0"
  - "0000:08:01.1"
  - "0000:08:01.2"
  - "0000:08:01.3"
  - "0000:08:01.4"
  - "0000:08:01.5"
```


The pcidevices-controller will now on its next scan detect the newly created vGPU devices, and create corresponding `VGPUDevice` objects

```
NAME                        ADDRESS        NODE NAME         ENABLED   UUID   VGPUTYPE   PARENTGPUDEVICE
harvester-82cd7-000008004   0000:08:00.4   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008005   0000:08:00.5   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008006   0000:08:00.6   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008007   0000:08:00.7   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008010   0000:08:01.0   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008011   0000:08:01.1   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008012   0000:08:01.2   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008013   0000:08:01.3   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008014   0000:08:01.4   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008015   0000:08:01.5   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008016   0000:08:01.6   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008017   0000:08:01.7   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008020   0000:08:02.0   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008021   0000:08:02.1   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008022   0000:08:02.2   harvester-82cd7   false                       0000:08:00.0
harvester-82cd7-000008023   0000:08:02.3   harvester-82cd7   false                       0000:08:00.0
```

A sample `VGPUDevice` object looks as follows:

```yaml
apiVersion: devices.harvesterhci.io/v1beta1
kind: VGPUDevice
metadata:
  creationTimestamp: "2023-11-28T00:41:14Z"
  generation: 1
  labels:
    harvesterhci.io/parentSRIOVGPUDevice: harvester-82cd7-000008000
    nodename: harvester-82cd7
  name: harvester-82cd7-000008021
  resourceVersion: "40796789"
  uid: c724275b-f6a3-4414-a28c-29f0adabad8d
spec:
  address: "0000:08:02.1"
  enabled: false
  nodeName: harvester-82cd7
  parentGPUDeviceAddress: "0000:08:00.0"
  vGPUTypeName: ""
status:
  availableTypes:
    NVIDIA A2-2A: nvidia-750
    NVIDIA A2-2B: nvidia-743
    NVIDIA A2-2Q: nvidia-745
    NVIDIA A2-4A: nvidia-751
    NVIDIA A2-4C: nvidia-754
    NVIDIA A2-4Q: nvidia-746
    NVIDIA A2-8A: nvidia-752
    NVIDIA A2-8C: nvidia-755
    NVIDIA A2-8Q: nvidia-747
    NVIDIA A2-16A: nvidia-753
    NVIDIA A2-16C: nvidia-756
    NVIDIA A2-16Q: nvidia-748
    NVIDIA A2-1A: nvidia-749
    NVIDIA A2-1B: nvidia-742
    NVIDIA A2-1Q: nvidia-744
```

The `VGPUDevice` status contains the information about `availableTypes` which represents the available profiles available for the vGPU device based on the parent GPU device. 

Users need to refer to the GPU documentation from NVIDIA to understand the various types available and capability of each profile.

A user can now enable a `VGPUDevice` with a specific profile by editing the `VGPUDevice` object

When a `VGPUDevice` is enabled via the correct profile, the pcidevices-controller performs the following changes:
* sets up the mdev device for vGPU
* updates kubevirt CR to define the correct `mediatedDevices` definition in `permittedHostDevices` section
* sets up the device plugin to advertise the vGPU for VM workload scheduling
* it recalcuates all other `VGPUDevice` available types to ensure they now correctly reflect the subset of vGPU profiles possible

For example a configure `VGPUDevice` definition looks as follows:

```yaml
apiVersion: devices.harvesterhci.io/v1beta1
kind: VGPUDevice
metadata:
  creationTimestamp: "2023-11-28T00:41:11Z"
  generation: 4
  labels:
    harvesterhci.io/parentSRIOVGPUDevice: harvester-82cd7-000008000
    nodename: harvester-82cd7
  name: harvester-82cd7-000008004
  resourceVersion: "40799301"
  uid: 8897c831-a5dc-4820-8110-75378c4c6cc1
spec:
  address: "0000:08:00.4"
  enabled: true
  nodeName: harvester-82cd7
  parentGPUDeviceAddress: "0000:08:00.0"
  vGPUTypeName: NVIDIA A2-8Q
status:
  configureVGPUTypeName: NVIDIA A2-8Q
  uuid: 5b5b7326-6bb8-48cb-a321-8fa58ccbb38c
  vGPUStatus: vGPUConfigured
```

Non configured `VGPUDevice` are updated to reflect the remaining availableTypes in the status

```yaml
apiVersion: devices.harvesterhci.io/v1beta1
kind: VGPUDevice
metadata:
  creationTimestamp: "2023-11-28T00:41:14Z"
  generation: 1
  labels:
    harvesterhci.io/parentSRIOVGPUDevice: harvester-82cd7-000008000
    nodename: harvester-82cd7
  name: harvester-82cd7-000008021
  resourceVersion: "40799285"
  uid: c724275b-f6a3-4414-a28c-29f0adabad8d
spec:
  address: "0000:08:02.1"
  enabled: false
  nodeName: harvester-82cd7
  parentGPUDeviceAddress: "0000:08:00.0"
  vGPUTypeName: ""
status:
  availableTypes:
    NVIDIA A2-8A: nvidia-752
    NVIDIA A2-8C: nvidia-755
    NVIDIA A2-8Q: nvidia-747
```

Changes have been introduced to pcidevices-controller webhook to ensure the following:
* `SRIOVGPUDevice` cannot be disabled if any `VGPUDevice` is enabled
* `VGPUDevice` cannot use a profile not available in the list of `availableTypes` in the status subresource

### Test plan

Integration test plan.

### Upgrade strategy

Anything that requires if user want to upgrade to this enhancement

## Note [optional]

UI changes are needed to provide a better UX.
- Add a new **SR-IOV GPU Management** page. This should only be enabled when both `pcidevices-controller` and `nvidia-driver-toolkit` addon are deployed successfully
- Add a new **VGPU Management** page, which allows users to enable/disable a vGPU by defining the correct profile.
- Extra form element in the VM page, allowing users to passthrough a vGPU to a VM
- Users can filter vGPU devices by parent GPU based on a label already present in the `VGPUDevice`
- Users can allocate vGPU devices when creating k8s clusters via Rancher

Machine driver changes are needed to allow passing a list of vGPU devices when provisioning VMs via Rancher

Terraform provider changes are also needed to creation of VMs with vGPU devices