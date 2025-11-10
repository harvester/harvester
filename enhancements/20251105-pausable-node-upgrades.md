# Pausable Node Upgrades

## Summary

This enhancement allows users to manage upgrades during the node upgrade phase of the Harvester Upgrade process. Specifically, in Harvester Upgrade V1, when a cluster node is transitioning to the pre-draining state, the pre-drain job can only be created after the user actively approves it. Within the time period, the user can perform various operations, such as manually live-migrating the virtual machines from the to-be-pre-drained node to other suitable nodes, or simply turning off the virtual machines at the last minute (they no longer need to be shut off at the beginning of an upgrade).

### Related Issues

https://github.com/harvester/harvester/issues/8980

## Motivation

There are different types of user workloads. Some of these workloads are not eligible for live migration. In such cases, they will inevitably be shut down during the Harvester Upgrade process, particularly during the node upgrade phase. Such behavior/requirement is less acceptable to users, as they cannot know the order of node upgrades in advance and don't have enough time to shut down the virtual machines that will be affected gracefully. Moreover, many users prefer to handle an upgrade event manually rather than let it execute autonomously. They would like to nudge the workload migration based on node-specific criteria. For instance, moving workloads to a more capable node. All of this stems from the inability to manually intervene in node upgrade operations and the overly simplistic virtual machine eviction mechanism.

Note: In recent Harvester versions, virtual machines that cannot be live-migrated must be turned off before an upgrade can start, resulting in significant downtime for user workloads and services.

### Goals

- Enable per-upgrade opt-in for pausable node upgrades
- Automatically pause node upgrade before pre-drain job creation
- Allow per-node manual approval to proceed with pre-drain

### Non-goals [optional]

- Provide the ability to pause node upgrades at any time
- Support pausing during other upgrade phases
- Provide an interface specifying the node upgrade order explicitly or skipping the upgrade for any node
- Modify the pre-drain/drain/upgrade logic itself
- Implement automated user workload placement decisions

## Proposal

### User Stories

#### Story 1: Manual Virtual Machine Migration During Pause

As a cluster administrator, I want to manually migrate virtual machines while an upgrade for a node is paused so that I can move workloads to more capable nodes based on my knowledge and preference.

#### Story 2: Last-Minute VM Shutdown

As a cluster administrator with virtual machines requiring graceful shutdown, I want to shut down virtual machines right before node pre-drain so that I minimize downtime (virtual machines don't need to be off during entire upgrade).

### User Experience In Detail

The user can set the node upgrade **mode** to be `auto` or `manual`. The default value is `auto`, which suggests the node upgrade behavior we're all familiar with: nodes are upgraded one by one autonomously. If the mode is set to `manual`, the user can further set the pausability of upgrades for specific nodes in the **pauseNodes** field. Nodes not in the **pauseNodes** field will still undergo node upgrades automatically; if no nodes are specified in **pauseNodes**, all nodes will be paused before entering the pre-draining state, provided the mode is `manual`. When the **mode** is set to `auto` or not being set at all, the values in the **pauseNodes** are completely ignored.

For instance, the following setting will pause upgrades for nodes `node-2`, `node-5`, and `node-9`. Other nodes in the cluster will still be upgraded autonomously.

```json
{
  "nodeUpgradeOption": {
    "strategy": {
      "mode": "manual",
      "pauseNodes": [
        "node-2",
        "node-5",
        "node-9"
      ]
    }
  }
}
```

Another example, the following setting will pause all the nodes in the cluster before they enters the pre-draining state.

```json
{
  "nodeUpgradeOption": {
    "strategy": {
      "mode": "manual"
    }
  }
}
```

When the user triggers an upgrade for the cluster by clicking the "Upgrade" button on the dashboard UI or manually creating the Upgrade custom resource, the relevant setting will be automatically applied. Note that it will not take effect if the user configures the setting after the upgrade has started. During the node upgrade phase, the user can observe each node's upgrade progress in the Upgrade custom resource and in the upgrade dialog.

After the user completes the tasks and is ready to continue with the node upgrade, they can remove the annotation from the node or click the "Resume" button on the upgrade dialog to proceed.

### API changes

There are no actual API changes involved.

## Design

### Implementation Overview

The implementation includes two major parts: a set of new setting fields and the node annotation manipulation. Users configure the `nodeUpgradeOption` fields in the `upgrade-config` setting to specify their node upgrade preferences. The upgrade controller then annotates the nodes marked for pause according to the setting when the upgrade is initiated.

During the node upgrade phase, the secret controller reconciles the machine-plan secrets. When the time comes (when the machine-plan secret for a node is annotated as eligible to upgrade), it checks whether the node has the annotation before creating the pre-drain job. When the annotation is removed from the node, the secret controller is triggered again and reconciles the machine-plan secrets to create the corresponding pre-drain job, thus continuing the original node upgrade flow.

In theory, a successful upgrade should have no node-upgrade-pause annotations left on the node objects (users should have removed them because they consented to proceed with the node upgrade). For cases such as failed or aborted upgrades (where the Upgrade custom resource is removed in the middle of an upgrade), the annotation must be cleaned up automatically by the upgrade controller.

### Test plan

Due to the Harvester Upgrade implementation, single-node cluster upgrades are executed differently from other cluster configurations; it's required to test at least both.

The following assumes the master-head version already includes the proposed enhancement.

#### Single-node cluster upgrade

1. Prepare a single-node cluster with the master-head version
1. Configure the `upgrade-config` setting:
   ```json
   {
     "nodeUpgradeOption": {
       "mode": "manual"
     }
   }
   ```
1. Trigger an upgrade using the same master-head version ISO image
1. The upgrade should be paused when entering the node upgrade phase
1. Consent the node upgrade to proceed by removing the node-upgrade-pause annotation on the node
1. The entire upgrade should finish successfully

#### Multi-node cluster upgrade

1. Prepare a three-node cluster with the master-head version
1. Configure the `upgrade-config` setting:
   ```json
   {
     "nodeUpgradeOption": {
       "mode": "manual",
       "pauseNodes": [
         "node-2",
         "node-3"
       ]
     }
   }
   ```
1. Trigger an upgrade using the same master-head version ISO image
1. The upgrade should be paused for `node-2` and `node-3` (the order is not guaranteed) before they enter the pre-draining state
1. Consent the node upgrades to proceed by removing the node-upgrade-pause annotation on the nodes, respectively
1. The entire upgrade should finish successfully

#### Aborted upgrade

1. Prepare a three-node cluster with the master-head version
1. Configure the `upgrade-config` setting:
   ```json
   {
     "nodeUpgradeOption": {
       "mode": "manual"
     }
   }
   ```
1. Trigger an upgrade using the same master-head version ISO image
1. Check whether all nodes have the node-upgrade-pause annotation
1. Remove the Upgrade custom resource during the image-preload phase
1. Check that all nodes should have no node-upgrade-pause annotations left

### Upgrade strategy

This enhancement is unavailable when upgrading to the release that introduces it. It is only supported for upgrades from that release onward.

## Note [optional]

The proposed pausable node upgrades enhancement, compared to the "Restore VM" function, provides an alternative way to minimize downtime for user workloads during Harvester Upgrade by giving users finer-grained control over the node upgrade process.
