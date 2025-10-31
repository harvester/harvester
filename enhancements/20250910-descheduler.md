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

N/A

## Note [optional]

N/A
