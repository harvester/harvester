# NIC Hotplug

## Summary

This enhancement leverages KubeVirt's network interface hot-plug feature to enable users to add or remove specific types of Network Interface Controllers (NICs) from a running Virtual Machine (VM).

### Related Issues

https://github.com/harvester/harvester/issues/7042

## Motivation

### Goals

- Enable users to hot-plug a `virtio` interface connected via `bridge` or `SR-IOV` binding to a running VM without downtime.
- Enable users to hot-unplug an interface connected via `bridge` binding from a running VM without downtime.

### Non-goals

- Support for hot-plugging interfaces with models not supported by KubeVirt.
- Support migration based hotplug; the LiveUpdate roll-out strategy is preferred.
- Addressing existing Virtio limitations.

## Proposal

### User Stories

#### Story 1: Hot-plug an interface using the `virtio` model

A user needs to add a new network interface to a running VM to connect it to a new network. Previously, this required scheduling downtime to shut down the VM, add the interface, and restart it. With this enhancement, the user can add a `virtio` interface using `bridge` or `SR-IOV` binding to the running VM, making the new network available immediately without service interruption.

#### Story 2: Hot-unplug an interface connected through `bridge` binding

A user needs to reconfigure a VM's network topology by removing an obsolete network interface. Instead of shutting down the VM, the user can now hot-unplug the `bridge`-bound interface from the running VM, simplifying network management and avoiding downtime.

### User Experience In Detail

The following steps outline the user workflow for managing network interfaces on a running VM:

1.  Navigate to the details page of a running VM.
2.  On the **Networks** tab, interfaces that support hot-unplug will display a corresponding action button (e.g., "Delete" or "Unplug").
3.  To hot-plug an interface, the user can click the kebab menu (three vertical dots) on either the VM details page or the VM list page and select the **Add Network Interface** option. A dialog will appear, prompting the user to specify an interface name and select a network from a dropdown list.

### API changes

Two new API actions will be introduced for the `VirtualMachine` resource.

**Add NIC**
```
POST /v1/harvester/vm/<vm>?action=addNic
Content-Type: application/json

{
  "interfaceName": "dynif1",
  "networkName": "default/vmnet1",
  "bindingMethod": "bridge"  // or "sriov"
}
```

**Remove NIC**
```
POST /v1/harvester/vm/<vm>?action=removeNic
Content-Type: application/json

{
  "interfaceName": "dynif1"
}
```

## Design


### Implementation Overview

#### Satisfy KubeVirt Hot-plug Network Interface Requirements

According to the KubeVirt documentation[^1], two requirements must be met:

1.  Multus CNI must be deployed using a [thick plugin](https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/thick-plugin.md) architecture.
2.  The Multus Dynamic Networks Controller must be installed.

As per the RKE2 documentation, these requirements can be met by applying the appropriate `HelmChartConfig` via the Harvester installer.

```yaml
# pkg/config/templates/rancherd-23-multus-config.yaml
resources:
- apiVersion: helm.cattle.io/v1
  kind: HelmChartConfig
  metadata:
    name: rke2-multus
    namespace: kube-system
  spec:
    valuesContent: |-
      thickPlugin:
        enabled: true
      dynamicNetworksController:
        enabled: true
```

#### Network Interface Hot-plug Actions

The API server will provide two new actions, `addNic` and `removeNic`, for `VirtualMachine` resources.

-   `addNic` will patch the `VirtualMachine` resource by adding new elements to `spec.template.spec.domain.devices.interfaces` and `spec.template.spec.networks`. The interface `model` will be hardcoded to `virtio`, and only `bridge` and `SR-IOV` binding methods will be permitted.

-   `removeNic` will patch the `VirtualMachine` resource by setting the `state` of the specified interface in `spec.template.spec.domain.devices.interfaces` to `absent`. This action will only be permitted for interfaces using the `bridge` binding method.

### Test plan

#### Regression tests

Since this enhancement migrates the Multus CNI from a thin to a thick plugin architecture, all existing network functionalities, including the storage network, must be thoroughly tested to prevent regressions.

#### Functional tests

1.  Verify that a NIC with the `bridge` binding method can be added to a running VM.
2.  Verify that a NIC with the `SR-IOV` binding method can be added to a running VM.
3.  Verify that attempts to add a NIC with any other binding method are blocked.
4.  Verify that a NIC with the `bridge` binding method can be removed from a running VM.
5.  Verify that attempts to remove a NIC with any other binding method are blocked.

### Upgrade strategy

The NIC hot-plug feature must be compatible with VMs created prior to the upgrade. Additionally, the migration of Multus CNI from the thin to the thick plugin architecture must be verified to ensure a seamless Harvester upgrade process.

## Note

The KubeVirt documentation[^1] specifies a maximum number of interfaces that can be hot-plugged. This limitation should be clearly communicated in the Harvester user documentation.

A `RestartRequired` condition has been observed on the `VirtualMachine` resource after a NIC hot-plug operation is followed by a VM reboot. This appears to be incorrect behavior, and we will coordinate with the KubeVirt community to investigate and resolve the issue.


[^1]: https://kubevirt.io/user-guide/network/hotplug_interfaces/
