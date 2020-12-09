# How to Create VM

Users can choose to create one or multiple virtual machines from the `Virtual Machines` page.

1. Choose the option to create either one or multiple VM instances.
1. The VM name is required.
1. (Optional) you can select to use the VM template, by default we have added iso, raw, and Windows image template.
1. Config the CPU and Memory of the VM
1. Select a custom VM image
1. Select SSH keys or upload a new one if not exist.
1. [Volumes Tab] Users can choose to add more disks to the VM, the default disk will be the root disk  
1. [Networks Tab] Config the networks, the `Management Network` is added by default, it is also possible to add to secondary networks to the VMs using vlan networks(configured on the Advanced -> Networks).
1. [Advanced Options] You can config advanced options like hostname, cloud-init data in the `Advanced Options` section.

![](./assets/create-vm.png)

#### Networks

##### Management Network

A management network represents the default vm eth0 interface configured by the cluster network solution that is present in each VM, by default VM can be accessed via the management network. 

For App mode, if you are using the [RKE](https://rancher.com/docs/rke/latest/en/) cluster please ensure the `ipv4.ip_forward` is enabled of the CNI plugin to make the pod network to work as expected. [#94](https://github.com/rancher/harvester/issues/94). 

##### Secondary Network
It is also possible to connect VMs using the secondary networks with build-in vlan networks, this is powered by [Multus](https://github.com/intel/multus-cni). 

For App mode, it is assumed that multus is installed across your cluster and a corresponding NetworkAttachmentDefinition CRD was created.