# VLAN Enhancement

## Summary

IT usually configures the switch with VLAN trunk mode rather than VLAN access mode or hybrid mode in a production environment. Therefore, we should add VLAN support for `harvester-mgmt` and allow multiple Linux bridges in Harvester node configuration.

### Related Issues

- https://github.com/harvester/harvester/issues/1390

## Terminology

| Term | Short Term | Description |
| --- | --- | --- |
| management interface | mgmt intf | mgmt intf is the owner of node IP. In previous version, mgmt intf is `harvester-mgmt`.|
| VLAN Trunk mode | - | The configuration that only allow VLAN tagged packet for ingress and egress. |
| VLAN Hybrid mode | - | The configuration that allow VLAN tagged and untagged packet for ingress and egress. Need configured Native VLAN. |
| VLAN Access mode | - | The configuration that only allow untagged packet for ingress and egress. In VLAN-aware switch, it will need a VLAN ID to be configured also. |
| native vlan | - | The configuration for popping vlan tag when egress, pushing vlan tag when ingress |
| PVID | - | Same as natvie vlan in Linux bridge. |
| VLAN interface | vlan intf | SVI in vlan-aware switch. eth0.100 in Linux. |
| uplink | - | The network interface is for Linux bridge to send traffic to outside the node. |
| network attached definition | NAD, nad, net-attach-def | Network configuration CRD for Multus CNI |

## Motivation

### Goals

- Migrate node IP from `harvester-mgmt` to the new management interface to allow VLAN tagged packet for management traffic.
- Allow multiple Linux bridges to be configured by Harvester
- Allow the same name Linux bridge with different uplinks

### Non-goals [optional]

What is out of scope for this enhancement? Listing non-goals helps to focus discussion and make progress.

TODO

## Proposal

### User Stories

#### Switch configuration needs VLAN hybrid mode always for data center

Before this enhancement, users have to configure the VLAN-aware switch with VLAN hybrid mode for the ports because the management interface will only work in untagged mode.

After this enhancement, users can setup VLAN ID for the management interface.

### User Experience In Detail

#### Installer UI

We will have another option for adding VLAN ID to the management interface.

#### Harvester config

In Harvester config, we will create more network devices to implement this enhancement, so users need to add those interfaces on their own.

> **NOTE**: Maybe we could change the network section of Harvester config to have an abstract layer above `wicked`.

#### Web UI

In Web UI, we will need an advanced network configuration page to configure additional custom Linux bridges to serve VM VLAN networks.


### API changes

## Design

### Implementation Overview

- In the installer, we will create a Linux bridge named `br0` by default and attach `harvester-mgmt` to it by default. The management interface is `br0` here.
- In the installer, if users configure VLAN ID for the management interface, we will create another VLAN interface named `br0.<VLAN ID>` and configure node IP on it.
- `br0` is always available and fixed.
- Network attached definition can't attach to `br0`.
- Harvester Network Controller will create a veth pair to connect `br0` and user defined `br<X>` for uplink if users want to use same interface of `harvester-mgmt` or management interface to provide VM VLAN networks.
- `br0` and vlan interface need to inherit first MAC address from physical network interfaces of `harvester-mgmt`.
    - `wicked` need to set a fixed MAC address for `br0` and vlan interface.
- The uplink of `br0` and `br<X>` is always a `bond`.
- VLAN filtering in `br0` and `br<X>` is always enable.
    - PVID is always 1.
    - Always use VLAN 1 as untagged network.
- Harvester Network controller don't need to copy IP address and route from `harvester-mgmt` to bridge anymore
    - In installation statge, we will generate DHCP or Static IP configuration on `br0` or vlan interface.

> **NOTE**: We could consider to migrate all Linux bridges' name to `br<X>` where `<X>` is a decimal number. In original design, The name `harvester-br0` is too long to have a VLAN interface with same naming rule of `eth0.100`.

#### Proof of Concept

The following commands are executed by root account or `sudo`.

##### Basic

```
# need to set fixed mac address otherwise bridge will generate MAC address randomly.
ip link add link br0 address 11:22:33:44:55:66 type bridge vlan_filtering 1
ip link add harvester-mgmt master br0
# inherit from br0 directly.
ip link add link br0.100 type vlan id 100

for i in {2..4094}; do bridge vlan add vid $i dev harvester-mgmt; done
bridge vlan add vid 100 dev br0 self

ip link set br0 up
ip link set br0.100 up

ip addr add 192.168.0.2/24 dev br0.100
ip route add default via 192.168.0.1

# check everything is correct

# check MAC address
ip link show dev eth0
ip link show dev harvester-mgmt
ip link show dev br0
ip link show dev br0.100
```

##### Use `harvester-mgmt` as VLAN network uplink

```
# need same setup in Basic section

ip link add br1 type bridge vlan_filtering 1
ip link add veth-br0 type veth peer name veth-other
ip link set veth-br0 master br0
ip link set veth-other master br1

for i in {2..4094}; do bridge vlan add vid $i dev veth-br0; done
for i in {2..4094}; do bridge vlan add vid $i dev veth-other; done

ip link set br1 up
ip link set veth-br0 up
ip link set veth-other up

# create VM veth
ip link add veth-in type veth peer name veth-out
ip link set veth-out master br1
bridge vlan add vid 101 dev veth-out pvid untagged master
```

##### Use other physical interface for VLAN network uplink

```
# need same setup in Basic section

ip link add br2 type bridge vlan_filtering 1
ip link add bond-br2 type bond mode balance-tlb
ip link set eth2 master bond-br2
ip link set bond-br2 master br2

for i in {2..4094}; do bridge vlan add vid $i dev bond-br2; done

ip link set eth2 up
ip link set bond-br2 up
ip link set br2 up

# create VM veth
ip link add veth-in type veth peer name veth-out
ip link set veth-out master br2
bridge vlan add vid 101 dev veth-out pvid untagged master
```

#### Upgrade

> **TODO**: Huge migratation script need.

### Test plan

Integration test plan.

### Upgrade strategy

Anything that requires if user want to upgrade to this enhancement

## Note [optional]

Additional nodes.
