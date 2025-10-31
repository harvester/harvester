# Title

Support selection of a cluster network with vlan-id to act as underlay for the kubeovn overlay networks used by VMs.

## Summary

Currently, Harvester configures the management interface with an IP address on only a single VLAN during installation.This means that when using KubeOVN overlay networks for VM traffic across nodes, only the management interface on a single vlan is used as the underlay network, which may lead to overlapping traffic and lack of proper isolation. This feature request proposes enabling any vlan interface on the Harvester host to act as a Layer‑3 underlay: 

 HEP https://github.com/rrajendran17/harvester-harvester/blob/HEP-ipconfig/enhancements/20251015-support-ipconfig-on-clusternetworks.md details the design and implementation for creating and assigning IP address to vlan interfaces created on a cluster network.

### Related Issues

https://github.com/harvester/harvester/issues/7834

## Motivation

- Separating VM inter‑node traffic from the management network reduces contention, improves security and performance.

- Users often want to choose the physical uplink and VLAN for VM traffic; this feature gives them the means to configure the underlay network explicitly rather than implicitly using the mgmt interface.

- In many deployments the management interface must be isolated from data or VM traffic; enabling a dedicated underlay ensures network best‑practices are followed.

- Using the mgmt cluster network on a different vlan for VM overlay traffic ensures traffic isolation between vlans over the same physical link.

### Goals

- Allow users to select a cluster network with vlan-id (`tunnel interface`) to be used as underlay for overlay networks used by VMs.

- Only one tunnel interface or underlay must exist for all the overlay networks in the cluster.

- Only the onfigured tunnel interface should be available when choosing the cluster network during creation of `VM Networks` with type `Overlay Network` from UI.

### Non-goals

-  Verifying the IP connectivity between nodes before using the vlan interface as underlay.
-  Support for this feature on user defined ovs bridges.

## Proposal

- Introduce a new CRD in network controller which handles the tunnel interface configuration by user.
- Users select a cluser network and a vlan id to be used as tunnel interface.
- Network controller handles the change, and update the annotations `ovn.kubernetes.io/tunnel_interface` on each node in the cluster.
- kubeovn takes care of updating the remote tunnel endpoints for vxlan in ovs bridges on each node to act as underlay.

### User Stories

- Harvester cluster with 3 nodes with `mgmt-br.2021` having the following IPs.
   - Node 1: `10.115.252.135/23`
   - Node 2: `10.115.252.136/23`
   - Node 3: `10.115.252.137/23`

- Default cluster network `mgmt` spanning all 3 nodes.

- Create cluster network `cluster-1` with networkconfig spanning all 3 nodes

- cluster-1-br.2012 exists on all 3 nodes with the following IPs.
   - Node 1: `10.115.8.15/21`
   - Node 2: `10.115.8.16/21`
   - Node 3: `10.115.8.17/21`

- Harvester cluster with 3 nodes with `mgmt-br.2014` having the following IPs.
   - Node 1: `10.115.24.11/21`
   - Node 2: `10.115.24.12/21`
   - Node 3: `10.115.24.13/21`


#### Story 1 Create a new resource type `UnderlayTunnelIntfConfig` with name `tunnel-1` with cluster-network `cluster-1` and vlan-id `2012`

- updates the node annoation `ovn.kubernetes.io/tunnel_interface` with `cluster-1-br.2012'
- Node 1 default ovs bridge is updated with the remote ips of `10.115.8.16/21` and `10.115.8.17/21`
- Node 2 default ovs bridge is updated with the remote ips of `10.115.8.15/21` and `10.115.8.17/21`
- Node 3 default ovs bridge is updated with the remote ips of `10.115.8.15/21` and `10.115.8.16/21`

#### Story 2 Update resource type `UnderlayTunnelIntfConfig` `tunnel-1` with cluster-network `mgmt` and vlan-id `2014`

- updates the node annoation `ovn.kubernetes.io/tunnel_interface` with `mgmt-br.2014'
- Node 1 default ovs bridge is updated with the remote ips of `10.115.24.12/21` and `10.115.24.13/21`
- Node 2 default ovs bridge is updated with the remote ips of `10.115.24.11/21` and `10.115.24.13/21`
- Node 3 default ovs bridge is updated with the remote ips of `10.115.24.11/21` and `10.115.24.12/21`

#### Story 3 Create resource type `UnderlayTunnelIntfConfig` with name `tunnel-2` with cluster-network `cluster-1` and vlan-id `2012`

- webhook rejects the configuration

#### Story 4 Create a VM Network resource with type `Overlay Network` with cluster-network `mgmt` and create VMs using this VM Network on different nodes.

- VM network resource created successfully and in ready state.
- VM created successfully and in running state.
- Traffic between the VMs must use the underlay over `mgmt-br.2014`

#### Story 5 Update `UnderlayTunnelIntfConfig` having name `tunnel-1` with cluster-network `mgmt` and vlan-id `2012`

- webhook rejects the configuration as there are running VMs using overlay networks.

### User Experience In Detail

users will be able to select a cluster network with vlan-id (`tunnel interface`) using a new resource type `UnderlayTunnelIntfConfig` to be used as underlay for overlay networks used by VMs.

### API changes

## Design

### Implementation Overview

- Introduce a new resource as following,

```
apiVersion: network.harvesterhci.io/v1beta1
kind: UnderlayTunnelIntfConfig
metadata:
  annotations:
    network.harvesterhci.io/clusternetwork: cluster-1
    network.harvesterhci.io/vlan-id: '2014'
  name: tunnelconfig-1
```

- UnderlayTunnelIntfConfig resource agent running on each node does the following,
  - updates the `ovn.kubernetes.io/tunnel_interface` annotation on the node.

- kubeovn takes care of updating the remote tunnel endpoints for vxlan in ovs bridges on each node to act as underlay.

- Kubeovn operator handles the creation of `UnderlayTunnelIntfConfig` crd when enabling the kube-ovn add-on and removing the crd when disabling the kube-ovn add-on.

### Test plan

- Creating a `UnderlayTunnelIntfConfig` with `cluster-network` and `vlan-id` should update the `ovn.kubernetes.io/tunnel_interface` annotations on each node in the cluster.
- Validate if underlay used is same as configured by user using `UnderlayTunnelIntfConfig` by using `ovs-vsctl show` by exec into ovs pods running on each node.
- Webhook must reject the `UnderlayTunnelIntfConfig` with `cluster network` not spanning all nodes in the cluster.
- Webhook must reject the `UnderlayTunnelIntfConfig` for which `cluster network` or `VM Network` not created or not in `ready state`.
- Webhook must reject the `UnderlayTunnelIntfConfig` if the vlan interface for `cluster network` and `vlan-id` not created or not assigned with any IP address on Harvester host.
- Webhook must reject the `UnderlayTunnelIntfConfig` for another tunnel interface.
- Webhook must reject VM Network Configuration with type `Overlay Network` on `cluster network` which is not configured in `UnderlayTunnelIntfConfig`.
- Webhook must reject updating `UnderlayTunnelIntfConfig` if VMs or VMIs present for overlay networks.

### Upgrade strategy

None

## Note [optional]

Additional notes.
