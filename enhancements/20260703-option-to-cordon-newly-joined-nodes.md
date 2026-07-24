# Option to Cordon Newly Joined Nodes

## Summary

When deploying a Harvester cluster, each new node added becomes immediately available for use by workloads. In some cases this is not desirable, for example if some post-installation configuration of nodes is required prior to having them run VMs. We can allow this by adding a configuration option to initially bring nodes up in a cordoned state.

### Related Issues

https://github.com/harvester/harvester/issues/4424
https://github.com/harvester/harvester/issues/11074

## Motivation

### Goals

- Allow newly joined nodes to come up in a cordoned state, so that they won't host workloads until the admin manually uncordons the nodes.

### Non-goals

- Allow control plane nodes to be initially cordoned.

## Proposal

### User Stories

#### Story 1

I'm installing Harvester, and I want to initially prevent VMs being deployed on some or all nodes, until a later time.

#### Story 2

I want to add some additional nodes to a Harvester cluster, and I want to prevent VMs being deployed on those new nodes, until a later time.

### User Experience In Detail

In either of the above cases (installing a new cluster, or adding new nodes to an existing cluster), the user needs to provide a Harvester configuration file which specifies `install.cordoned: true`, for example:

```yaml
scheme_version: 1
server_url: "..."
token: TOKEN_VALUE
...
install:
  mode: join
  cordoned: true
  ...
```

When any node is added to the cluster with that config option set, it will be immediately cordoned (the node's `spec.unschedulable` will be set to `true`).

Later, when the user is ready for the node to host workloads, they can use the "Uncordon" option in the Harvester GUI to make the node available for use.

Any nodes that are cordoned will not be considered for promotion to management nodes until they are uncordoned.

### API changes

- Add an `install.cordoned` option to the Harvester config file.

## Design

### Implementation Overview

Adds a new Harvester config option, `install.cordoned`. If this is set to true, it triggers the addition of a label to the node: `harvesterhci.io/install-cordoned=true`.

The promote handler's `OnNodeChanged()` function looks for this label, and if it's present, sets the node to unschedulable, then adds `harvesterhci.io/install-cordoned=true` as an annotation to indicate the cordon was processed, and shouldn't be performed again.

The node will not be cordoned if doing so would leave no other nodes available. This means the first node when creating a cluster will never be automatically cordoned - it only works for nodes that are joining a cluster.

The logic is added to the promote handler because making a whole new controller just for this would be a bit ridiculous, and given the promote handler is responsible for manipulating nodes when they're added and removed, it seemed a sensible place.

The `install.cordoned` option is not exposed in the installer GUI, it must be via remote configuration file or kernel command line argument.

### Test plan

- Install a cluster with 2 or more nodes _without_ setting the `install.cordoned` config option. Verify the nodes come up and aren't cordoned.
- Install a cluster with 2 nodes, both with the default role, but specify `install.cordoned` in the Harvester config file for all nodes.
  - The first node should be running normally (not cordoned) even though the option was set for that node.
  - The second node should be cordoned (check `kubectl get nodes` - if it says `SchedulingDisabled` for a node, then that node is cordoned).
  - Check the node labels. You should see the `harvesterhci.io/install-cordoned=true` label present on each node. Once the node is cordoned, `harvesterhci.io/install-cordoned=true` will also be added as an annotation to indicate the cordon has been done.
- Add a third node to the cluster, again with the default role, and with `install.cordoned` in the config file. The second and third node should remain cordoned and should not automatically be promoted.
- Add a fourth node, again with the default role, and with `install.cordoned` in the config file. It should come up cordoned and stay that way.
- Uncordon any two of the second, third and fourth nodes. Those nodes that you uncordon should now be automatically promoted to management.
- Reboot any of the nodes that initially came up cordoned, but were later uncordoned. They should remain uncordoned after reboot.

### Upgrade strategy

The upgrade process is unaffected.

## Note

The original feature request is described as "install in maintenance mode", and talks of taking nodes out of maintenance mode when they're ready to be used. Maintenance mode is a special thing though, for systems that are already running, which evicts running VMs from a node and then eventually cordons it. In the case this HEP addresses (joining new nodes), we have no need for that eviction process, so all we do is cordon them. So the nodes will just appear in the UI as Cordoned, not as in Maintenance.
