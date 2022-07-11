# SR-IOV Configuration

SR-IOV stands for Single-Root I/O Virtualization. It provides a way for virtual machines to directly access and 
even share physical network interfaces. SR-IOV drivers create multiple VFs (Virtual Functions). 

![diagram explaining SR-IOV](20220614-sriov-configuration/VFs.png)

VFs are exposed by the kernel as a named NIC. For example, `eth1` is a NIC, and there are 4 VFs, 
so the OS and the VMs both see `eth1v1`, `eth1v2`, `eth1v3` and `eth1v4`. 
The PF (Physical Function) is represented by `eth1`, and the VFs are the four `eth1vN` interfaces above.

# Guest cluster mode 
The guest cluster mode is a way of using SR-IOV in the guest cluster. The guest cluster is a cluster running as VMs
on your harvester cluster. In order to use SR-IOV in the guest cluster, you must enable PCI Passthrough. See 
technical details section below for more on what PCI Passthrough is, and how it is enabled in harvester.

## User experience
The harvester cluster admin starts by enabling the IOMMU extensions in their BIOS.

Then the cluster admin installs harvester on their nodes. This cluster is called the host cluster.

After setting up the harvester cluster, the admin creates VMs. The VMs are then arranged into 
a cluster, this is called the guest cluster.

To prepare a host node for PCI-passthrough, the admin sets the `pci-passthrough.enabled` 
setting on harvester. Harvester will automatically set the appropriate kernel parameters and bind the 
PCI device to the [vfio-pci driver](https://kubevirt.io/user-guide/virtual_machines/host-devices).

Then the admin must reboot the harvester nodes for the kernel parameters to take effect.

Next the admin needs to create a VM that allow-lists the [vfio mediated devices](https://www.kernel.org/doc/html/latest/driver-api/vfio-mediated-device.html)

Example:
```yaml
configuration:
  permittedHostDevices:
    pciHostDevices:
    - pciVendorSelector: "10DE:1EB8"
      resourceName: "nvidia.com/TU104GL_Tesla_T4"
      externalResourceProvider: true
    - pciVendorSelector: "8086:6F54"
      resourceName: "intel.com/qat"
    mediatedDevices:
    - mdevNameSelector: "GRID T4-1Q"
      resourceName: "nvidia.com/GRID_T4-1Q"
```

Then the KubeVirt VM in your harvester cluster will have direct access to the underlying PCI device, and it will be possible to enable SR-IOV in the guest cluster.

TODO: detailed explanation of how to do this


## Technical details
Steps to enable PCI Passthrough mode, which gives direct access to your PCI device, 
avoiding the hypervisor translation layer.

1. Enable IOMMU extensions in your BIOS
2. Enable the iommu and iommu.passthrough kernel parameters 
3. Unbind the driver from the PCI device 
4. Bind the `vfio-pci` driver to the PCI device
5. Modify the KubeVirt CR to include the permitted mdevs (mediated devices)

### 

# Host cluster mode
    
## User Experience

The Harvester cluster admin starts by installing Harvester. By default, the SR-IOV feature is disabled.

If the user wants to enable the feature, they need to set the `sriov-networking-enabled` setting to `true`

Admin wants to create an SR-IOV Network. To do so, they navigate to the Harvester Dashboard UI. 
Admin expands the Advanced menu on the left side and looks right below "Networks", to see the "SR-IOV"
tab. This will show the form to create the SriovNetworkNodePolicy and the SriovNetwork.

After the node policy is created, the networks' `spec.resourceName` is set to the policy's `spec.resource`.

At this point, admin can attach VMs to the SriovNetwork and begin taking advantage of the direct-memory access I/O provided by their NIC.

## Requirements
1. SR-IOV nav item (under Advanced) is only visible when `sriov-networking-enabled` setting to `true`
2. Clicking on the SR-IOV nav item will show a form to edit the fields for these two CRDs:
   2a. [SriovNetwork](https://docs.openshift.com/container-platform/4.6/networking/hardware_networks/configuring-sriov-net-attach.html) and
   2b. [SriovNetworkNodePolicy](https://docs.openshift.com/container-platform/4.6/networking/hardware_networks/configuring-sriov-device.html)
3. When the SriovNetworkNodePolicy is created, the spec.resourceName should be the default value for the SriovNetwork's spec.resourceName
4. the VLAN number iun the SriovNetwork must be unique from all the other VLAN numbers currently
