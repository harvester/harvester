# Support Installing Specific NVIDIA Driver

Enable Harvester to install specific NVIDIA drivers per node by reading node annotations from the Kubernetes API.

## Summary

This enhancement includes modifications to the NVIDIA driver toolkit entrypoint script to query the Kubernetes API for node annotations, and updates to the Helm chart to provide the necessary RBAC permissions for accessing node information.


### Related Issues

https://github.com/harvester/harvester/issues/HARV-8338

## Motivation

### Goals

- Enable per-node NVIDIA driver installation based on node annotations

### Non-goals

- Automatic driver version detection based on GPU hardware

## Proposal

This enhancement allows the NVIDIA driver toolkit to install specific driver versions per node by reading node annotations from the Kubernetes API. The system will look for a specific annotation `sriovgpu.harvesterhci.io/custom-driver` on each node to determine which driver to download.

### User Stories

#### Story 1: Download Specific NVIDIA Driver
A Harvester administrator manages a cluster with different generations of NVIDIA GPUs across nodes. Node A has xxx GPUs that require driver version xxx, while Node B has yyy GPUs that work best with driver version yyy. 

With this enhancement, the administrator can add the annotation to each node with the appropriate driver location:
- Node A: `sriovgpu.harvesterhci.io/custom-driver=https://example.com/drivers/NVIDIA-Linux-x86_64-vgpu-kvm-xxx.run`
- Node B: `sriovgpu.harvesterhci.io/custom-driver=https://example.com/drivers/NVIDIA-Linux-x86_64-vgpu-kvm-yyy.run`

Without the annotation, the nvidia-driver-toolkit will download the default driver version from the addon.

### User Experience In Detail

Users can specify custom NVIDIA drivers by adding the appropriate annotation to nodes with NVIDIA GPUs that require specific driver versions.

### API changes

None.

## Design

### Implementation Overview

The enhancement consists of three main components:

#### 1. Downward API

Although we cannot read node metadata directly with the Downward API, we can retrieve the node name and then fetch node metadata from the Kubernetes API.

```yaml
env:
- name: NODE_NAME
  valueFrom:
    fieldRef:
      apiVersion: v1
      fieldPath: spec.nodeName
```

#### 2. Entrypoint Script

The key is to extract the value of the custom driver annotation from the node's metadata.

#### 3. RBAC Configuration

The RBAC should only have nodes get permission. For principle of least privilege, other permissions are not allowed.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: nvidia-driver-runtime
rules:
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get"]
```

### Test Plan

1. Ensure the default NVIDIA driver is downloaded from the addon when the node doesn't contain the custom driver annotation.
2. Ensure the custom NVIDIA driver is downloaded from the node annotation when the node contains the custom driver annotation.


### Upgrade Strategy

For users who have enabled the add-on, they need to restart the DaemonSet to trigger the download after annotating the node if they want to install different drivers on each node.

## Note

None.