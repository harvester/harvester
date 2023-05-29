# Adjust the scale of ResourceQuota during migration

## Summary

VM migration is impossible in the current implementation when the resource quota defined by a `ResourceQuota` object has been exhausted. The migration requires creating a pod with equivalent resources to the VM being migrated, and if the resources needed for pod creation exceed the quota, the migration cannot proceed. 

Kubevirt is responsible for the migration logic and is not a native feature of Kubernetes. However, it is essential to address the interaction between `ResourceQuota` and migration within Harvester itself. Since Kubevirt does not provide a built-in solution for handling this interaction, it becomes necessary to address this aspect appropriately on the Harvester side.

### Related Issues

[https://github.com/harvester/harvester/issues/3124](https://github.com/harvester/harvester/issues/3124)

## Motivation

### Goals

- Enable VM migration even when the resource quota has been exhausted.

### Non-Goals

- Don't form a calculation of resource usage for hybrid pods deployed within the same namespace that encompasses both VM and non-VM pods.

## Proposal

Increase the resource limit of `ResourceQuota` based on the specifications of the target VM before migration, and restore (decrease) the resource limit to its previous value after the migration is complete.

### User Stories

**VM Migration**

Users need to migrate VMs in Harvester, but the migration fails due to exhausted resources. When VM migration is **Pending**, Harvester should automatically scale up `ResourceQuota` to meet the resource requirements for VM migration. After the VM migration completes, Harvester should automatically scale down the quota.

When creating, starting, or restoring VMs, Harvester should verify whether the resources are sufficient. If resources are insufficient, the system should prompt the user that the operation cannot be completed.

**Upgrade**

When performing a Harvester node **Upgrade** operation, Harvester must ensure that all VMs within the node successfully migrate to other nodes.

**Maintenance**

For a Harvester node performing maintenance, if the user checks the **Force** parameter, Harvester should adequately shut down single-replica VMs and migrate multiple-replica VMs to other nodes. Since single-replica VMs have been shut down, they will not be counted toward resource scaling.

**Change ResourceQuota**

Users may attempt to modify the `ResourceQuota` during a VM migration. In such cases, Harvester should reject the request and inform the user that the `ResourceQuota` cannot be modified during the VM migration.

Rancher's `ResourceQuota` is managed through the Rancher Namespace Controller. If an administrator attempts to modify the `ResourceQuota` from the UI, the `ResourceQuota` Annotation of the Namespace compute resources (CR) is changed. The current design does not allow making changes during a VM migration to avoid conflicts with upstream logic. Additionally, `ResourceQuota` is not a resource that is frequently modified. For more information, refer to the ([Rancher ResourceQuota](https://ranchermanager.docs.rancher.com/how-to-guides/advanced-user-guides/manage-projects/manage-project-resource-quotas/about-project-resource-quotas)) documentation.

**VM Overhead**

Harvester must add Overhead resources to each pod instance created for a VM. During VM migration, the target pod must also include Memory Overhead resources. For more information, refer to the ([Memory Overhead](https://kubevirt.io/user-guide/virtual_machines/virtual_hardware/#memory-overhead)) documentation.

**Hybrid Pods**

For hybrid pods scenarios, Harvester only calculates the used resources based on the `status.used` field in the `ResourceQuota`, which includes the used resources of all pods in the namespace.

### User Experience In Detail

This feature will execute after the `ResourceQuota` resource limits have been configured.

Administrators or tenants do not need to configure this, as all actions are automated. Users just need to pay attention to the prompt messages during the operation or check the event records of the VM.

### API changes

Add a new annotation to `ResourceQuota`：`harvesterhci.io/migrating-{vm-name}`.

`harvesterhci.io/migrating-{vm-name}` represents the VM instances that are being migrated. When the migration is in **Pending**, the Controller will retrieve the resource specifications of the target VM pod, temporarily increase it to the `spec.hard.limits` field of the ResourceQuota, and record it in `harvesterhci.io/migrating-{vm-name}`.  After the migration is complete, the Controller will reduce the resource limits of the `ResourceQuota` according to the resource specifications of that VM and delete the record from `harvesterhci.io/migrating-{vm-name}`.

`harvesterhci.io/migrating-{vm-name}` is a type of `ResourceList`, the value is the Kubernetes core resource list type. For details, refer to ['ResourceList` struct](https://github.com/kubernetes/api/blob/8360d82aecbc72aa039281a394ebed2eaf0c0ccc/core/v1/types.go#L5548-L5549).

## Design

### Implementation Overview

**VM Migration**

The Migration Controller watches the status of the migration process. When the migration status is **Pending**, it automatically scales up the `ResourceQuota` to meet the resource requirements of the VM migration. After the migration is complete, it automatically scales down the quota.

In addition, validators are also added to the VM and Restore CRs to ensure sufficient resources to create, start, or restore a VM.

When VM migration is created, the Migration Controller adds an annotation to the `ResourceQuota` in its namespace, as follows:

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  annotations:
    harvesterhci.io/migrating-vm-01: '{"limits.cpu":"1","limits.memory":"1267028Ki"}'
  labels:
    cattle.io/creator: norman
    resourcequota.management.cattle.io/default-resource-quota: "true"
  name: default-x7fpr
  namespace: rs
spec:
  hard:
    limits.cpu: "4"
    limits.memory: 4952Mi
status:
  hard:
    limits.cpu: "4"
    limits.memory: 4952Mi
  used:
    limits.cpu: "4"
    limits.memory: 5068112Ki
```

The key is `vm_name` with a value of the corresponding VM's pod resources limits. When the VM migration is **Pending**, Harvester should automatically scale up the `ResourceQuota` to meet the resource requirements for the VM migration. Once the VM migration is complete, Harvester should automatically scale down and delete the key.

Because the webhook is not reliable for exceptional cases like high concurrency or timing problems, the VM controller will perform a secondary verification for VMs. If the resources are insufficient, the VM's RunStrategy will be automatically changed to **Halted**, and an event will be recorded to reflect its lack of resources.

**Upgrade**

The implementation is similar to the above. If many VM migrations occur in a node, causing the `ResourceQuota` to scale up and the VM Migration is **Pending**, the VMs that already started may race for resources. To avoid this situation, calculate the actual available resources for the VMs using this formula: **actual limit - (used - migrations)**. Compare this result with the resource quota to see if there are enough resources for the VM to start.

**Maintenance**

Similar to an upgrade, if there are single replica VMs in the node, the system will shut them down, and they will not be counted toward used resources, so no processing is needed.

**Changing ResourceQuota**

When a user changes `ResourceQuota`, the system checks in the annotations whether any VMs are migrating. If there are, the user is notified that they cannot change the `ResourceQuota` while the VM is being migrated.

**Overhead**

Before creating a pod instance for each VM, the upstream first calculates the Overhead resources of the VM and adds them to the VM pod before deployment. During VM migration, the target pod already contains the Overhead resources, and that pod's resource specifications are obtained during migration. In addition, the Overhead resources are calculated before the VM starts and then overlayed with the VM resource specifications to verify that it can be started. For details, refer to [GetMemoryOverhead](https://github.com/kubevirt/kubevirt/blob/2bb88c3d35d33177ea16c0f1e9fffdef1fd350c6/pkg/virt-controller/services/template.go#L1804-L1893).

**Hybrid Pods**

For the hybrid pod scenario, only the used resources under `status.used` in `ResourceQuota` are counted to include the used resources of all pods in the namespace.

**Note:** For the same namespace, if a non-VM pod is **Pending**, the VM migration is also in **Pending**. The non-VM Pod may race for resources and cause the migration to fail. We do not recommend deploying VMs and non-VM Pods in the same namespace.

### Test plan

Aim to verify the functionality of resource quota adjustments during migration. This involves migrating VMs in Harvester and adjusting resource quotas before and after migration. We will ensure the migration operation can complete, even when the resource quota is exhausted.

**Environment**

All operations are based on the multi-tenancy mode.([documentation](https://docs.harvesterhci.io/v1.1/rancher/virtualization-management)).

First, create a project and configure resource quotas by adding CPU and Memory limits:
- CPU: Project limit and namespace default limit set to 4000ms.
- Memory: Project limit and namespace default limit set to 4952MiB.
- The Memory configuration is based on the VM Pod's memory limit * 4.
  All resource operations are performed within the same namespace.

**Test VM migration capability, including normal and resource-limited migration scenarios.**

- Normal migration test:
1. Configure ResourceQuota with CPU limit of 4000ms and memory limit of 4952MiB.
2. Create a VM (1c/1Gi, all tests are based on this configuration) and deploy it on the cluster.
3. Ensure that the target node has enough resources to support migration.
4. Use Harvester's VM migration feature to migrate the VM to the target node.
5. After the migration is complete, check if the VM is running on the target node.

- Resource-limited migration test:
1. Configure ResourceQuota with CPU limit of 1000ms and memory limit of 1238MiB.
2. Create a VM and deploy it on the cluster.
3. Ensure that the target node has enough resources to support migration.
4. Use Harvester's VM migration feature to migrate the VM to the target node.
5. After the migration is complete, check if the VM is running on the target node.

**Test the impact of upgrade operations on VM migration, and ensure that all multi-replicas VMs have been migrated to other nodes.**

1. Configure ResourceQuota with CPU limit of 2000ms and memory limit of 2476MiB.
2. Ensure that you have at least three nodes running Harvester, and have created two VMs and specified that they run on the same node.
3. Confirm that all VMs are running on the same node.
4. Perform the Harvester upgrade operation and check whether the VMs have been migrated to another node during the upgrade.
5. After the Harvester upgrade is complete, check if the VMs are running normally.

**Test the impact of maintenance operations on VM migration. Ensure that single-replica VMs have stopped and multi-replica VMs have been migrated to other nodes when resources are limited.**

1. Configure ResourceQuota with CPU limit of 3000ms and memory limit of 3714MiB.
2. Ensure that you have at least three nodes running Harvester, and have created three VMs with single replica of image and specified that they run on the same node.
> Note: See the [single-replica `StorageClass`](https://docs.harvesterhci.io/v1.1/advanced/storageclass/#parameters-tab) creation documentation for details.
> Use `StorageClass` node selector to ensure that the VMs are running on the same node, see the [documentation](https://docs.harvesterhci.io/v1.1/host/#storage-tags) for details.
3. Then perform maintenance on the node running the VMs.
4. Wait a few minutes until the maintenance operation is completed and confirm that the multi-replica ****VMs on that node have been migrated to other nodes.
5. Confirm that the single-replica VMs have stopped running.
6. Confirm that all VMs are running normally on other nodes.
7. End the maintenance operation and restore the node to an active state.
8. Confirm that the node has rejoined the cluster and no VMs are running on it.

**Test the ability to change ResourceQuota during VM migration to ensure that users cannot change ResourceQuota during VM migration.**

1. Configure ResourceQuota with CPU limit of 2000ms and memory limit of 2476MiB.
2. Create a running VM on the cluster and assign ResourceQuota to it.
3. Start migrating the VM to another node.
4. During VM migration, attempt to change the ResourceQuota on the cluster. Try to increase, decrease, or delete some of the restrictions.
5. Check if Harvester allows the change of ResourceQuota. If the change is allowed, it indicates that Harvester has not successfully prevented changing ResourceQuota during VM migration.


### Upgrade strategy

- No effect on new cluster installations.
- After upgrading an existing cluster.
  - No effect if no configured ResourceQuota.
  - If the ResourceQuota quota is less than the used quota(Used is the current observed total usage of the resource in the namespace, see the [documentation](https://kubernetes.io/docs/concepts/policy/resource-quotas/).), the migration will also fail, and the ResourceQuota will need to be adjusted.

## Note [optional]

N/A