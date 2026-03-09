# NIC Hotplug

## Summary

This enhancement leverages KubeVirt's network interface hot-plug feature to enable users to add or remove specific types of Network Interface Controllers (NICs) from a running Virtual Machine (VM).

### Related Issues

https://github.com/harvester/harvester/issues/7042

## Motivation

### Goals

- Enable users to hot-plug or hot-unplug a `virtio` interface connected via `bridge` binding to a running VM without downtime if the VM is live-migratable.

### Non-goals

- Support for hot-plugging or hot-unplugging interfaces with models other than `virtio`.
- Support for hot-plugging interfaces with `SR-IOV` binding method: In Harvester, [SR-IOV Network devices](https://docs.harvesterhci.io/v1.6/advanced/addons/pcidevices/#sriov-network-devices) are managed by [our own device plugin framework](https://github.com/harvester/pcidevices) and are assigned to VM via [the hostDevices field](https://kubevirt.io/user-guide/compute/host-devices/#starting-a-virtual-machine). Given that this is not the typical method shared in [KubeVirt's document](https://kubevirt.io/user-guide/network/interfaces_and_networks/#sriov), we would postpone this feature to the future enhancement.
- Support non-migration based hotplug with the LiveUpdate roll-out strategy: Multus thick plugin, which is one of the requirement, currently brings [a severe risk of race condition preventing pods from starting after a node reboot](https://github.com/k8snetworkplumbingwg/multus-cni/issues/1221). Additionally, there are a few uncertainties to the existing network related features while upgrading Multus from thin plugin to thick plugin. That being said, this is still a valuable feature for VMs that are not live-migratable to have a chance to hot-plug or hot-unplug NICs. It would be revisited in the future enhancement after the issue is fixed in the upstream.
- Addressing existing Virtio limitations.
- Support for [Overlay Network, which is provided by Kube-OVN](https://docs.harvesterhci.io/v1.6/networking/harvester-network/#overlay-network-experimental) VM networks: A custom binding interface `managedTap` is used to allow dhcp capability to work correctly.

## Proposal

### User Stories

#### Story 1: Hot-plug an interface

A user needs to add a new network interface to a running VM to connect it to a new network. Previously, this required scheduling downtime to shut down the VM, add the interface, and restart it. With this enhancement, the user can add a `virtio` interface using `bridge` binding to the running VM, making the new network available immediately without service interruption.

#### Story 2: Hot-unplug an interface

A user needs to reconfigure a VM's network topology by removing an obsolete network interface. Instead of shutting down the VM, the user can now hot-unplug a `bridge`-bound `virtio` interface from the running VM, simplifying network management and avoiding downtime.

### User Experience In Detail

The following steps outline the user workflow for managing network interfaces on a running VM:

1. Navigate to the details page of a running VM.
2. On the **Networks** tab, interfaces that support hot-unplug will display a corresponding action button "Unplug".
3. To hot-plug an interface, the user can click the kebab menu (three vertical dots) on either the VM details page or the VM list page and select the **Add Network Interface** option. A dialog will appear, prompting the user to specify an interface name and select a network from a dropdown list.

### API changes

The following new API actions will be introduced for the `VirtualMachine` resource.

**Add NIC**
```
POST /v1/harvester/vm/<vm>?action=addNic
Content-Type: application/json

{
  "interfaceName": "dynif1",
  "networkName": "default/vmnet1",
  "macAddress": "da:59:6c:5e:8b:fb"  // empty string or missing field indicates the Mac address is auto-generated
}

# Response
- status code: 204
```

**Remove NIC**
```
POST /v1/harvester/vm/<vm>?action=removeNic
Content-Type: application/json

{
  "interfaceName": "dynif1"
}

# Response
- status code: 204
```

**Find hot-pluggable NICs** which returns interfaces that can be hot-unplugged for the VM
```
POST /v1/harvester/vm/<vm>?action=findHotunpluggableNics
Content-Type: application/json

# Response
- status code: 200
- body:
{
  "interfaces": [
    "dynif1",
    "dynif2"
  ]
}
```

## Design


### Implementation Overview

#### Check whether the VM is hot-pluggable / hot-unpluggable

If the VM is a guest cluster VM (with label `harvesterhci.io/creator: docker-machine-driver-harvester`), it would be not hot-pluggable / hot-unpluggable before the integration with Rancher Manager is done. This protection ensures the stability of the guest cluster.

If the VM is not live-migratable, then it isn't hot-pluggable / hot-unpluggable.

If the VM is currently in transitional state, then it isn't hot-pluggable / hot-unpluggable until it reaches the stable running state.

#### Hot-pluggable VM Networks

In the current milestone, NetworkAttachmentDefinitions with `bridge` type in their cni configs are hot-pluggable and hot-unpluggable. Namely, `UntaggedNetwork`, `L2VlanNetwork`, and `L2VlanTrunkNetwork` are supported while `OverlayNetwork` isn't.

#### Network Interface Hot-plug Actions

The API server will provide the following new actions for `VirtualMachine` resources.

- `addNic` will patch the `VirtualMachine` resource by adding new elements to `spec.template.spec.domain.devices.interfaces` and `spec.template.spec.networks`. The interface `model` will be hardcoded to `virtio` while the binding method would be `bridge`. The network must be a Hot-pluggable VM Network.

- `removeNic` will patch the `VirtualMachine` resource by setting the `state` of the specified interface in `spec.template.spec.domain.devices.interfaces` to `absent`. This action will only be permitted for interfaces using the `bridge` binding method.

- `findHotunpluggableNics` will check elements in `spec.template.spec.domain.devices.interfaces` and `spec.template.spec.networks`, then get a list of names of interfaces that satisfied the following conditions.
  - the model is `virtio`.
  - the binding method is `bridge`.
  - the network source is multus and the reference NetworkAttachmentDefinition must be a Hot-pluggable VM Network.

All these actions will be available when the VM is possible to hot-plug NICs:
- It has to be live-migratable.
- MAC addresses of all the interfaces should be present in the VM spec.

#### Adjust the MAC addresses preserving strategy

A `RestartRequired` condition has been observed on the `VirtualMachine` resource after a NIC hot-plug operation is followed by a VM reboot. This appears to be related to our previous implementation for preserving MAC addresses across reboots. The MAC addresses would be in the spec of VMI but not in the spec of VM after the first reboot. This confuses the VM controller and causes the unpexected `RestartRequired` condition. Details can be found in these threads: https://github.com/harvester/harvester/pull/8339, https://github.com/harvester/harvester/issues/6844#issuecomment-2551020204, https://github.com/harvester/harvester/pull/7402, https://github.com/harvester/harvester/pull/7084.

##### For the new VMs, directly store the auto-generated MAC addresses while creating VMs

Generate random locally administered unicast MAC addresses:
- Set the Local Bit (the 2nd least significant bit) to 1.
- Clear the Multicast Bit (the least significant bit) to 0, ensuring Unicast.

The existing validating webhook for creating VM could ensure the cluster-wise uniqueness of MAC addresses.

##### For the old VMs, backfill the observed MAC addresses if missing while stopping

The controller would backfill the observed MAC addresses from the annotation to the spec of VM while it detects `spec.runStategy` is set to `Halted`. This indicates that, for those existing VMs with missing MAC addresses in their spec, they need to be stopped first to enable NIC Hotplug even they were already live-migratable.

### Test plan

#### Functional tests

1. Verify that a `virtio` model NIC with the `bridge` binding method can be hot-plugging to a running VM.
2. Verify that a `virtio` model NIC with the `bridge` binding method can be hot-unplugging from a running VM.
3. Verify that only Hot-pluggable VM Networks should be selectable while hot-plugging.
4. Verify that only the expected interfaces should be returned while listing hot-unpluggable NICs.
5. Verify that non-live-migratable VMs or VMs with missing MAC addresses in the spec shouldn't be allowed to hotplug NICs.

### Upgrade strategy

N/A

## Note

The KubeVirt documentation[^1] specifies a maximum number of interfaces that can be hot-plugged. This limitation should be clearly communicated in the Harvester user documentation.


[^1]: https://kubevirt.io/user-guide/network/hotplug_interfaces/
