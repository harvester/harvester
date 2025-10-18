# Title

This feature supports creation of vlan sub interfaces on mgmt and user defined cluster networks and configuration of IP addresses on these sub interfaces.

## Summary

This feature allows multiple virtual interfaces to be created per vlan on all cluster networks which can be used for L3 connectivity from Harvester Nodes to the external environment.

### Related Issues

https://github.com/harvester/harvester/issues/8101

## Motivation

- Some data‑plane or storage networks must be reachable as routed L3 subnets (for example: dedicated storage network with static or dynamic IP addressing)

- Operators may want to isolate management traffic from application traffic using different routed subnets on separate physical NICs (different gateways, different next‑hop routers, or different physical uplinks).

- Cloud/edge environments sometimes require that a non‑management NIC has its own IP address on a different L3 network to integrate with existing routing/OSPF/BGP or external services.

Harvester currently configures a single VLAN sub-interface on the management NIC by default during installation. However, users need the ability to configure multiple VLAN sub-interfaces on both the management NIC and other user defined cluster networks with full L3 support.These configurations must be automated and persist across reboots and upgrades. Manual setup today is error-prone and not sustainable for production use.

### Goals

- Support creation of vlan sub interfaces on Harvester hosts when users configures a VM Network with L3Connectivy requires as `true`
- Support assignment of IP addresses to the vlan sub interfaces on Harvester hosts by static method or dynamically using DHCP.
- Support IPv6 addresses on the vlan sub interfaces of Harvester Hosts

### Non-goals

- Supporting on VM Network with type `L2UntaggedNetwork`,`OverlayNetwork` and `L2VlanTrunkNetwork`.
- Supporting IPPools or external IPAM integration for IP allocation to the vlan sub interfaces on Harvester hosts.

## Proposal

- Allow users to configure/unconfigure Layer 3 network Information on the VM Network resource.
- Harvester network controller NAD handles the change and updates the annotations of the cluster network resource.
- Harvester network controller cluster network agent handles the bridge vlan and creation/update/deletion of nmconnection profiles on the host.
- systemd service installed during harvester installation monitors nmconnection profile on the host,handles the network config on the vlan sub interface.

### User Stories

Harvester cluster with 3 nodes with `mgmt` interface on vlan 2021 having the following IPs.
- Node 1: `10.115.252.135/23`
- Node 2: `10.115.252.136/23`
- Node 3: `10.115.252.137/23`

- Default cluster network `mgmt` spanning all 3 nodes.
- Create cluster network `cluster-1` with networkconfig spanning all 3 nodes
- Create cluster network `cluster-2` with networkconfig spanning all node 1 and node 2.

#### Story 1: Create VM Network with cluster network `cluster-1`, vid `2012`,`L3 Connectivity` field set and mode `DHCP`

- bridge vlan vid `2012` added to `cluster-1-br` on node 1,node 2 and node 3
- vlan sub interface `cluster-1-br.2012` is created on node 1,node 2 and node 3 in the cluster and ip address is assigned from DHCP.

#### Story 2: Create VM Network with cluster network `cluster-2`, vid `2013`,`L3 Connectivity` field set and mode `DHCP`

- bridge vlan vid `2013` added to `cluster-1-br` on node 1,node 2
- vlan sub interface `cluster-1-br.2013` is created on node 1,node 2 in the cluster and ip address is assigned from DHCP.

#### Story 3: Update VM Network with cluster network `cluster-1`, vid `2012`,`L3 Connectivity` field set and mode `Static`
     "node1": "192.168.1.10/24",
     "node2": "192.168.1.11/24",
     "node3": "192.168.1.12/24"

A vlan sub interface `cluster-1-br.2012` is created on all 3 nodes in the cluster and ip address "192.168.1.10/24" assigned to interface `cluster-1-br.2012` of node 1,
"192.168.1.11/24" assigned to `cluster-1-br.2012` of node 2 and "192.168.1.12/24" assigned to interface `cluster-1-br.2012` of node 3.

#### Story 4: Update VM Network with cluster network `cluster-1`, vid `2012`,`L3 Connectivity` field unset

- bridge vlan vid `2012` removed from `cluster-1-br` on node 1,node 2 and node 3
- vlan sub interface `cluster-1-br.2013` is removed from node 1,node 2 and node 3

#### Story 5: Delete VM Network with cluster network `cluster-1`, vid `2012`

- bridge vlan vid `2012` removed from `cluster-1-br` on node 1,node 2 and node 3
- vlan sub interface `cluster-1-br.2012` is removed from node 1,node 2 and node 3

#### Story 6: New node node 4 added to the cluster

- bridge vlan vid `2012` added to `cluster-1-br` on node 4
- vlan sub interface `cluster-1-br.2012` is created on node 4 in the cluster and ip address is assigned from DHCP.

#### Story 7: `Node 3` removed from network config of `cluster-1` or `Node 3` shutdown

- bridge vlan vid `2012` removed from `cluster-1-br` on node 3
- vlan sub interface `cluster-1-br.2012` is removed on node 3

#### Story 8: New node node 4 added to the cluster (L3 connectivity mode `static`)

- User reconfigures VM Network `2012` with node IP "192.168.1.13/24" for node 4
- bridge vlan vid `2012` added to `cluster-1-br` on node 4
- vlan sub interface `cluster-1-br.2012` is created on node 4 in the cluster and ip address "192.168.1.12/24" is assigned to `cluster-1-br.2012`

#### Story 9: Reboot node 1, node 2 and node 3

After the nodes come up,
- bridge vlan vid `2012` added to `cluster-1-br` on node 1,node 2 and node 3
- vlan sub interface `cluster-1-br.2012` is created on node 1,node 2 and node 3 in the cluster and ip address is assigned from DHCP or static


### User Experience In Detail

User will be able to create L3 interfaces on required nodes in the cluster and assign IP address to them using static or DHCP mode and will be able to persist this across reboots.
This could be achieved either from UI by providing the network config details on the VM Network page or updating the network config info on VM Network annotations manually.

### API changes

None.

## Design

### Implementation Overview

- Support the following user configuration on the annotations of the Virtual Machine Network resource
  - IPconnectivity Required(true/false)
  - Mode(Static/DHCP)
  - Mode (Static) - List of Node to IP address mapping 

case 1: VM Network Resource with mode Static

```
apiVersion: k8s.cni.cncf.io/v1
kind: NetworkAttachmentDefinition
metadata:
  annotations:
    network.harvesterhci.io/route: >-
      {"mode":"auto","cidr":"10.115.8.0/21","gateway":"10.115.15.254","connectivity":"true"}
    network.harvesterhci.io/l3-connectivity: "true"
    network.harvesterhci.io/mode: "static"
    network.harvesterhci.io/node-ip-map: |
      {
        "node1": "192.168.1.10/24",
        "node2": "192.168.1.11/24",
        "node3": "192.168.1.12/24"
      }
  labels:
    network.harvesterhci.io/clusternetwork: cluster-1
    network.harvesterhci.io/ready: 'true'
    network.harvesterhci.io/type: L2VlanNetwork
    network.harvesterhci.io/vlan-id: '2012'
  name: vlan2012
  namespace: default
  resourceVersion: '7776036'
  uid: d1c055f0-b9d9-4573-9254-09e8c4089054
spec:
  config: >-
    {"cniVersion":"0.3.1","name":"vlan2012","type":"bridge","bridge":"cluster-1-br","promiscMode":true,"vlan":2012,"ipam":{}}
```
case 2: VM Network Resource with mode DHCP

```
apiVersion: k8s.cni.cncf.io/v1
kind: NetworkAttachmentDefinition
metadata:
  annotations:
    network.harvesterhci.io/route: >-
      {"mode":"auto","cidr":"10.115.8.0/21","gateway":"10.115.15.254","connectivity":"true"}
    network.harvesterhci.io/l3-connectivity: "true"
    network.harvesterhci.io/mode: "dhcp"
  labels:
    network.harvesterhci.io/clusternetwork: cluster-1
    network.harvesterhci.io/ready: 'true'
    network.harvesterhci.io/type: L2VlanNetwork
    network.harvesterhci.io/vlan-id: '2012'
  name: vlan2012
  namespace: default
  resourceVersion: '7776036'
  uid: d1c055f0-b9d9-4573-9254-09e8c4089054
spec:
  config: >-
    {"cniVersion":"0.3.1","name":"vlan2012","type":"bridge","bridge":"cluster-1-br","promiscMode":true,"vlan":2012,"ipam":{}}
```

- Harvester network controller handles the nad change and updates the annotations of the cluster network with the following.
  - VLAN-ID (2 - 4094)
  - Mode (Static or DHCP)
  - List of nodenames to IP address mapping (if Mode = Static)

case 1: Cluster Network Resource with mode Static after annotations updated from NAD:

```
apiVersion: network.harvesterhci.io/v1beta1
kind: ClusterNetwork
metadata:
  annotations:
    network.harvesterhci.io/mtu-source-vc: vlanconfig-1
    network.harvesterhci.io/uplink-mtu: '1500'
    network.harvesterhci.io/l3connectivity-nad2012: "true"
    network.harvesterhci.io/vlan-id-nad2012: "2012"
    network.harvesterhci.io/mode-nad2012: "static"
    network.harvesterhci.io/ipmap-default-nad2012: |
      {
        "node1": "192.168.1.10/24",
        "node2": "192.168.1.11/24",
        "node3": "192.168.1.12/24"
      }
  name: cluster-1
```
case 2: Cluster Network Resource with mode DHCP after annotations updated from NAD:

```
apiVersion: network.harvesterhci.io/v1beta1
kind: ClusterNetwork
metadata:
  annotations:
    network.harvesterhci.io/mtu-source-vc: vlanconfig-1
    network.harvesterhci.io/uplink-mtu: '1500'
    network.harvesterhci.io/l3connectivity-nad2012: "true"
    network.harvesterhci.io/vlan-id-nad2012: "2012"
    network.harvesterhci.io/mode-nad2012: "dhcp"
  name: cluster-1
```

Create/Update:

- Harvester network controller cluster network agent running as daemonset on each of the node handles the above update from nad and does the following on the host.
  - Adds/updates the vlan id to the bridge (`bridge vlan add vid <vlan-id> dev <bridgename> self`) through netlink.
  - Creats/updates file named cluster-1-vlan2012.nmconnection with config (vlan-id,cluster bridge name, mode,ipaddress if static mode) under /etc/NetworkManager/system-connections on each host.
  - If the mode is `DHCP`, the network manager takes care of requesting IP address from the DHCP server for the vlan sub interface.
  - If the mode is `static`, the network controller cluster network agent compares the nodename from the annotation list for the vid to its nodename and writes the ip address to the nmconnection profile.
  - Skipped on nodes which does not have `uplink` (bridge link is not created when networkconfig not created for a node)

- Create a systemd service during harvester-installation which monitors for file create/update in the path /etc/NetworkManager/system-connections and invokes a service to execute a script to run 
  `nmcli connection reload`

- The vlan interface will be created and brought up by network manager on `nmcli connection reload` and ip will be assigned to the interface.

Delete:

- Harvester network controller cluster network agent running as daemonset on each of the node handles the above update from nad and does the following on the host.
  - Deletes the vlan id from the bridge (`bridge vlan del vid <vlan-id> dev <bridgename> self`) through netlink.
  - Deletes file cluster-1-vlan2012.nmconnection under /etc/NetworkManager/system-connections on each host.
  - Skipped on nodes which does not have `uplink` (bridge link is not created when networkconfig not created for a node)

- The systemd service created during harvester-installation deletes the nmconnection profile file under the path /etc/NetworkManager/system-connections and invokes a service to execute a script to run 
  `nmcli connection reload`

- The vlan interface will be removed on  `nmcli connection reload`.

nmconnection profiles:

```
[connection]
id=vlan-mgmt
uuid=d36f3083-03f3-31dc-b871-0e32dbb3476c
type=vlan
timestamp=1760411842

[ethernet]

[vlan]
flags=1
id=2012
parent=cluster-1-br

[ipv4]
address1=192.168.1.10/24,192.168.1.1
dns=8.8.8.8;
method=manual

```
```
[connection]
id=vlan-mgmt
uuid=136f5099-0431-42cc-c982-1e32dcc5287d
type=vlan
timestamp=1760411843

[ethernet]

[vlan]
flags=1
id=2012
parent=cluster-1-br

[ipv4]
dns=8.8.8.8;
method=auto

```

Steps to create a systemd service to monitor file creation under /etc/NetworkManager/system-connections/ on the host

1.Create nm-reload.path file under /etc/systemd/system/ with the following contents

```
[Unit]
Description=Watch /etc/NetworkManager/system-connections/ for changes to nmconnection profiles

[Path]
PathModified=/etc/NetworkManager/system-connections/
PathChanged=/etc/NetworkManager/system-connections/

Unit=nm-reload-dispatch.service

[Install]
WantedBy=multi-user.target

```

2.Create nm-reload-dispatch.service file under /etc/systemd/system/ with the following contents

```
[Unit]
Description=Dispatch wicked ifreload based on ifcfg-* changes

[Service]
Type=oneshot
ExecStart=/tmp/nm-reload-dispatch.sh
```

3.Create a reload script which executes `nmcli connection reload` from the dispatch service

```
#!/bin/bash

SERVICE_NAME="nm-path-reload"

logger -t "$SERVICE_NAME" "Change detected in system-connections — reloading NetworkManager connections"

nmcli connection reload

if [ $? -eq 0 ]; then
    logger -t "$SERVICE_NAME" "nmcli connection reload successful"
else
    logger -t "$SERVICE_NAME" "nmcli connection reload failed"
fi

```
For network controller to write files to the host path, a volume from host directory must be mounted into the container. 
Network controller creates nmconnection files under /host-network.

```
spec:
  template:
spec:
  containers:
    volumeMounts:
    - mountPath: /host-network
      name: host-network-config
  volumes:
      - hostPath:
          path: /etc/NetworkManager/system-connections/
          type: Directory
        name: host-network-config
```

Example output of `ip addr show` and `bridge vlan show`

```
122: cluster-br-1.2012@cluster-1-br: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default qlen 1000
    link/ether 02:a5:50:16:5d:f9 brd ff:ff:ff:ff:ff:ff
    inet 10.115.14.230/21 brd 10.115.15.255 scope global clusbr.2012
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
### Test plan

Integration test plan.

### Upgrade strategy

Anything that requires if user want to upgrade to this enhancement

## Note [optional]

Additional notes.
