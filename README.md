Rancher Harvester
========
[![Build Status](https://drone-publish.rancher.io/api/badges/rancher/harvester/status.svg)](https://drone-publish.rancher.io/rancher/harvester)

Rancher Harvester is an open source [hyper-converged infrastructure](https://en.wikipedia.org/wiki/Hyper-converged_infrastructure) (HCI) solution based on Kubernetes. It is an on-prem cloud computing virtualization platform.

## Overview
Harvester makes it easy to get HCI up and running on the bare metal servers. Here are some notable features of the Harvester that currently provided:
1. VM lifecycle management includes SSH-Key injection, Cloud-init and, graphic and serial port console
1. Build-in storage management based on [Longhorn](https://longhorn.io/)
1. Networking management with both overlay management networks and VLAN
1. Built-in image repository
1. Virtual Machine templates

Check out this [demo](https://youtu.be/wVBXkS1AgHg) to get a quick overview of the Harvester UI.

## Hardware Requirements
To get the Harvester server up and running the following minimum hardware requirements are required:

| Type | Minimum Requirements |
|:---|:---|
| CPU | 4 cores required, a minimum of 16 cores is preferred |
| Memory | 8 GB is required, a minimum of 32 GB is preferred |
| Disk |  at least 120 GB is required, 500 GB or above is preferred |
| Network Card | at least 1 GB Ethernet port, for example, Ethernet 1GbE, RJ45  |
| Network Switch | Equipped with at least 1 GB switch, 10 GB switch is recommended (Will need to enabling trunking of port for VLAN support) |

## Installation
Harvester supports two modes of installation:

### Bare-metal
In the `Bare-metal` mode, users can use the ISO to install Harvester directly on the bare-metal server to form a Harvester cluster. Users can add one or many compute nodes to join the existing cluster. A standalone Harvester node can still allow users to create and manage the virtual machines.

You can find the Harvester [ISO image](https://github.com/rancher/harvester/releases) from the Github releases.

To find more detail about ISO installation, please refer to the ISO docs [here](./docs/iso-installation.md).


### App [Development Mode]
In the `App` mode, the user can deploy Harvester using [Helm](https://github.com/rancher/harvester/tree/master/deploy/charts/harvester) to an existing Kubernetes cluster.

Note: Hardware-assisted virtualization must be supported on the Kubernetes nodes.


## Documentation
Please refer to the docs [here](./docs).


## Source code
Harvester is 100% open-source software. The project source code is spread across a number of repos:

| Name | Repo Address |
|:---|:---|
| Harvester UI | https://github.com/rancher/harvester-ui |
| Harvester Installer | https://github.com/rancher/harvester-installer |
| Harvester Network Controller | https://github.com/rancher/harvester-network-controller|


## Community
If you need any help with Harvester, please join us at either our [Rancher forums](https://forums.rancher.com/) or [Slack](https://slack.rancher.io/) where most of our team hangs out at.

If you have any feedback or questions, feel free to [file an issue](https://github.com/rancher/harvester/issues/new/choose).


## License
Copyright (c) 2020 [Rancher Labs, Inc.](http://rancher.com)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
