# Descheduler

## Summary

The Descheduler is a Kubernetes component that helps maintain optimal pod distribution across nodes by evicting pods that violate certain policies. By integrating the Descheduler into Harvester, we can enhance the cluster's resilience and performance, ensuring that workloads are balanced and resources are utilized efficiently.

### Related Issues

https://github.com/harvester/harvester/issues/2311

## Motivation

### Goals

- Integrate the Descheduler into Harvester to improve pod distribution and resource utilization.
- VM pods can be evicted.
- User can enable or disable the Descheduler as needed.
- User can configure the node utilization threshold for the Descheduler.
- Support DefaultEvictor and LowNodeUtilization strategies.

### Non-goals [optional]

- Support more strategies other than DefaultEvictor and LowNodeUtilization.

## Proposal

### User Stories

- User finds that some nodes are overutilized while others are underutilized, leading to performance bottlenecks.
- By enabling the Descheduler, the system can automatically redistribute pods across nodes, improving overall cluster performance and resource utilization.

### User Experience In Detail

1. Enable the Descheduler via Harvester UI.
2. Configure the node utilization threshold for the Descheduler.
3. Select included and excluded namespaces for the Descheduler.
4. Monitor the Descheduler's activity and the resulting pod distribution across nodes.

### API changes

N/A

## Design

### Implementation Overview

- The Descheduler will be added as an AddOn in Harvester.
- When user enables the Descheduler, the AddOn controller will create a Descheduler deployment in the `kube-system` namespace.

### Test plan

- Create a 2-node Harvester cluster.
- Create several VMs and migrate them to the control-plane node. Don't select the node selector on VMs.
- Enable the Descheduler and set the node utilization threshold to 20%.
- Verify that some VMs are evicted from the control-plane node and scheduled to the worker node.
- Disable the Descheduler and verify that no VMs are evicted.

### Upgrade strategy

During Harvester upgrade, if the Descheduler AddOn is enabled, it may cause unexpected pod evictions. To mitigate this, the AddOn controller will temporarily disable the Descheduler AddOn before the upgrade and enable it after the upgrade is complete.

- After SystemServicesUpgraded condition is true, the upgrade controller adds the current upgrade information to the Descheduler AddOn labels and set "harvesterhci.io/reenable-descheduler-addon" annotation to the upgrade CR. Since we update two objects in one upgrade step, we need to make sure both AddOn and Upgrade CR are updated successfully before disabling the Descheduler AddOn. This avoids the scenario where only one of them is updated successfully, causing the Descheduler to be disabled permanently.
- The AddOn controller watches the Descheduler AddOn labels. When it sees the label with the current upgrade information, it disables the Descheduler deployment.
- After the upgrade is complete, the upgrade controller checks whether there is "harvesterhci.io/reenable-descheduler-addon" annotation in the Upgrade CR. If it exists, it enables the Descheduler AddOn and removes the annotation on the upgrade CR and the label on the Descheduler AddOn.

## Note [optional]

N/A
