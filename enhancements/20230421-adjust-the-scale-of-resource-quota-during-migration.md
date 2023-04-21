# Adjust the scale of ResourceQuota during migration

## Summary

When a user has used up all the ResourceQuota quota, it is not possible to perform VM migration. This is because ResourceQuota limits the quota usage for the entire namespace, and migration requires the creation of a Pod equivalent to the VM being migrated. As Kubernetes considers exceeding the quota to result in the inability to create Pods, migration cannot proceed.

It should be noted that migration logic is controlled by Kubevirt, not a native behavior of Kubernetes. Additionally, Kubevirt does not provide a solution between ResourceQuota and migration.

### Related Issues

[https://github.com/harvester/harvester/issues/3124](https://github.com/harvester/harvester/issues/3124)

## Motivation

### Goals

- It is possible to perform VM migration even after ResourceQuota has been exhausted.

### Non-Goals

The calculation of resource usage for Hybrid Pods encompasses both VM and non-VM Pods that are deployed within the same namespace.

## Proposal

Increase the resource limit of ResourceQuota based on the specifications of the target VM before migration, and restore (decrease) the resource limit to its previous value after the migration is complete.

### User Stories

**VM Migration**

Users need to migrate VMs in Harvester, but the migration fails due to exhausted resources. When VM migration is in Pending, Harvester should automatically scale up ResourceQuota to meet the resource requirements for VM migration. After the VM migration is completed, Harvester should automatically scale down the quota.

When creating, starting, or restoring VMs, Harvester should verify whether the resources are sufficient. If resources are insufficient, the system should prompt the user that the operation cannot be completed.

**Upgrade**

When performing a Harvester node Upgrade operation, Harvester needs to ensure that all VMs within the node have been migrated to other nodes.

**Maintenance**

For a Harvester node performing maintenance, if the Force parameter is checked, Harvester should properly shut down single-replica VMs and migrate multiple-replica VMs to other nodes. Since single-replica VMs have already been shut down, they will not be counted towards resource scaling.

**Change ResourceQuota**

When a VM is being migrated, users may attempt to modify the ResourceQuota. In such cases, Harvester should reject the request and inform the user that the ResourceQuota cannot be modified while the VM is being migrated.

Rancher's ResourceQuota is managed through the Rancher Namespace Controller. If an administrator attempts to modify the ResourceQuota from the UI, the ResourceQuota Annotation of the Namespace CR is modified. The current design does not allow changes to be made while a VM is being migrated to avoid conflicts with upstream logic. Additionally, ResourceQuota is not a resource that is frequently modified ([see the Rancher ResourceQuota documentation](https://ranchermanager.docs.rancher.com/how-to-guides/advanced-user-guides/manage-projects/manage-project-resource-quotas/about-project-resource-quotas)).

**VM Overhead**

Harvester needs to add Overhead resources to each Pod instance created for a VM. During VM migration, the target Pod also needs to include Overhead resources([Overhead documentation](https://kubevirt.io/user-guide/virtual_machines/virtual_hardware/#memory-overhead)).

**Hybrid Pods**

For Hybrid Pods scenarios, Harvester only calculates the used resources based on the `status.used` field in the ResourceQuota, which includes the used resources of all Pods in the namespace.

### User Experience In Detail

This feature will only be executed when the ResourceQuota resource limits have been configured.

Administrators or tenants do not need to configure this, as all actions are automated. Users can simply pay attention to the prompt messages during the operation or check the event records of the VM.

### API changes

Add a new annotation to ResourceQuota：`harvesterhci.io/migratingVMs`.

`harvesterhci.io/migratingVMs` represents the VM instances that are being migrated. When the migration is in Pending, the Controller will retrieve the resource specifications of the target VM Pod, temporarily increase it to the `spec.hard.limits` field of the ResourceQuota, and record it in `harvesterhci.io/migratingVMs`. After the migration is completed, the Controller will reduce the resource limits of the ResourceQuota according to the resource specifications of that VM and delete the record from `harvesterhci.io/migratingVMs`.

`[harvesterhci.io/migratingVMs](http://harvesterhci.io/migratingVMs)` it's a type of `map[string]ResourceList`, where the key is the VM name, and the value is the Kubernetes core resource list type.(`[ResourceList` struct](https://github.com/kubernetes/api/blob/8360d82aecbc72aa039281a394ebed2eaf0c0ccc/core/v1/types.go#L5548-L5549)).

## Design

### Implementation Overview

**VM Migration**

The Migration Controller watches the status of the migration process. When the migration status is Pending, it automatically scales up the ResourceQuota to meet the resource requirements of the VM migration. After the migration is complete, it automatically scales down the quota.

In addition, validators are added to the VM and Restore CRs to ensure that there are sufficient resources to create, start, or restore a VM.

When a migration is created, the Migration Controller adds an annotation to the ResourceQuota in its namespace, as follows:

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  annotations:
    harvesterhci.io/migratingVMs: '{"vm-01":{"limits.cpu":"1","limits.memory":"1267028Ki"}}'
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

The key is `vm_name` with a value of the corresponding VM's Pod resources limits. When Migration is in Pending, Harvester should automatically scale up the ResourceQuota to meet the resource requirements for the VM migration. Once the VM migration is complete, Harvester should automatically scale down and delete the key.

Due to the unreliability of the Webhook, the VM Controller will perform a second verification for VMs. If the resources are insufficient, the VM's RunStrategy will be changed to Halted, and an event will be sent indicating the lack of resources.

**Upgrade**

The implementation is similar to the above. If a large number of VM migrations occur in a node, causing the ResourceQuota to scale up and Migration to be in Pending, the VMs that are already started may race for resources. To avoid this situation, the actual available resources for the VMs are calculated using the formula: actual limit - (used - migrations), and the result is compared to see if there are enough resources for the VM to start.

**Maintenance**

Similar to Upgrade, but if there are single replica VMs in the node, the system will shut them down, and they will not be counted toward used resources, so no processing is needed.

**Changing ResourceQuota**

When a user changes ResourceQuota, the system checks whether there are any migrating VMs in the annotations. If there are, the user will be prompted that they cannot change the ResourceQuota while the VM is being migrated.

**Overhead**

Before creating a Pod instance for each VM, the upstream first calculates the Overhead resources of the VM and adds them to the VM Pod before deployment. During VM migration, the target Pod already contains the Overhead resources and the resource specifications of that Pod are obtained during migration. In addition, the Overhead is calculated before the VM is started([calculation](https://github.com/kubevirt/kubevirt/blob/2bb88c3d35d33177ea16c0f1e9fffdef1fd350c6/pkg/virt-controller/services/template.go#L1804-L1893)) And overlay it with the VM resource specifications and verify that it can be started.

**Hybrid Pods**

For the Hybrid Pods scenario, only the used resources under `status.used` in ResourceQuota will be counted, to include the used resources of all Pods in the Namespace.

**For the same Namespace, if a non-VM Pod is in Pending while Migration is also in Pending, the non-VM Pod may race for resources and cause the migration to fail. We do not recommend deploying VMs and non-VM Pods in the same Namespace.**

### Test plan

Aim to verify the functionality of resource quota adjustment during migration. This will involve migrating VMs in Harvester and adjusting resource quotas before and after migration. We will ensure that the migration operation can be completed, even when the resource quota is exhausted.

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
2. Ensure that you have at least three nodes running Harvester, and have created three VMs and specified that they run on the same node. After creating one VM, access Longhorn Volume and change the number of VM replicas to 1, then delete other replicas except for the running node.
   Use the following script to perform these operations:

    ```bash
    #!/bin/bash
    namespace="<namespace>"
    vm_name="<vm_name>"
    # Get VM PVC's spec.volumeName
    pvc_name=$(kubectl -n${namespace} get vm ${vm_name} -ojson | jq -r '.spec.template.spec.volumes[0].persistentVolumeClaim.claimName')
    
    volume_name=$(kubectl -n${namespace} get pvc ${pvc_name} -ojson | jq -r '.spec.volumeName')
    
    # update VM volume replicas to 1
    kubectl -nlonghorn-system patch volumes ${volume_name} --type=json -p '[{"op": "replace", "path": "/spec/numberOfReplicas", "value": 1}]'
    
    # remove replicas of nodes that are not VM running
    node_name=$(kubectl -n${namespace} get vmi ${vm_name} -ojson | jq -r '.status.nodeName')
    kubectl -nlonghorn-system get replicas -l longhornvolume=${volume_name} --no-headers=true \
    	| grep -v ${node_name} \
      | awk '{print $1}' \
      | xargs -I {} kubectl -nlonghorn-system delete replicas {}
    ```

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

Anything that requires if user want to upgrade to this enhancement

## Note [optional]

Additional nodes.