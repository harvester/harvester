# Sidecar support

Enable Kubvirt sidecar support to allow fine tuning of the Kubevirt generated domain definitions

## Summary

This enhancement aims to provide a generic way to tweak Kubevirt generated domain definitions to allow fine tuning of limited set of settings.

### Related Issues

https://github.com/harvester/harvester/issues/5619

## Motivation

### Goals

To provide a generic way to leverage Harvester built and 3rd party sidecars with Harvester

### Non-goals [optional]

* Instructions for building sidecars
* Distribution mechanism for 3rd party sidecars

## Proposal

We will introduce the following changes
* enable the sidecar feature gate
* setting to allow cluster administrators to whitelist sidecars for usage within the cluster

### User Stories

#### User wants to tweak kubevirt generated domain definition

Users attempting to passthrough GPUs/vGPUs with more than 24G of memory to VM's booted in EFI mode, run into BAR resizing issues. 
The default mmio aperture on in the firmware is not large enough to map the GPU/vGPU memory.
This results in the OS being unable to leverage the hardware. This can be addressed by passing additional firmware arguments to tweak the mmio aperture size in the guest.

Kubevirt does not directly expose this setting, and the only possible solution is to leverage a sidecar to patch the VM domain definition to add additional arguments.

### User Experience In Detail
Users will be expected to edit the VM definition as a yaml and apply the sidecar specfic annotations to their VM.

For example the following annotations are needed to inject a sidecar and any additional arguments that the sidecar looks up on boot
```
    hooks.kubevirt.io/hookSidecars: '[{"args": ["--version", "v1alpha2"], "image":
      "registry:5000/kubevirt/example-hook-sidecar:devel"}]'
    smbios.vm.kubevirt.io/baseBoardManufacturer: Radical Edward
```
The VM will boot with the sidecar and apply the sidecar will apply the VM domain definition changes.

### API changes

#### Setting
We will have a new setting `whitelisted-sidecars`

```
apiVersion: harvesterhci.io/v1beta1
kind: Setting
metadata:
  name: whitelisted-sidecars
default: "sidecarimage1:tag,sidecarimage2:tag"
value: ""  
```

Cluster admins can define additional sidecars by passing them via the `value` field.

## Design

### Implementation Overview

The implementation will consist of the following:
* A new setting `whitelisted-sidecars` which the cluster admins can control at a cluster level which sidecar images can be used to edit the cluster

* Sidecar validating webhook in harvester. The webhook will watch the VM objects and check if the vm contains the sidecar injection annotation `hooks.kubevirt.io/hookSidecars`. 
  It will then extract the image name specified in the annotation, and validate the same against the whitelist provided by the `whitelisted-sidecars` setting.


### Test plan
* Install new build of harvester
* Edit the settign `whitelisted-sidecars` to provide value of a custom image
* Create a VM with the `hooks.kubevirt.io/hookSidecars: '[{"image":"registry:5000/kubevirt/example-hook-sidecar:devel"}]'` referring to the whitelisted sidecar
* Start VM, and check it has 3 containers in the virt-launcher pod. One of these containers should be the sidecar container using the image specificed via the `hooks.kubevirt.io/hookSidecars` annotation.

### Upgrade strategy

No chages will be needed specfically for the upgrade. The `sidecar` feature gate will automatically enabled on older clusters during the upgrade phase by the rancher managed chart.

Harvester controller on boot will automatically create the `whitelisted-sidecars` setting along with the default values.

## Note [optional]


