# Support witness node

## Summary

The witness node is a lightweight node that only runs the etcd components. The witness node is not schedulable and does not run any workload. The witness node forms a quorum with the other two nodes.

### Related Issues

https://github.com/harvester/harvester/issues/3266

## Motivation

Some users do not have three powerful nodes for the edge uses case. Instead, they would have two powerful nodes and one lightweight node. The lightweight node is only used for the `etcd` role. Kubernetes need at least three `etcd` nodes to form a quorum. The witness node support will help the edge uses case. That will benefit both cost and high availability.

Reference:
  - https://etcd.io/docs/v3.5/faq/#why-an-odd-number-of-cluster-members
  - https://etcd.io/docs/v3.5/faq/#what-is-failure-tolerance

### Goals

Create a three-node cluster with one witness node.

## Proposal

### User Stories

For the edge use case, the witness node support benefits both cost and high availability. The classical scenario would be two nodes and one witness node.

### User Experience In Detail

#### Installation
- Install with interactive mode:
   1. There is no change for the first node (`create` mode). The first node must be the master node.
   2. For the second/third node, we will select the `join` mode to join the current cluster. Then, we will move to the role selection panel. On this panel, we can choose the role of `Default` or `Witness`. If we select the `Witness` role, the node will be promoted to only the etcd role, which will be the witness node.

- Install with non-interactive mode:
   1. Create with different `install.role` determining the node role (master/witness). Note: the first node will be the master node regardless of the value of `install.role`.

#### Running workloads
After we create the cluster with the witness node, we can run the workloads as usual. The only difference is that the witness node does not run any workload. Only the etcd related workload and some necessary workload (like harvester-node-manager) will be scheduled for the witness node.

#### Specify the node role (Optional)
The users can specify the node role when the cluster is running. The minimal requirement is to keep the three etcd roles to form a quorum, so we can try to promote the node to etcd or master role. And we can also delete the node if we can satisfy the minimal requirement.

#### Upgrade
The upgrade should work as usual.

### API changes

None

## Design

We need to document the following limitations and requirements for the witness node feature.

### Limitations


- The replica number of the default StorageClass is `3`, but in the two master nodes + one witness node scenario, the replica number should be `2`. Users could manually create a new StorageClass and set it as default or use the installation configuration to change the replica number of the default StorageClass [link](https://docs.harvesterhci.io/v1.2/install/harvester-configuration#installharvesterstorage_classreplica_count).

- Only allow one witness node in the cluster. If we have more than one witness node, promote controller will not count the other witness nodes into the management node number.

#### Requirements

Only one witness node is allowed in the cluster.

### Implementation Overview

#### Promote controller

Harvester depends on the job `promote` to promote a node to related role after it joins (the default role is worker). We can only promote the node to `etcd` role with the label `node-role.harvesterhci.io/witness=true`. Also, we need to add taint for etcd role to prevent workload from scheduling to the node.

After this feature, the harvester node will have the following roles:
1. master node roles:
   - node-role.kubernetes.io/control-plane: "true"
   - node-role.kubernetes.io/etcd: "true"
   - node-role.kubernetes.io/master: "true"

2. etcd node roles:
   - node-role.kubernetes.io/etcd: "true"

3. worker node roles:

#### Taint and Toleration

Promote controller will taint the witness node with the following taint:
```
taints:
- key: key: node-role.kubernetes.io/etcd
  effect: NoExecute
  value: "true"
```

Add the following toleration to the promote job and node manager:
```
tolerations:
- effect: NoExecute
  operator: Exists
```

**OPTIONAL**: Promote controller can change the node role dynamically.

#### Harvester WebHook

Add some checking mechanism to avoid adding the control-plane label, `node-role.kubernetes.io/control-plane: "true"`, to the witness node by mistake.

#### Installation

1. Interactive mode:
   - We will move to the role selection panel when we select the join mode. We can choose the role of `Default` or `Witness`. If we select the `Witness` role, the node will be promoted to only the etcd role, which will be the witness node.

2. Non-interactive mode:
   - Use the `install.role=witness` to set the witness node.
     - supported `default`, `witness`

   Example:
   ```yaml
   scheme_version: 1
   server_url: <server_url>
   token: <join token>
   os:
     hostname: <hostname>
     password: <password>
     ntp_servers: <ntp servers>
   install:
     mode: join
     role: witness <-- this would make the node as etcd role (witness node)
     management_interface:
     interfaces:
       - name: <management interface name>
     method: dhcp
     device: <target disk>
     iso_url: <iso url>
     tty: ttyS0
   ```

#### Miscellaneous changes

- **Longhorn**: 
    
   The default StorageClass `harvester-longhorn` will have three replicas, but it could not be satisfied with the witness node. We need to update the default StorageClass to two replicas.

- **Harvester-Node-Manager**:

   The node manager needs to run on the whole cluster because it handles the NTP synced and other node-related jobs. We need to add some toleration to the node manager to ensure it can be scheduled to the witness node.

- **sync-additional-ca**:

  The `sync-additional-ca` job will sync the additional CA to the node. We need to add some toleration to the job to ensure it can be scheduled to the witness node.

- **kubelet**:

  Because the witness node will taint with the `node-role.kubernetes.io/etcd: "true"` when provision, we need to run kubelet with `--register-with-taints=node-role.kubernetes.io/etcd=true:NoExecute` argument to ensure the kubelet can work.

### Test plan

1. Create a three-node cluster through both interactive and non-interactive modes.
2. Check the node roles after the cluster is created. (Two master nodes and one witness node)
3. Verify that all basic functions are working as expected.
4. Also, check the `rancher-vcluster` can be created and worked as well.

### Upgrade strategy

Upgrade should not be affected.

## Note [optional]

Web UI should consider supporting the pormote operation. The user can select the node and specify the role for this node.

There is the resource usage for the witness node. (harvester-node-2 is the witness node)
```
NAME↑                   STATUS       ROLE                             VERSION                     PODS        CPU         MEM       %CPU        %MEM        CPU/A        MEM/A AGE          
harvester-node-0        Ready        control-plane,etcd,master        v1.25.9+rke2r1                71       1244       10081         12          63        10000        15976 24h          
harvester-node-1        Ready        control-plane,etcd,master        v1.25.9+rke2r1                28        628        5794          6          36        10000        15976 24h          
harvester-node-2        Ready        etcd                             v1.25.9+rke2r1                 7        223        3025          2          18        10000        15976 24h          
``` 