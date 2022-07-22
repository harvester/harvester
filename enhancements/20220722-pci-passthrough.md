# PCI Passthrough

PCI Passthrough allows Virtual Machines to have direct access to a PCI device, without having to copy data through the hypervisor.

## Summary

Users who want to use [SR-IOV](20220614-single-root-io-virtualization.md) in a virtualized "guest" cluster on Harvester must first enable PCI Passthrough, which we can call PCI-PT for short.

### Related Issues

The URL For the related enhancement issues in Harvester repo.

## Motivation

### Goals

- Enable [SR-IOV in Guest cluster mode](20220614-single-root-io-virtualization.md)
- Enable [GPU support](20220722-gpu-support.md)

## Proposal

### User Stories

#### Story 1
Alice wants to run TensorFlow on her cluster. 
Each node has an NVIDIA GeForce GTX 3080 and 
she wants to train a large language model faster
than CPU mode will allow.

By enabling GPU support during Harvester installation, 
the requisite NVIDIA drivers have been installed. Now 
she has to enable 
[PCI Passthrough](20220722-pci-passthrough.md) 
so that the containers can talk to the GPU.

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
Alice needs to have installed Harvester and checked the optional GPU drivers 
box. This is a gocui UI element that appears in the 
[harvester-installer](https://github.com/harvester/harvester-installer).

After installing Harvester, it reboots. Since GPU drivers were installed, Harvester 
also enabled the `iommu=on` kernel parameter. This protects the host memory 
even though the guest has direct access to that memory.

She enables PCI passthrough by setting the `pci-pt-enable` setting to `true`.

Shen then enables the GPU by setting the `gpu-enable` setting to `true`.

After that setting is updated, the settings controller then checks if the device is an 
NVIDIA or an AMD GPU (run `nvidia-smi` binary and check the process exit status).

Based on the return status, if it's 0 then it installs the Nvidia DevicePlugin. Otherwise 
it installs the AMD DevicePlugin. Now checking the node with 
`kubectl get node mynode -o yaml | yq .status.allocatable` , the GPU should appear 
in the output, this is a sign that the GPU can be allocated to a pod.

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
