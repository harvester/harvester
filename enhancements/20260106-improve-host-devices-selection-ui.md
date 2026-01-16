# Improve host devices selection UX

## Summary

When detaching the PCI/USB/vGPU devices from the VM, it only removes `spec.template.spec.domain.devices.hostDevices` from the `virtualmachine` CR on the frontend, not on the backend. So, it will throw an error after you detach and disable the PCI/USB/vGPU devices. You can refer to this demo:

https://github.com/user-attachments/assets/f7307de9-1f16-4b89-8314-67cd0696c3da

### Related Issues

- https://github.com/harvester/harvester/issues/6958
- https://github.com/harvester/harvester/issues/6892#issuecomment-2458803798

## Motivation

### Goals

Make the UX better to understand and use.

### Non-goals [optional]

None

## Proposal

In general, removing a PCI device from a VM and disabling the PCI device should be split into two steps instead of one step. The most straightforward way is to prevent users from disabling host devices during VM editing. Disabling host devices is only allowed on the appropriate page like the PCI Device, USB Device, and vGPU Device tabs.

### User Stories

If users want to detach the host device from the VM, they can just detach it like before.

If users want to detach and disable a host device from the VM together, they should follow these steps:

1. Detach the host device from the VM.
2. Disable it on the appropriate tab. If you want to disable a USB device, you need to do that on the USB Device tab.

### API changes

None.

## Design

### Implementation Overview

Forbid the disabling host device button on the VM creation page.

### Test plan

None.

### Upgrade strategy

It's only a UX change.

## Note [optional]
