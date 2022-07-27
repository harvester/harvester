# PCI Passthrough

PCI Passthrough allows Virtual Machines to have direct access to a PCI device, without 
having to copy data through the hypervisor.

## Summary

Users who want to use [SR-IOV](20220614-single-root-io-virtualization.md) in a virtualized "guest" cluster 
on Harvester must first enable PCI Passthrough, which we can call PCI-PT for short.

### Related Issues

The URL For the related enhancement issues in Harvester repo.

## Motivation

### Goals

- Enable [SR-IOV in Guest cluster mode](20220614-single-root-io-virtualization.md)
- Enable [GPU support](20220722-gpu-support.md)

## Proposal

### User Stories

#### Story 1
Alice wants to run a TensorFlow machine learning application on her cluster. 

She has three nodes in the cluster, with a mix of different GPUs. 

List of nodes:
- node1
  - NVIDIA GeForce GTX 1660
  - AMD Radeon RX 6800
- node2
  - NVIDIA GeForce GTX 2080
  - NVIDIA Tesla V100
- node3 
  - NVIDIA GeForce GTX 1660
  - NVIDIA Tesla V100
  - AMD Radeon RX 6800
  
During installation, the harvester installer noticed these devices and installed the 
necessary drivers.

In order to expose these devices to the VMs, Alice goes to the Advanced menu and 
selects "PCI Passthrough", and is presented with a list of devices, and all the nodes 
they belong to:


- NVIDIA GeForce GTX 1660
  - node1
  - node3
- AMD Radeon RX 6800
  - node1
  - node3
- NVIDIA GeForce GTX 2080
  - node2
- NVIDIA Tesla V100
  - node2
  - node3
  
Alice's TensorFlow application contains some CUDA-specific code, so she only enables 
the three NVIDIA devices by checking the box next to them. By default, all the nodes 
with that device are selected. Then she presses the "Pass devices through on selected nodes"
button.

Alice goes to create a VM, and in the form to create a VM, there is a "PCI devices" list.
When she selects a PCI Device, the Node Scheduling section is automatically updated to 
"Run VM on node(s) matching scheduling rules", where the scheduling rule is "node X has 
device D". This will ensure that the VM only runs on nodes with the selected device.

The UI for selecting PCI devices will automatically narrow down to the intersection of all 
the PCI devices belonging to the nodes that contain the already-selected devices. To show this 
with an example, suppose Alice selected "NVIDIA GeForce GTX 1660", then, since only 
node1 and node3 have that device, only the devices on node1 and node3 would be included 
in the list. 

node1: $\{\text{geforce1660}, \text{radeon6800}\}$

node3: $\{\text{geforce1660}, \text{teslaV100}, \text{radeon6800}\}$

And their intersection is: 

$\{\text{geforce1660}, \text{radeon6800}\} \cap \{\text{geforce1660}, \text{teslaV100}, \text{radeon6800}\} = \{\text{geforce1660}, \text{radeon6800} \}$

So only those two devices would remain in the PCI Devices list. As soon as any are unselected, the list would expand again.

Once the VM is created, it is scheduled to run on the nodes with those devices, and those devices have already been configured 
for PCI-Passthrough.


#### Story 2
Alice wants to run a DPDK Application in her guest cluster and 
make use of 
[SR-IOV](20220614-single-root-io-virtualization.md), 
also set up in the guest cluster.

First she has to enable PCI Passthrough so that the 
[SR-IOV](20220614-single-root-io-virtualization.md)
in the guest cluster can directly use 
the underlying PF.
    

### User Experience In Detail

#### Story 1 in detail
Alice needs to have installed Harvester with the PCI devices plugged in. The installer detected
the GPUs and installed the necessary drivers.

After installing Harvester, it reboots. Since GPU drivers were installed, Harvester 
also enabled the `iommu=on` kernel parameter. This protects the host memory 
even though the guest has direct access to that memory.

She enables PCI Passthrough by visiting the Advanced &rarr; PCI Passthrough interface 
described in [story 1](#story-1). That list of devices and their nodes is stored as a json 
object in the setting `pci-passthrough-config`:

```json
{
    devices: [
        {id: 1, name: "Intel Corporation Ethernet Connection (11) I219-LM"}
    ],
    nodes: [
        {name: "node1", pciDevicesToPassThrough: [1]}
    ]
}
```

After that setting is updated, the settings controller will loop through the devices and prepare them 
for PCI Passthrough:

The preparation for passthrough consists of unbinding the driver, doing a driver override, then binding the 
device to the `vfio-pci` driver:

**Unbind the device from the driver**

```
echo 0000:02:00.3 > /sys/bus/pci/drivers/igb/unbind
```

**Driver override**
```
echo "vfio-pci" > /sys/bus/pci/devices/0000\:02\:00.3/driver_override
```

**Bind the device to the vfio-pci driver**
```

echo 0000:02:00.3 > /sys/bus/pci/drivers/vfio-pci/bind
```

Then, once all devices have been prepared for PCI Passthrough, the nodes are labeled with their 
devices so that kubevirt knows where to schedule VMs that request those devices as resources.



#### Story 2 in detail
She enables PCI passthrough by setting the `pci-pt-enable` setting to `true`. The 
settings controller checks if the iommu=on kernel paramater is set, if not, it edits 
the bootloader to add that parameter in. Then the system needs to be rebooted.

After rebooting, the Passthrough device should have driver `vfio-pci` loaded. That 
driver will enable VMs to pass through the hypervisor and directly connect to the PF.

Now Alice sets up a VM onto which she installs a Rancher cluster. She follows a tutorial 
like [this](https://docs.rke2.io/install/network_options/) and then labels the guest node 
```shell
kubectl label node $NODE-NAME feature.node.kubernetes.io/network-sriov.capable=true
```
Then the `sriov-network-operator` runs in the guest cluster, finding the node labeled `sriov.capable`.

In the guest cluster main node, Alice creates a pod that attaches to the `SriovNetwork` and runs 
her dpdk test application. The throughput is output to standard out, it shows the full 10Gbps now.

### API changes

## Design

### Implementation Overview

Overview on how the enhancement will be implemented.

### Test plan

#### Manual GPU
1. Install Harvester with GPU option enabled
2. Set `pci-pt-enable` to `true` in host cluster
3. Set `gpu-enable` to `true`
4. Create a Pod using the cuda-vector-add image:
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: cuda-vector-add
spec:
  restartPolicy: OnFailure
  containers:
    - name: cuda-vector-add
      # https://github.com/kubernetes/kubernetes/blob/v1.7.11/test/images/nvidia-cuda/Dockerfile
      image: "k8s.gcr.io/cuda-vector-add:v0.1"
      resources:
        limits:
          nvidia.com/gpu: 1 # requesting 1 GPU
```
5. Check the Pod logs for signs of failure or success


#### Manual SR-IOV in Guest mode
1. Set `pci-pt-enable` to `true` in host cluster, then reboot
2. Create VM to run Rancher, this is the guest cluster
3. Label the VM as `sriov.capable`
4. Install the sriov-network-operator into the Rancher cluster
5. Run a pod in the guest cluster that uses the maximum throughput, and measure that throughput. 
    If it is close to the hardware limit, then SR-IOV in Guest mode is working.

### Upgrade strategy

TODO
