Rancher Harvester
========
[![Build Status](https://drone-publish.rancher.io/api/badges/rancher/harvester/status.svg)](https://drone-publish.rancher.io/rancher/harvester)
[![Go Report Card](https://goreportcard.com/badge/github.com/rancher/harvester)](https://goreportcard.com/report/github.com/rancher/harvester)

Rancher Harvester is an open source [hyper-converged infrastructure](https://en.wikipedia.org/wiki/Hyper-converged_infrastructure) (HCI) software built on Kubernetes. It is an open source alternative to vSphere and Nutanix.

![harvester-ui](./docs/assets/harvester-ui.png)

## Overview
Harvester implements HCI on bare metal servers. Here are some notable features of the Harvester:
1. VM lifecycle management including SSH-Key injection, Cloud-init and, graphic and serial port console
1. Distributed block storage
1. Multiple NICs connecting to the management network or VLANs
1. ISO image repository
1. Virtual Machine templates

The following diagram gives a high-level architecture of Harvester:

![](./docs/assets/architecture.png)

- [MinIO](https://min.io/) is a cloud storage server compatible with Amazon S3.
- [Longhorn](https://longhorn.io/) is a lightweight, reliable and easy-to-use distributed block storage system for Kubernetes.
- [KubeVirt](https://kubevirt.io/) is a virtual machine management add-on for Kubernetes.
- [K3OS](https://k3os.io/) is a Linux distribution designed to remove as much OS maintenance as possible in a Kubernetes cluster. The OS is designed to be managed by kubectl.

## Hardware Requirements
To get the Harvester server up and running the following minimum hardware is required:

| Type | Requirements |
|:---|:---|
| CPU | 4 cores minimum, 16 cores or above preferred |
| Memory | 8 GB minimum, 32 GB or above preferred |
| Disk |  120 GB minimum, 500 GB or above preferred |
| Network Card | 1 Gbps Ethernet minimum, 10Gbps Ethernet recommended |
| Network Switch | Trunking of ports required for VLAN support |

## Installation
Harvester supports two modes of installation:

### Bare-metal
In the `Bare-metal` mode, users can use the ISO to install Harvester directly on the bare-metal server to form a Harvester cluster. Users can add one or many compute nodes to join the existing cluster.

To get the Harvester ISO, download it from the [Github releases.](https://github.com/rancher/harvester/releases)

During installation you can either choose to form a new cluster, or join the node to an existing cluster.

Note: This [video](https://youtu.be/97ADieBX6bE) shows a quick overview of the ISO installation.

1. Mount the Harvester ISO disk and boot the server by selecting the `Harvester Installer`.
![iso-install.png](./docs/assets/iso-install.png)
1. Choose the installation mode by either creating a new Harvester cluster, or by joining an existing one.
1. Choose the installation device that the Harvester will be formatted to.
1. Configure the `cluster token`. This token will be used for adding other nodes to the cluster.
1. Configure the login password of the host. The default ssh user is `rancher`.
1. (Optional) you can choose to import SSH keys from a remote URL server. Your GitHub public keys can be used with `https://github.com/<username>.keys`.
1. Select the network interface for the management network.
1. (Optional) If you need to use an HTTP proxy to access the outside world, enter the proxy URL address here, otherwise, leave this blank.
1. (Optional) If you need to customize the host with cloud-init config, enter the HTTP URL here.
1. Confirm the installation options and the Harvester will be installed to your host. The installation may take a few minutes to be complete.
1. Once the installation is complete it will restart the host and a console UI with management URL and status will be displayed. <small>(You can Use F12 to switch between Harvester console and the Shell)</small>
1. The default credentials for the web interface are username:`admin` and password: `password`.
![iso-installed.png](./docs/assets/iso-installed.png)


### App [Development Mode]
In the `App` mode, the user can deploy Harvester using [Helm](https://github.com/rancher/harvester/tree/master/deploy/charts/harvester) to an existing Kubernetes cluster.

Note: Hardware-assisted virtualization must be supported on the Kubernetes nodes.


## Documentation
Please refer to the following documentation to find out more details:
- Installation
  * [ISO Mode](#bare-metal)
  * [App Mode - for development](./docs/app-mode-installation.md)
- [Authentication](./docs/authentication.md)
- [Upload Images](./docs/upload-image.md)
- [Create a VM](./docs/create-vm.md)
- [Access to the VM](./docs/access-to-the-vm.md)

Demo: Check out this [demo](https://youtu.be/wVBXkS1AgHg) to get a quick overview of the Harvester UI.


## Source code
Harvester is 100% open-source software. The project source code is spread across a number of repos:

| Name | Repo Address |
|:---|:---|
| Harvester | https://github.com/rancher/harvester |
| Harvester UI | https://github.com/rancher/harvester-ui |
| Harvester Installer | https://github.com/rancher/harvester-installer |
| Harvester Network Controller | https://github.com/rancher/harvester-network-controller|

Check out this [demo](https://youtu.be/wVBXkS1AgHg) to get a quick overview of the Harvester UI.


## Community
If you need any help with Harvester, please join us at either our [Rancher forums](https://forums.rancher.com/) or [Slack](https://slack.rancher.io/) where most of our team hangs out at.

If you have any feedback or questions, feel free to [file an issue](https://github.com/rancher/harvester/issues/new/choose).


## License
Copyright (c) 2021 [Rancher Labs, Inc.](http://rancher.com)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
