# How to Create a VM

Create one or more virtual machines from the **Virtual Machines** page.

1. Choose the option to create either one or multiple VM instances.
1. The VM name is required.
1. (Optional) you can select to use the VM template. By default we have added ISO, raw, and Windows image templates.
1. Configure the CPU and Memory of the VM.
1. Select a custom VM image.
1. Select SSH keys or upload a new one.
1. To add more disks to the VM, go to the **Volumes** tab. The default disk will be the root disk.
1. To configure networks, go to the **Networks** tab. The **Management Network** is added by default. It is also possible to add secondary networks to the VMs using vlan networks (configured on **Advanced > Networks**).
1. Optional: Configure advanced options like hostname and cloud-init data in the **Advanced Options** section.

![](./assets/create-vm.png)

### Cloud config examples

Config for the password of the default user:

```YAML
#cloud-config
password: password
chpasswd: { expire: False }
ssh_pwauth: True
```

Network-data configuration using DHCP:

```YAML
version: 1
config:
  - type: physical
    name: eth0
    subnets:
      - type: dhcp
  - type: physical
    name: eth1
    subnets:
      - type: dhcp
```

You can also use the `Cloud Config Template` feature to include a pre-defined cloud-init config for the VM.

#### Networks

##### Management Network

A management network represents the default vm eth0 interface configured by the cluster network solution that is present in each VM.

By default, a VM can be accessed via the management network. 

##### Secondary Network

It is also possible to connect VMs using additional networks with Harvester's built-in [vlan networks](./harvester-network.md).
