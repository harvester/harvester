# Baremetal GPU workload suport

## Summary

Currently Harvester only allows consuming GPU's via VM workloads. The HEP details the changes needed to Harvester to allow users to consume GPU's via container workloads on the Harvester baremetal cluster.

### Related Issues

https://github.com/harvester/harvester/issues/5820

## Motivation

### Goals

Allow Harvester users to run container workloads that can directly consume NVIDIA GPUs


### User Stories

#### Story 1: Allow user baremetal workloads to consume GPU resources
As a user, I want users to be able to run workloads needing GPU access directly on the Harvester cluster.


### User Experience In Detail

The user can use a combination of node labels to split the Harvester cluster into nodes capable of running container based GPU workloads.

For example, in a Harvester cluster with 3 nodes supporting [vGPUs](https://docs.harvesterhci.io/v1.8/advanced/vgpusupport). The user can split the cluster into 1 node or 2 nodes dedicated for running containerized GPU workloads. 

To achieve this, the user will need to perform the following actions:
* Add label `harvesterhci.io/gpu-baremetal-workloads: "true"` to each node dedicated for running containerized GPU workloads.
* Add the following labels to nodes dedicated for `vGPU` based workloads
```
    nvidia.com/gpu.deploy.container-toolkit: "false"
    nvidia.com/gpu.deploy.dcgm: "false"
    nvidia.com/gpu.deploy.dcgm-exporter: "false"
    nvidia.com/gpu.deploy.device-plugin: "false"
    nvidia.com/gpu.deploy.driver: "false"
    nvidia.com/gpu.deploy.gpu-feature-discovery: "false"
    nvidia.com/gpu.deploy.node-status-exporter: "false"
    nvidia.com/gpu.deploy.operator-validator: "false"
```
* Install GPU operator directly on the harvester cluster
```yaml
apiVersion: helm.cattle.io/v1
kind: HelmChart
metadata:
  name: gpu-operator
  namespace: kube-system
spec:
  repo: https://helm.ngc.nvidia.com/nvidia
  chart: gpu-operator
  version: v26.3.2
  targetNamespace: gpu-operator
  createNamespace: true
  valuesContent: |-
    cdi:
      nriPluginEnabled: true
    driver:
      repository: registry.suse.com/third-party/nvidia
      usePrecompiled: true
      version: 595 # This depends on the nvidia driver that works with your GPU architecture
```

Once GPU operator is installed, due to the additional labels on `vGPU` nodes, the operator will only target the nodes dedicated to running containerized workloads.

Users can now run baremetal GPU workloads directly on the Harvester cluster

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nbody-gpu-benchmark
  namespace: default
spec:
  restartPolicy: OnFailure
  containers:
  - name: cuda-container
    image: nvcr.io/nvidia/k8s/cuda-sample:nbody
    args: ["nbody", "-gpu", "-benchmark"]
    resources:
      limits:
        nvidia.com/gpu: 1
```
### API changes

There are no new API introduced as part of this change.

## Design

### Implementation Overview

To support GPU operator the following changes are needed:

#### nvidia-driver-runtime chart:
The nvidia-driver-runtime chart needs change to skip nodes with the new label `harvesterhci.io/gpu-baremetal-workloads: "true"`
This is to ensure that only the GPU operator managed driver is installed on nodes identified for containerized workloads

Related PR: https://github.com/harvester/charts/pull/563

#### pcidevices controller:
The pcidevices controller needs changes to ensure nodes with the new label `harvesterhci.io/gpu-baremetal-workloads: "true"` are skipped during reconcile from managing `SRIOVGPUDevice` and `vGPUDevice` objects. On an existing cluster, applying the label will result in existing `SRIOVGPUDevice` and `vGPUDevice` objects being removed.

Related PR: https://github.com/harvester/pcidevices/pull/260

A validating webhook change is needed to track GPU's on a per node level. This information can then be used to block creating PCIDeviceClaims for NVIDIA GPU's on nodes which are configure for baremetal workloads.

Related PR: https://github.com/harvester/pcidevices/pull/296

### Test plan

#### Verify consuming GPUs with VMs and containerized workloads
* Install Harvester to 2 or node cluster, where atleast 2 nodes have supported GPU's
* Use the label `harvesterhci.io/gpu-baremetal-workloads: "true"` to dedicate nodes for running containerized GPU workloads
* Apply the labels below to nodes running on `vGPU` workloads
```
   kubectl label node $NODENAME nvidia.com/gpu.deploy.operands=false
```
* Install GPU operator directly on the harvester cluster
```yaml
apiVersion: helm.cattle.io/v1
kind: HelmChart
metadata:
  name: gpu-operator
  namespace: kube-system
spec:
  repo: https://helm.ngc.nvidia.com/nvidia
  chart: gpu-operator
  version: v26.3.2
  targetNamespace: gpu-operator
  createNamespace: true
  valuesContent: |-
    cdi:
      nriPluginEnabled: true
    driver:
      repository: registry.suse.com/third-party/nvidia
      usePrecompiled: true
      version: 595 # This depends on the nvidia driver that works with your GPU architecture
``` 
* Wait for GPU workloads to be deployed
* Run containerized workloads needing GPU workloads

### Upgrade strategy

No changes to upgrades. The addon updates for nvidia-driver-runtime and pcidevices will be rolled out as part of the upgrade. There will be no change in behaviour of these addons unless user labels the nodes `harvesterhci.io/gpu-baremetal-workloads: "true"` to isolate them for containerized GPU workloads.

## Note [optional]

The list of labels needed for skipping nodes with `vGPU` workloads is identified from https://github.com/NVIDIA/gpu-operator/blob/main/controllers/state_manager.go#L88
