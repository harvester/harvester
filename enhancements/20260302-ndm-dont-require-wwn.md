# Rework BlockDevice naming to avoid requiring WWNs

## Summary

Harvester's node-disk-manager (NDM) component creates BlockDevice CRs which map to disks each host in the cluster. This allows Harvester to provision disks for use by Longhorn or LVM. The current naming for BD CRs is deterministic, based primarily on the WWN of the disk, or if a WWN is not present, falling back to a possible filesystem UUID. This means that Harvester can't really work with disks that don't have WWNs, as happens with some RAID controllers. It also results in problems if a disk's WWN changes - this shouldn't ever happen, but we've seen at least one case where WWNs _did_ change for a certain make and model of NVMe due to a kernel update. We need to rework how NDM identifies and tracks disks and BD CRs so that it doesn't require that devices have WWNs in order to operate, and so that it will gracefully handle the case where WWNs might change.

### Related Issues

- https://github.com/harvester/harvester/issues/6261
- https://github.com/harvester/harvester/issues/8295

## Motivation

### Goals

- Enable NDM to work with disks that don't have WWNs
- Ensure NDM doesn't get confused when an existing disk's WWN is changed
- Existing deployments (which will still be using the original naming scheme) must continue to work correctly after upgrading to the new version of NDM


## Proposal

- Decouple BlockDevice CR name generation from any actual property of the underying disk.
- When scanning disks, identify and tie the disks back to BD CRs based on, in order of preference:
  - What's actually on the disk, i.e. UUID (of filesystem or LVM volume) if present
  - WWN if present
  - Vendor+Model+Serial+BusPath if neither of the above are present

### User Stories

This proposal is internal to NDM so there aren't really any user stories aside from the fact that once implemented the user won't have to apply [workarounds](https://harvesterhci.io/kb/handle_disks_without_wwns) in order to use disks that don't have WWNs.

## Design

### Implementation Overview

Update NDM's scanner so that it operates as follows:

- Scan all disks in the system.
  - For each disk:
    - If it has a UUID:
        - Check the list of existing BDs for that UUID, if there's a match we've found an existing device.
    - If it has a WWN and hasn't already been found by UUID:
        - Check the list of existing BDs for that WWN, if there's a match we've found an existing device.
    - If the device doesn't match the above, check for an existing device by Vendor+Model+Serial+BusPath.  If this matches, we've found an existing device.
  - If we've found an existing device by one of the above three methods:
    - Check its status against the status of the disk we found, and update it to match if necessary.  This will pick up changed WWNs and re-activate provisioned devices that have gone away and come back.
  - If we haven't found any existing BD, create a new BD CR for this disk.  The name of the new CR is just a randomly generated UUID.
- Once the scan of all disks is complete, for each existing BD that _wasn't_ found:
  - If the BD was provisioned, mark it inactive (it might have been removed and be coming back later, or it's a multipath device and multipathd isn't running for whatever reason - it will be marked active automatically when it comes back online)
  - If the BD wasn't provisioned, delete it

Note some implications of the above, which are different to how NDM operated before:

- BD naming is random and has no connection to any property of the disk.
- BDs that haven't been provisioned will be removed from the list of BDs if the device is removed from the system.  If you add the same disk back later it will get a new random name.
- BDs that _have_ been provisioned will remain in the list of BDs - they'll just become inactive if the device is removed (this is the same as the previous behaviour for provisioned BDs).

### Test plan

TBD

### Upgrade strategy

The new scanning mechanism means that upgrades "just work".  Because we've decoupled the BD name from any property of the disk, the new scanner will work just fine with existing BD CRs that use the old naming scheme.

## Note

Recall that the scanner gives preference to "what's actually on the disk", which for a provisioned LHv1 volume or LVM volume is a UUID.  I had hoped to be able to detect LHv2 provisioned volumes in a similar way, but I've hit a wall in that LHv2 volumes don't seem to have any sort of ID that's easy to extract without starting up SPDK somehow and interrogating the device.

There _is_ data we could interrogate, notably the signature `SPDKBLOB` at the start of the device, and, later a name which actually _does_ match a BD CR (in the example below it's `2e303caa-89ab-404a-902b-85fccdf2a189`), but we'd need to write some tool that knows how to walk SPDK blobs to extract that data.  That's still not entirely relabile though because (for NVMe devices at least), once LH has activated them and SPDK has taken over, the device node (/(dev/nvme0xxx) disappears and we can't read that disk anymore.

TL;DR: right now, LHv2 volumes will be checked by WWN, and if there is no WWN it will fall back to Vendor+Model+Serial+BusPath.  It's not exactly what I wanted but it'll still work.

```
# dd if=/dev/nvme0n1 of=/dev/stdout bs=1024 count=100 2>/dev/null| hexdump -C
00000000  53 50 44 4b 42 4c 4f 42  03 00 00 00 00 10 00 00  |SPDKBLOB........|
00000010  00 00 00 00 00 00 00 00  01 00 00 00 00 00 10 00  |................|
00000020  01 00 00 00 04 00 00 00  05 00 00 00 04 00 00 00  |................|
00000030  0d 00 00 00 00 90 01 00  4c 56 4f 4c 53 54 4f 52  |........LVOLSTOR|
00000040  45 00 00 00 00 00 00 00  09 00 00 00 04 00 00 00  |E...............|
00000050  00 00 00 00 19 00 00 00  00 02 00 00 00 10 00 00  |................|
00000060  00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00  |................|
*
00000ff0  00 00 00 00 00 00 00 00  00 00 00 00 a0 e3 2f f4  |............../.|
00001000  00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00  |................|
*
0000d000  00 00 00 00 01 00 00 00  00 00 00 00 00 00 00 00  |................|
0000d010  03 18 00 00 00 04 00 00  00 00 00 00 00 00 00 00  |................|
0000d020  00 00 00 00 00 00 00 00  00 00 00 00 00 02 2d 00  |..............-.|
0000d030  00 00 04 00 25 00 75 75  69 64 37 65 36 66 66 66  |....%.uuid7e6fff|
0000d040  32 30 2d 32 31 62 36 2d  34 66 39 39 2d 62 36 30  |20-21b6-4f99-b60|
0000d050  38 2d 64 31 39 30 65 39  30 33 37 65 33 32 00 02  |8-d190e9037e32..|
0000d060  2f 00 00 00 04 00 27 00  6e 61 6d 65 32 65 33 30  |/.....'.name2e30|
0000d070  33 63 61 61 2d 38 39 61  62 2d 34 30 34 61 2d 39  |3caa-89ab-404a-9|
0000d080  30 32 62 2d 38 35 66 63  63 64 66 32 61 31 38 39  |02b-85fccdf2a189|
0000d090  6e 31 00 05 08 00 00 00  00 00 00 00 00 00 00 00  |n1..............|
0000d0a0  00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00  |................|
*
0000dff0  00 00 00 00 00 00 00 00  ff ff ff ff e6 4e ab a5  |.............N..|
0000e000  00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00  |................|
*
00019000
```
