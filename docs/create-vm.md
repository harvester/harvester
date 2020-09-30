# How to Create VM

Users can choose to create one or multiple virtual machines from the `Virtual Machine` page.

1. Choose the option to create one or multiple VM instances.
1. VM name and namespace are required.
1. (Optional) you can select to use the VM template, by default we have added iso, raw, and Windows image template.
1. Select a custom VM image
1. Config the CPU and Memory of the VM
1. Add disks to the VM, the default disk will be the root disk  
1. Config the networks, the `Pod Network` is added by default, it is also possible to connect to VMs to secondary networks using [Multus](#networks).
1. add SSH keys in the Authentication section.
1. You can config advanced options like hostname, cloud-init data in the `Advanced Details` section.

![](./assets/create-vm.png)

#### Networks

##### pod network

A pod network represents the default pod eth0 interface configured by the cluster network solution that is present in each pod, VM can be accessed via the pod network. 

For the [RKE](https://rancher.com/docs/rke/latest/en/) cluster please ensure the `ipv4.ip_forward` is enabled of the CNI plugin to make the pod network to work as expected. [#94](https://github.com/rancher/harvester/issues/94). 

##### multus
It is also possible to connect VMs to secondary networks using [Multus](https://github.com/intel/multus-cni). This assumes that multus is installed across your cluster and a corresponding NetworkAttachmentDefinition CRD was created.