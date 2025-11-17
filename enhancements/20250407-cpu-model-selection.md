# Selection for CPU models for VMs

## Summary

Harvester doesn't provide the options for selecting CPU models for VMs. This enhancement is to provide a way to select CPU models for VMs.

### Related Issues

https://github.com/harvester/harvester/issues/3015

## Motivation

Primarily, this HEP addresses the limitation of the current default host-model CPU model used by Harvester, where it blocks migrations that are technically legit and possible.

### Goals

- Support per-VM and global CPU model selections.
- Propagate the scheduling error on GUI.

### Non-goals

- Change the underlying KubeVirt CPU model or migration logic.
- Force migration. It should be done [by upstream](https://github.com/kubevirt/kubevirt/issues/15623).
- Support cpu features selection.

## Introduction

Before this enhancement, we need to understand how the CPU model and features work in KubeVirt.

In general, we can use `lscpu` to check CPU model and features in the node.

```
Architecture:                x86_64
  CPU op-mode(s):            32-bit, 64-bit
  Address sizes:             46 bits physical, 48 bits virtual
Vendor ID:                   GenuineIntel
  Model name:                12th Gen Intel(R) Core(TM) i9-12950HX
    Flags:                   fpu vme de pse tsc msr pae ....
```

KubeVirt has a labeler to label the node with above information. Hence, you'll see below information on the node resource.

```yaml
kind: Node
metadata:
  name: my-node-1
  labels:
    host-model-cpu.node.kubevirt.io/SierraForest: "true"
    cpu-model.node.kubevirt.io/IvyBridge: "true"
    cpu-model.node.kubevirt.io/SandyBridge: "true"
    cpu-model-migration.node.kubevirt.io/IvyBridge: "true"
    cpu-model-migration.node.kubevirt.io/SandyBridge: "true"
    cpu-feature.node.kubevirt.io/fpu: "true"
    cpu-feature.node.kubevirt.io/vme: "true"
```

Harvester uses `host-model` as default CPU model. The migration process will be like:

- Start a migration.
- Fill out the nodeSelector in the new POD with
  - A CPU model in `cpu-model-migration.node.kubevirt.io/SierraForest`
  - Some features in `cpu-feature.node.kubevirt.io/xxx`.
- Start the new POD.

That being said, if the CPU Model is from a different generation, the migration might fail with default `host-model`. However, different CPU generations could still have some common CPU models. For example, let's use `IvyBridge` as a common cpu model in different CPU generations. That means if we use `IvyBridge` as the CPU model in the `virtualmachine` spec, this VM can be migrated to another node even though the nodes' CPU generations are different.

## Proposal

Due to multiple nodes, we can't show a big matrix for all the CPU models. Instead, we'll provide a new UI that allows users to select or input the CPU model.

### User Stories

#### Story 1

I have multiple nodes that have a common CPU model called `Nehalem`. For some reason, I'll need to migrate my VMs to other nodes without manually shutting down. To ensure compatibility, I create my VMs with `Nehalem` as the CPU model.

### User Experience In Detail

If the cluster consists of mainly homogeneous nodes (including new nodes), users can define a universal common CPU model to ensure compatible migrations among the nodes.

### API changes

Backend should provide a API to let frontend generate a selection list.

API: `GET /v1/harvester/node?link=getCpuMigrationCapabilities`

```json
{
  "totalNodes": "<total number of ready nodes in the cluster>",
  "globalModels": ["host-model", "host-passthrough", "one from kubevirt spec.configuration.cpuModel"],
  "models": {
    "<cpu-model-name>": {
      "readyCount": "<number of ready nodes that support this CPU model>",
      "migrationSafe": "<boolean: true if readyCount equals totalNodes, false otherwise>"
    }
  }
}
```

**Field Descriptions:**
- `totalNodes`: Total number of ready nodes in the cluster
- `globalModels`: Array of special CPU models that are always available
  - `host-model`: Uses the host's CPU model (default behavior)
  - `host-passthrough`: Passes through the host's CPU features directly
  - KubeVirt configuration CPU model: The CPU model set in `kubevirt.spec.configuration.cpuModel` (if configured)
- `models`: Object containing all available CPU models in the cluster
  - `<cpu-model-name>`: The name of the CPU model (e.g., "IvyBridge", "Penryn")
    - `readyCount`: Number of ready nodes that support this CPU model
      - if `readyCount` is greater than one, that cpu model can be migrated to some nodes in the cluster.
      - if `readyCount` is equal to one, that cpu model can't be migrated.
    - `migrationSafe`: Boolean flag indicating if VM with this CPU model can migrate to any node
      - `true`: CPU model is available for migration on some nodes
      - `false`: CPU model is not available for migration on all nodes

About cache, we won't add it in this version, please check [discussion here](https://github.com/harvester/harvester/pull/7937#discussion_r2479700572).

### Case 1

- Node-1 cpuModel: IvyBridge, Penryn
- Node-2 cpuModel: IvyBridge, Westmere
- Node-3 cpuModel: IvyBridge, SandyBridge
- Node-4 cpuModel: IvyBridge, Westmere


```json
{  
  "totalNodes": 10,
  "globalModels": ["host-model", "host-passthrough"],
  "models": {
    "IvyBridge": {  
      "readyCount": 4,
      "migrationSafe": true  
    },  
    "Penryn": {  
      "readyCount": 1,  
      "migrationSafe": false  
    }  ,
    "Westmere": {  
      "readyCount": 2,  
      "migrationSafe": true  
    }  ,
    "SandyBridge": {  
      "readyCount": 1,  
      "migrationSafe": false  
    }  
  }
}  
```

### Case 2

- Node-1 cpuModel: IvyBridge, Penryn
- Node-2 cpuModel: IvyBridge
- Node-3 cpuModel: SandyBridge
- Node-4 cpuModel: Westmere

```json
{  
  "totalNodes": 4,  
  "globalModels": ["host-model", "host-passthrough"],
  "models": {  
    "IvyBridge": {  
      "readyCount": 2,  
      "migrationSafe": true  
    },  
    "Penryn": {  
      "readyCount": 1,  
      "migrationSafe": false  
    },
    "SandyBridge": {  
      "readyCount": 1,  
      "migrationSafe": false  
    },
    "Westmere": {  
      "readyCount": 1,  
      "migrationSafe": false  
    }
  }
}  
```

### Case 3

- Node-1 cpuModel: IvyBridge, Penryn
- Node-2 cpuModel: IvyBridge, Westmere
- Node-3 cpuModel: IvyBridge, SandyBridge
- Node-4 cpuModel: IvyBridge, Westmere
- KubeVirt configuration: cpuModel is set to "IvyBridge" by users

```json
{  
  "totalNodes": 4,
  "globalModels": ["host-model", "host-passthrough", "IvyBridge"],
  "models": {
    "IvyBridge": {  
      "readyCount": 4,
      "migrationSafe": true  
    },  
    "Penryn": {  
      "readyCount": 1,  
      "migrationSafe": false  
    },
    "Westmere": {  
      "readyCount": 2,  
      "migrationSafe": true  
    },
    "SandyBridge": {  
      "readyCount": 1,  
      "migrationSafe": false  
    }  
  }
}  
```

## Design

The CPU models are like this:

- Node-1 cpuModel: IvyBridge, Penryn
- Node-2 cpuModel: IvyBridge, Westmere
- Node-3 cpuModel: IvyBridge, SandyBridge
- Node-4 cpuModel: IvyBridge, Westmere

Possible UI/UX:

```
CPU Model:
--Global Models--
host-model (default)
host-passthrough
IvyBridge (cluster-wide setting)
--Migratable Models--
IvyBridge
Westmere
--Non-Migratable Models--
SandyBridge
Penryn
```

Note: The UI should group CPU models into three categories:
1. **Global Models**: Special models that are always available (host-model, host-passthrough, and cluster-wide configured model)
2. **Migratable Models**: CPU models with `migrationSafe: true`
3. **Non-Migratable Models**: CPU models with `migrationSafe: false`

### Implementation Overview

#### Frontend

We'll have two ways to configure this.

- Per-VM CPU model while creating the VM
- Global VM CPU model in settings

Frontend needs to create a new tab in the VM creation page and provide a dropdown selection menu for models. This selection is also available in the VM template page and global settings. 


Action Items:

- [ ] Create the new configurations in the "Advance Options" of the VM creation page and the VM template page.
- [ ] Provide a dropdown selection menu for models.
- [ ] Propagate the scheduling error on GUI.

#### Backend

Backend should reject unreasonable requests from the frontend. When users try to migrate a VM, the `findMigratableNodes` action should return available nodes that match the selected CPU model feature.

Action Items:

- [ ] Provide a CPU migration capabilities API for frontend.
- [ ] Filter the nodes based on the selected CPU model when calling `findMigratableNodesByVMI`.
- [ ] Write documentation on different usage of the policy field in the VM spec.
- [ ] Write documentation on how to configure cluster-wide CPU model in KubeVirt.

### Test plan

Ensure users can select CPU model and migrate the VM.

### Upgrade strategy

The current VM uses the default CPU model (host-model). If users would like to change the CPU model, they need to restart the VM.

## Note

### Real World Example of CPU Model and Feature

In order to have a better understanding of how the CPU model and features work in KubeVirt, I'll provide some real spec examples.

Let's say we have a Node with the following CPU models and features:

```yaml
kind: Node
metadata:
  name: my-node-1
  labels:
    host-model-cpu.node.kubevirt.io/Common-CPU: "true"
    cpu-model.node.kubevirt.io/Intel-A: "true"
    cpu-model.node.kubevirt.io/Common-CPU: "true"
    cpu-model-migration.node.kubevirt.io/Intel-A: "true"
    cpu-model-migration.node.kubevirt.io/Common-CPU: "true"
    cpu-feature.node.kubevirt.io/avx2: "true"
    cpu-feature.node.kubevirt.io/sse4_2: "true"
```

- Scenario 1: VM with default model

    A VM spec requires the following CPU models and features:

    ```yaml
    kind: VirtualMachine
    metadata:
      name: my-vm-1
    spec:
      template:
        spec:
          domain:
            cpu:
              model: host-model # Default. You could ignore this line as well.
    ```
    
    The Pod spec will be like this after first migration:
    
    ```yaml
    kind: Pod
    metadata:
    name: my-vm-1-pod
    spec:
    nodeSelector:
        cpu-model-migration.node.kubevirt.io/Common-CPU: "true"
        cpu-feature.node.kubevirt.io/avx2: "true"
        cpu-feature.node.kubevirt.io/sse4_2: "true"
    ```
    
    BTW, this one is before the migration:

    ```yaml
    kind: Pod
    metadata:
    name: my-vm-1-pod
    spec:
    nodeSelector:
        # yes, there are no any selectors here
    ```

- Scenario 2: VM with specific model

    ```yaml
    kind: VirtualMachine
    name: my-vm-2
    spec:
      template:
        spec:
          domain:
          cpu:
            model: Common-CPU
    ```
    
    The Pod spec will be like this:
    
    ```yaml
    kind: Pod
    metadata:
    name: my-vm-2-pod
    spec:
        nodeSelector:
          cpu-model.node.kubevirt.io/Common-CPU: "true"
    ```

- Scenario 3: VM with specific feature

    ```yaml
    kind: VirtualMachine
    name: my-vm-2
    spec:
      template:
        spec:
          domain:
          cpu:
            features:
            - name: avx2
              policy: require
    ```

  The Pod spec will be like this:

    ```yaml
    kind: Pod
    metadata:
    name: my-vm-2-pod
    spec:
        nodeSelector:
          cpu-feature.node.kubevirt.io/avx2: "true"
    ```

- Scenario 4: VM with specific feature and cluster-wide cpu model

    ```yaml
    kind: VirtualMachine
    name: my-vm-2
    spec:
      template:
        spec:
          domain:
          cpu:
            features:
            - name: avx2
              policy: require
    ```

    The KubeVirt config is:

    ```yaml
    kind: KubeVirt
    metadata:
    name: kubevirt
    namespace: kubevirt 
    spec:
      configuration:
        cpuModel: "Common-CPU"
    ```

    The Pod spec will be like this:

    ```yaml
    kind: Pod
    metadata:
    name: my-vm-2-pod
    spec:
      nodeSelector:
        cpu-feature.node.kubevirt.io/avx2: "true"
        cpu-model.node.kubevirt.io/Common-CPU: "true"
    ```

- Scenario 5: VM with specific model, feature and cluster-wide cpu model

    ```yaml
    kind: VirtualMachine
    name: my-vm-2
    spec:
      template:
        spec:
          domain:
          cpu:
            model: Intel-A
            features:
            - name: avx2
              policy: require
    ```

  The KubeVirt config is:

    ```yaml
    kind: KubeVirt
    metadata:
    name: kubevirt
    namespace: kubevirt 
    spec:
      configuration:
        cpuModel: "Common-CPU"
    ```

  The Pod spec will be like this:

    ```yaml
    kind: Pod
    metadata:
    name: my-vm-2-pod
    spec:
      nodeSelector:
        cpu-feature.node.kubevirt.io/avx2: "true"
        cpu-model.node.kubevirt.io/Intel-A: "true" # It's overridden by the VM spec
    ```