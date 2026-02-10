# Improve Host Devices Selection UI/UX

## Summary

When detaching PCI/USB/vGPU devices from a VM, the system only removes `spec.template.spec.domain.devices.hostDevices` from the `virtualmachine` CR on the frontend, not on the backend. As a result, an error is thrown when you detach and disable PCI/USB/vGPU devices in a single operation. Refer to this demo for more details:

https://github.com/user-attachments/assets/f7307de9-1f16-4b89-8314-67cd0696c3da

### Related Issues

- https://github.com/harvester/harvester/issues/6958
- https://github.com/harvester/harvester/issues/6892#issuecomment-2458803798

## Motivation

### Goals

Improve the UI/UX to make it easier to understand and use.

### Non-goals [optional]

None

## Proposal

In general, removing a PCI device from a VM and disabling the PCI device should be split into two separate steps instead of being performed as a single operation. The most straightforward approach is to prevent users from disabling host devices during VM editing.

However, we can make it easier by changing the error message to a warning message such as: "Please detach the PCI device and save the changes before disabling the PCI device."

This doesn't break the current user experience.

### User Stories

If users want to detach a host device from the VM, they can simply detach it as before. If users want to both detach and disable a host device from the VM, they should follow these steps:

1. Detach the host device from the VM.
2. Disable it on the appropriate tab. For example, to disable a USB device, navigate to the USB Device tab.

### API changes

None.

## Design

### Implementation Overview

Add a new warning message to inform users of the required steps for disabling a PCI device on the VM edit page.

### Test plan

None.

### Upgrade strategy

This is a UI/UX-only change and does not require any upgrade considerations.

## Note [optional]
