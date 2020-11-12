# Harvester-Network-Controller Helm Chart

[Harvester Network Contrller](https://github.com/rancher/harvester-network-controller) is a network controller that helps to manage the host network configuration of the Harvester cluster.

Introduction
------------

This chart installs the network-controller daemonset on the [harvester](https://github.com/rancher/harvester) cluster using the [Helm](https://helm.sh) package manager.

Prerequisites
-------------
- [multus-cni](https://github.com/intel/multus-cni) v3.6+
- Vlan filtering support on bridge
- Switch to support `trunk` mode

