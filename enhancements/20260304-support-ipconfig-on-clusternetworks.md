# Title

This feature supports creation of vlan sub interfaces on mgmt and user defined cluster networks and configuration of IPv4 addresses on these sub interfaces.

## Summary

This feature allows multiple virtual interfaces to be created per vlan on all cluster networks which can be used for IPv4 connectivity from Harvester Nodes to the external environment.

### Related Issues

https://github.com/harvester/harvester/issues/8101

## Motivation

- Some data‑plane or storage networks must be reachable as routed L3 subnets (for example: dedicated storage network with static or dynamic IP addressing)

- Operators may want to isolate management traffic from application traffic using different routed subnets on separate physical NICs (different gateways, different next‑hop routers, or different physical uplinks).

- Cloud/edge environments sometimes require that a non‑management NIC has its own IPv4 address on a different Layer3 network to integrate with existing routing/OSPF/BGP or external services.

Harvester currently configures a single VLAN sub-interface on the management NIC by default during installation. However, users need the ability to configure multiple VLAN sub-interfaces on both the management NIC and other user defined cluster networks with full L3 support.These configurations must be automated and persist across reboots and upgrades. Manual setup today is error-prone and not sustainable for production use.

### Goals

- Support creation of vlan sub interfaces on Harvester hosts when users configures a new resource `HostNetworkConfig`.
- Support assignment of IPv4 addresses to the vlan sub interfaces on Harvester hosts by static method or dynamically using DHCP.

### Non-goals

- Supporting IPv4 Connectivity on untagged vlan or vlan ID 0.
- Supporting IPPools or external IPAM integration for IP allocation to the vlan sub interfaces on Harvester hosts.
- Supporting IPv6 addresses on the vlan sub interfaces of Harvester Hosts.

## Proposal

- Introduce a new CRD `HostNetworkConfig` for managing the Layer3 Network config on Harvester hosts.
- Allow users to create/update/delete `HostNetworkConfig` resource from UI and CLI.
- Create a new network controller hostnetworkconfig agent for handling `HostNetworkConfig`.
- Hostnetworkconfig controller agent runs on each node and handles the add/update/del of the vlan to the uplinks.
- Hostnetworkconfig controller agent also creates and updates the ip address on the vlan sub interface using netlink commands.

### User Stories

The user stories are independent and should not be considered in any order.
users are required to provide correct vlan and networkconfig considering their setup.

Harvester cluster with 3 nodes with `mgmt` interface on vlan 2021 having the following IPs.
- Node 1: `10.115.252.135/23`
- Node 2: `10.115.252.136/23`
- Node 3: `10.115.252.137/23`

- Default cluster network `mgmt` spanning all 3 nodes.
- Create cluster network `br-1` with networkconfig spanning all nodes in the cluster.
- Create cluster network `br-2` with networkconfig spanning all node 1 and node 2.

#### Story 1: Create Host Network with cluster network `br-1`, vid `2012`, and mode `DHCP`

- bridge vlan vid `2012` added to `br-1-br` and `br-1-bo` on node 1,node 2 and node 3
- vlan sub interface `br-1-br.2012` is created on node 1,node 2 and node 3 in the cluster and ip address is assigned from DHCP.

#### Story 2: Create Host Network with cluster network `br-2`, vid `2013`, and mode `DHCP`

- bridge vlan vid `2013` added to `br-2-br` and `br-2-bo` on node 1,node 2
- vlan sub interface `br-2-br.2013` is created on node 1,node 2 in the cluster and ip address is assigned from DHCP.

#### Story 3: Update Host Network cluster network `br-1`, vid `2012`, with mode `Static` and ips
     "node 1": "192.168.1.10/24",
     "node 2": "192.168.1.11/24",
     "node 3": "192.168.1.12/24"

Existing ip addresses on br-1.2012 removed on node-1,node-2,node-3 and ip address "192.168.1.10/24" assigned to interface `br-1-br.2012` of node 1,
"192.168.1.11/24" assigned to `br-1-br.2012` of node 2 and "192.168.1.12/24" assigned to interface `br-1-br.2012` of node 3.

#### Story 4: Delete Host Network config with cluster network `br-1`, vid `2012`

- bridge vlan vid `2012` removed from `br-1-br` and `br-1-bo` on node 1,node 2 and node 3
- vlan sub interface `br-1-br.2012` is removed from node 1,node 2 and node 3

#### Story 5: Delete vlanconfig/networkconfig under cluster network `br-1`

- Remove all vlan associated with `br-1-br` and `br-1-bo` on node 1, node 2 and node 3
- Remove all vlan sub interfaces associated with `br-1` from node 1,node 2 and node 3

#### Story 6: Create vlanconfig/networkconfig under cluster network `br-1` again

- bridge vlan vid `2012` added to `br-1-br` and `br-1-bo` on node 1,node 2 and node 3
- vlan sub interface `br-1-br.2012` is created on node 1,node 2 and node 3 in the cluster and ip address is assigned from DHCP.

#### Story 7: New node node 4 added to the cluster

- bridge vlan vid `2012` added to `br-1-br` and `br-1-bo` on node 4
- vlan sub interface `br-1-br.2012` is created on node 4 in the cluster and ip address is assigned from DHCP.

#### Story 8: New node node 4 added to the cluster (if mode `static`)

- User reconfigures Host Network config with cluster network `cluster-1`, vlan-id `2012` with node IP "192.168.1.13/24" for node 4
- bridge vlan vid `2012` added to `br-1-br` on node 4
- vlan sub interface `br-1-br.2012` is created on node 4 in the cluster and ip address "192.168.1.13/24" is assigned to `br-1-br.2012`

#### Story 9: Reboot node 1, node 2 and node 3

After the nodes come up,
- bridge vlan vid `2012` added to `br-1-br` abd on node 1,node 2 and node 3
- vlan sub interface `br-1-br.2012` is created on node 1,node 2 and node 3 in the cluster and ip address is assigned from DHCP or static

#### Story 10: Update node selector in vlanconfig under cluster network `br-1` to node-1

- bridge vlan vid `2012` removed from `br-1-br` and `br-1-bo` on node 2 and node 3 but exists in node-1
- vlan sub interface `br-1-br.2012` is removed on node 2 and node 3 but exists in node-1

#### Story 11: Create Host Network with cluster network `mgmt`, vid `2014` and mode `DHCP`

- bridge vlan vid `2014` added to `mgmt-br` and `mgmt-bo` on node 1,node and node 3
- vlan sub interface `mgmt-br.2014` is created on node 1,node 2,node 3 in the cluster and ip address is assigned from DHCP.

#### Story 12: Create Host Network with cluster network `br-1`, vid `2012`, and mode `DHCP` and node selectors `network-role: l3`
- Lable node 1 "kubectl label node `nodename` `network-role=l3`"
- bridge vlan vid `2012` added to `br-1-br` and `br-1-bo` on node 1
- vlan sub interface `br-1-br.2012` is created on node 1 in the cluster and ip address is assigned from DHCP.

Since labels match only node-1, hostnetworkconfig is applied only on node-1 and not on node-2 and node-3.
If there are no node selector on hostnetworkconfig, then the config applies to all nodes in the cluster network by default.

#### Story 13: Create Host Network with cluster network `br-1`, vid `2012`, and mode `DHCP` and node selectors `network-role: l3`
- node-1, node-2 and node-3 are labeled with `network-role=l3`"
- bridge vlan vid `2012` added to `br-1-br` and `br-1-bo` on node 1,node 2 and node 3
- vlan sub interface `br-1-br.2012` is created on node 1,node 2 and node 3 in the cluster and ip address is assigned from DHCP.
- remove the label on node-3 "kubectl label node `nodename` `network-role-`"
- bridge vlan vid `2012` removed from `br-1-br` and `br-1-bo` on node 3
- vlan sub interface `br-1-br.2012` is removed on node 3

#### Story 14: Create Host Network with cluster network `br-1`, vid `2012`, and mode `DHCP` and node selectors `network-role: l3`
- node-1 and node-2 are labeled with `network-role=l3`"
- bridge vlan vid `2012` added to `br-1-br` and `br-1-bo` on node 1,node 2
- vlan sub interface `br-1-br.2012` is created on node 1,node 2in the cluster and ip address is assigned from DHCP.
- Lable node 3 "kubectl label node `nodename` `network-role=l3`"
- bridge vlan vid `2012` added to `br-1-br` and `br-1-bo` on node 3
- vlan sub interface `br-1-br.2012` is added on node 3

### User Experience In Detail

User will be able to create vlan interfaces on required nodes in the cluster and assign IP address to them using static or DHCP mode and will be able to persist this across reboots.
This could be achieved by introducing a new host network config under `Networks tab` in UI which allows users to specify `clusternetwork`, `vlan-id`, `mode` and `list of nodename to ip mapping` if mode is static.

### API changes

```
type HostNetworkConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec HostNetworkConfigSpec `json:"spec"`
	Status HostNetworkConfigStatus `json:"status,omitempty"`
}

type IPAddr string

type HostNetworkConfigSpec struct {
	Description string `json:"description,omitempty"`

	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`

	ClusterNetwork string `json:"clusterNetwork"`

	Underlay bool `json:"underlay"`

	VlanID uint16 `json:"vlanID"`

	Mode string `json:"mode"`

	HostIPs map[string]IPAddr `json:"ips,omitempty"`
}

type HostNetworkConfigStatus struct {
	Conditions []Condition `json:"conditions,omitempty"`
	
	NodeStatus map[string]HostNetworkConfigNodeStatus `json:"nodeStatus,omitempty"`
}

type HostNetworkConfigNodeStatus struct {
	ClusterNetwork string `json:"clusterNetwork"`

	VlanID uint16 `json:"vlanID"`

	Mode string `json:"mode"`

	Conditions []Condition `json:"conditions,omitempty"`
}
```
- hostnetworkconfig agent controller to handle vlan and IP network config on nodes.
- hostnetworkconfig webhook to handle validations.

## Design

### Implementation Overview

#### Overview

case 1: HostNetworkConfig with mode Static

```
apiVersion: network.harvesterhci.io/v1beta1
kind: HostNetworkConfig
metadata:
  name: hostnetworkconfig1
spec:
  clusterNetwork: br-1
  vlanID: 2012
  mode: static
  ips:
    node1: 192.168.1.10/24
    node2: 192.168.1.11/24
    node3: 192.168.1.12/24
```
case 2: HostNetworkConfig with mode DHCP

```
apiVersion: network.harvesterhci.io/v1beta1
kind: HostNetworkConfig
metadata:
  name: hostnetworkconfig2
spec:
  clusterNetwork: br-1
  vlanID: 2013
  mode: dhcp
```

case 3: HostNetworkconfig with node selectors

```
apiVersion: network.harvesterhci.io/v1beta1
kind: HostNetworkConfig
metadata:
  name: hostnetworkconfig3
spec:
  nodeSelector:
    matchLabels:
      network-role: l3
  clusterNetwork: br-1
  vlanID: 2014
  mode: dhcp

```
Create/Update:
- Harvester network controller `hostnetworkconfig` agent running as daemonset on each of the node handles the above configuration and does the following on the host
  using netlink.
  - `bridge vlan add vid <vlan-id> dev <bridge-link-br> self`.
  - `bridge vlan add vid <vlan-id> dev <bridge-link-bo>`.
  - `ip link add link <bridge-link-br> name <vlansubintf-name> type vlan id <vlan-id>`.
  - `ip link set <vlansubintf-name> up`.
  - `ip address add <ipaddr> dev <vlansubintf-name>` (if the mode is static and ip address is selected from node name in the config).

- If the mode is DHCP,use the existing `nclient4` package from network controller to use `Request` function which completes the 4-way Discover-Offer-Request-Ack handshake.
  - Send nclient4.Request().
  - Parse lease.ACK and configure the ip address on the vlan sub interface.
  - Start a lease manager that performs periodic DHCP renewals and automatically reconfigures sub-interface IP addresses if the lease assigns a new address.
    - Any error during lease renewal process will be recorded to the status of corresponding `HostNetworkConfig` resource.

- Hostnetworkconfig `status per node` will be updated to ready `True` or `False` based on the outcome of the configuration on each node.

Delete:
- `hostnetworkconfig` agent does the following on the host using netlink.
  - `bridge vlan del vid <vlan-id> dev <bridge-link-br> self`
  - `bridge vlan del vid <vlan-id> dev <bridge-link-bo>`
  - `ip link del link <bridge-link-bo> name <vlansubintf-name> type vlan id <vlan-id>`

- Skipped on nodes which does not have `uplink`(bridge link is not created when vlanconfig does not include the node) or node selectors not configured.

- In case of any errors, update the per node `ready` status as `false` for the corresponding `HostNetworkConfig` resource.

Example output of `ip addr show` and `bridge vlan show` after the vlan interface created and IP assigned

```
br.2012@cluster-1-br: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default qlen 1000
link/ether 02:a5:50:16:5d:f9 brd ff:ff:ff:ff:ff:ff
inet 10.115.14.230/21 brd 10.115.15.255 scope global br.2012
valid_lft forever preferred_lft forever

port              vlan-id  
mgmt-br           1 PVID Egress Untagged
                  2021
mgmt-bo           1 PVID Egress Untagged
                  2021
cluster-1-bo      1 PVID Egress Untagged
                  2012
cluster-1-br      1 PVID Egress Untagged
                  2012

```

#### Handling Node Updates
The existing node controller in harvester network controller already watches for any node changes.This can be reused to update matched nodes to `HostNetworkConfig` resource.
If a hostnetworkconfig exists and has node selector configured, any change in node label matching the node selector in the hostnetworkconfig will update the macthed node annotation in the hostnetworkconfig which in turn triggers the hostnetworkconfig reconcilation to add vlan interface on that node.

#### Handling vlanconfig Updates
Any change in vlanconfig's node selector or deletion of vlanconfig  will affect the HostNetworkConfig as the host network config should be added on new matched nodes and removed from nodes.
- Watch for change in vlanconfig and trigger reconcile of hostnetworkconfig.

### Test plan

- Create HostNetworkConfig resource with vlan-id,mode,ips should create a vlan interface and assign IP address on the interface on Harvester hosts.
- Remove vlanconfig on the cluster network should remove all bridge vlan and vlan interfaces associated with the cluster network on Harvester hosts selected by the network config.
- Create deleted vlanconfig again should create the vlan interface and assign IP address on the interface.
- Update vlanconfig to specific node selectors should take care adding/removing bridge vlan and vlan interfaces on required Harvester hosts.
- Remove HostNetworkConfig  should remove bridge vlan from the cluster network bridge and remove the vlan interface on Harvester hosts.
- Adding new node to the cluster (if network config spans all node in cluster and mode is `DHCP`) should add bridge vlan and vlan interfaces on the new Harvester host.
- Update to HostNetworkConfig for mode change should remove the existing ip addresses and configure IP address on them either through DHCP or static.
- Create/Update node selectors on hostnetworkconfig should create config only on the respective nodes matching the node selectors.
- Reboot of nodes must reconfigure all the bridge vlans and create vlan interfaces existed prior to reboot.
- Upgrade to newer version (v1.7.x to v1.8.0) should be successful without any errors/issues.
- HostNetworkconfig CRD in built validation must reject if mode is `static` and node to IP mapping list is not provided.
- HostNetworkconfig CRD in built validation must reject if mode is `static` and node IP address is not in valid CIDR format.
- Webhook must reject if number of nodes to IP mapping does not match the nodes covered by vlanconfig using node selectors.
- Webhook should reject update to vlan id or cluster network on existing hostnetworkconfig.
- Webhook must reject the update/delete to HostNetworkConfig used as underlay tunnel interface if there are VMs using overlay networks on that cluster network.
- Webhook must reject the update/delete on vlanconfig if there are VMs using overlay networks on that cluster network.
- Webhook must reject the update/delete of VM Network/NAD being used as vlan id in the hostnetworkconfig.
- Webhook should reject host network config if `cluster network` is not created or not in `ready` state.
- Webhook should reject Host Network config with vlan-id `0`.
- Webhook must reject invalid values on network config fields.

### Upgrade strategy

- None

## Note [optional]

