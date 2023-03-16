# SR-IOV Network Devices

## Summary

As of harvester v1.1.x, support for PCI device passthrough to workload VMs has been introduced. 

Building on top of this functionality, we can now extend support for passing through network virtual functions (VFs) to workload VMs.

### Related Issues

https://github.com/harvester/harvester/issues/2763

## Motivation

### Goals

- Enable Harvester to identify SR-IOV capable network devices.

- Allow users to define the number of virtual functions (VFs) on a network device.

- Support passthrough of the newly created network virtual functions as any other PCI devices.

### Non-goals

Custom driver installation for network devices.

## Proposal

### User Stories

#### Configure and passthrough virtual network functions

### User Experience In Detail
A harvester user wants to leverage SR-IOV capable virtual network functions for workload VM's running in a cluster.

Currently this is not possible as the user can only passthrough the entire network physical function to a VM, and this cannot be shared among multiple workload VM's running on the same node.

Once this change is implemented, the user will be able to browse SR-IOV capable network devices on the cluster, and define the number of VF's required to be configured for specific devices. The newly created network virtual functions, show up as regular pcidevices, which can now be passed through to VM's like any other pci device.

### API changes
No API changes will be introduced to core harvester. The additional CRD's will be managed by the pcidevices controller, as a result the end users will need to enable pcidevices controller addon to leverage this capability

## Design
The controller introduces a new CRD: sriovnetworkdevices.devices.harvesterhci.io

### Implementation Overview
The pcidevices controller addon when deployed runs a daemonset on the cluster.

One of the reconcile loops in the cluster, scans the nodes at a fixed interval (30 seconds) for pcidevices.

As part of this reconcile it will scan for network devices, and check the following:
* network device is not part of management bond or bridge interface
* network device is not being used in a vlan configuration for additional cluster networks
* network device is SR-IOV capable

If a network device meets these criteria then a sriovnetworkdevices crd is created in the format $NODENAME-$InterfaceName

```yaml
apiVersion: devices.harvesterhci.io/v1beta1
kind: SriovNetworkDevice
metadata:
  annotations:
    sriov.devices.harvesterhi.io/interface-name: eno49
  creationTimestamp: "2023-03-09T01:36:30Z"
  generation: 4
  labels:
    nodename: harvester-659jw
  name: harvester-659jw-eno49
  resourceVersion: "3268097"
  uid: e4caa2b2-ca5d-48e4-bd47-b80986e5b1c8
spec:
  address: "0000:04:00.0"
  nodeName: harvester-659jw
  numVFS: 4
```

A user can now edit the sriovnetworkdevice object and define the numVFS.

A value higher than 0 will result in sriovnetworkdevice controller configuring the correct number of virtual functions, and reporting the pci device address for corresponding virtual functions.


```yaml
apiVersion: devices.harvesterhci.io/v1beta1
kind: SriovNetworkDevice
metadata:
  annotations:
    sriov.devices.harvesterhi.io/interface-name: eno49
  creationTimestamp: "2023-03-09T01:36:30Z"
  generation: 4
  labels:
    nodename: harvester-659jw
  name: harvester-659jw-eno49
  resourceVersion: "3268097"
  uid: e4caa2b2-ca5d-48e4-bd47-b80986e5b1c8
spec:
  address: "0000:04:00.0"
  nodeName: harvester-659jw
  numVFS: 4
status:
  status: sriovNetworkDeviceEnabled
  vfAddresses:
  - "0000:04:10.0"
  - "0000:04:10.2"
  - "0000:04:10.4"
  - "0000:04:10.6"
  vfPCIDevices:
  - harvester-659jw-000004100
  - harvester-659jw-000004102
  - harvester-659jw-000004104
  - harvester-659jw-000004106
 ```

Once configured at the next scheduled reconcile of pcidevices, the new virtual functions will be detected and created as pcidevice crd's

```shell
harvester-659jw-000004100   0000:04:10.0   8086        10ed        harvester-659jw   Ethernet controller: Intel Corporation 82599 Ethernet Controller Virtual Function                                                           ixgbevf
harvester-659jw-000004102   0000:04:10.2   8086        10ed        harvester-659jw   Ethernet controller: Intel Corporation 82599 Ethernet Controller Virtual Function                                                           ixgbevf
harvester-659jw-000004104   0000:04:10.4   8086        10ed        harvester-659jw   Ethernet controller: Intel Corporation 82599 Ethernet Controller Virtual Function                                                           ixgbevf
harvester-659jw-000004106   0000:04:10.6   8086        10ed        harvester-659jw   Ethernet controller: Intel Corporation 82599 Ethernet Controller Virtual Function                                                           ixgbevf
```

These new virtual function pcidevices can now be passed through to workload VM's running on the cluster.

Setting the numVFS back to 0 will result in virtual functions being disabled from the corresponding physical devices.

The associated pcidevice CRD objects will be deleted at the next scheduled reconcile.

### Test plan

Integration test plan.

### Upgrade strategy
Currently, the upgrade of the addon is tied to the new releases of Harvester, and this feature enhancement is scheduled for Harvester v1.2.0.

## Note [optional]
UI changes are needed to provide a better UX.
- adding a new SR-IOV management page
- users can filter pci-devices based on the SR-IOV label or addresses
