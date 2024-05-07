# VGPU Support, Rancher UI external integration


## Summary

As of Harvester v1.3.x, support for vGPU devices has been introduced.

We can now add support to add vGPU devices from Rancher UI when provisioning Harvester clusters.

### Related Issues

https://github.com/harvester/harvester/issues/2764

### Goals

- Show the list of available vGPUs in Rancher cluster creation page.

- Allow users to assign vGPU devices to a cluster, based on devices availability for each cluster node.

- Show the list of assigned vGPUs in cluster view/edit page.

- Allow users to remove vGPU devices from a Harvester cluster.

## Proposal

### User Stories

#### Enable vGPUs in Harvester cluster and assign them in Rancher

A Harvester user wants to enable vGPUs in Harvester and assign one or more of those devices in clusters provisioning page.

The user is able to browse and enable vGPU profiles in Harvester UI. Each profile is bound to a vGPU type. 
Once a profile is enabled, the profile is visible in the dropdown element in Rancher UI.

The profile's label should contain the vGPU type and a unique key for profiles.

The user can now assign a new profile to the machine pools from a dropdown list, based on profile's availability.
Only available profiles should be displayed in the dropdown list or in alternative, receive a validation message for profiles that are not available.

The profiles availability should be consistent in both single and multi nodes Harvester environments.

Once the cluster is created, the user should be able to see the assigned vGPU devices in the provisioning VMs in Harvester UI.

## Design

We introduce a new UI element:
- Cluster Management -> Create -> RKE2 / Harvester -> Machine Pools -> Advanced -> 'vGPU devices' dropdown.

### Implementation Overview

The vGPU dropdown is populated using `v1` cluster API,

`/k8s/clusters/${ clusterId }/v1/devices.harvesterhci.io.vgpudevice`

filtering the results by `spec.enabled` field

An example of enabled vGPU is as follow:

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
  address: "0000:08:00.5"
  enabled: true
  nodeName: harvester-82cd7
  parentGPUDeviceAddress: "0000:08:00.0"
  vGPUTypeName: NVIDIA A2-2Q
status:
  configureVGPUTypeName: NVIDIA A2-2Q
  uuid: 5b5b7326-6bb8-48cb-a321-8fa58ccbb38c
  vGPUStatus: vGPUConfigured
```

We use the `v1` API to fetch the list on the Harvester nodes and calculate the vGPU availability

`/k8s/clusters/${ clusterId }/v1/nodes`

An example of Harvester node, where `status.allocatable` field contains the list of each VGPUType availability number:

```yaml
id: vgpu-test-1
type: node
apiVersion: v1
kind: Node
metadata:
  name: vgpu-test-1
  ...
spec:
  providerID: rke2://vgpu-test-1
  ...
status:
  allocatable:
    nvidia.com/NVIDIA_A2-2Q: '4'
    nvidia.com/NVIDIA_A2-8Q: '0'
    ...
```

The vGpu availability is calculated as **the sum of allocatable values in each Harvester nodes** for each vGPU type.
It shouldn't be possible to assign a vGPU profile where the vGPU type's availability is less then the number of nodes in each Machine pools.

Requirements:
- The `allocatable` field should be updated after assign vGPU devices so the UI can correctly calculate the remaining number of devices.


The label of each element in the dropdown should be: vGPUTypeName + vGPU profile name + availability.

For example:

```code
"NVIDIA_A2-2Q (profile: vgpu-test-1-000008005 - allocatable: 4)"
"NVIDIA_A2-2Q (profile: vgpu-test-1-000008006 - allocatable: 4)"
...
```

Once the cluster is created, The UI sends the vGPU profiles in the Harvester machine config request,

`/v1/rke-machine-config.cattle.io.harvesterconfigs/fleet-default`

An example of machine config payload:

```yaml
...
cpuCount: '4'
memorySize: '8'
vgpuInfo: '{"vGPURequests":[{"name":"vgpu-test-1-000008004","deviceName":"nvidia.com/NVIDIA_A2-2Q"}]}'
vmNamespace: harvester-public
type: rke-machine-config.cattle.io.harvesterconfig
...
```

where `vgpuInfo` contains the requested vGPU profiles.

When a cluster is created, all the provisioning VMs in Harvester should end up in a Active status.

### Test plan

Integration test plan.

### Upgrade strategy

Anything that requires if user want to upgrade to this enhancement.

## Note [optional]

- We should provide a better display name for dropdown options.
- We should identify the edge cases for both single/multi nodes Harvester environments, especially for the availability checks.
