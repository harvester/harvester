# Support Multipath Disk

Support for multipath disk devices in Harvester to enable reliable storage access through multiple paths for improved fault tolerance and performance.

## Summary

This enhancement introduces multipath device recognition and management in Harvester's node-disk-manager component, allowing users to configure and utilize multipath storage devices safely while ensuring proper identification and avoiding conflicts with existing Longhorn v2 and LVM DM-devices.

### Related Issues

- Primary Issue: https://github.com/harvester/harvester/issues/6975
- Non-WWN Device Handling Issue: https://github.com/harvester/harvester/issues/6261 (out of scope for this enhancement)
- Support filter list on NDM: https://github.com/harvester/harvester/issues/5059 (out of scope for this enhancement)

## Motivation

### Goals

- Enable Harvester to recognize and manage multipath disk devices properly
- Support multipath device configuration in Harvester installer with extended configuration options
- Distinguish between multipath devices and deliberate vendor devices (ex: Longhorn v2 DM-devices) to avoid conflicts
- Ensure multipath devices are back after starting multipathd service when multipathd service is disabled.

### Non-goals

- Implementing multipath configuration without DM_WWN support (this will be addressed in a separate issue #6261)
- Guaranteeing optimal multipathd performance

## Proposal

This enhancement adds multipath device support to Harvester by implementing device identification capabilities in the node-disk-manager component and extending the installer configuration options.

### User Stories

#### Story 1: Be able to use external storage like SAN, iSCSI, or Fibre Channel in NDM

Those disks expose different accessible paths to the node. However, even we aggregate those paths in to a single device with multipath service, NDM doesn't still support recognizing them. Right now, it's only able to be configured in Longhorn. That's why we need to support configuaring in Harvester.

After NDM is able to identify those aggregated disks, users can directly select those disk in Harvester instead of crating the settings in Longhorn.

#### Story 2: High availability and fault tolerance

Without multipath support, users cannot configure Harvester with multipath storage devices. This limitation results in single points of failure in storage connectivity and prevents effective use of enterprise SAN storage.

With multipath, storage paths can be aggregated into a single logical device, enabling high availability and fault tolerance. In this setup, users can configure multipath devices during Harvester installation, and the system will automatically fail over to alternative paths if one becomes unavailable.

### User Experience In Detail

Multipath devices appear in the Harvester UI as available storage devices and can be used for Longhorn storage configuration.

### API changes

No direct API changes to existing Harvester APIs. The enhancement extends the installer configuration schema to accept additional fields for multipath configuration.

## Design

### Implementation Overview

The multipath support is implemented through modifications to Harvester's node-disk-manager component:

1. **Enhanced Device Recognition:** Added support for multipath device identification by extending device property extraction to handle `DM_SERIAL` and `DM_WWN` attributes specific to multipath devices. For example, we'll retrieve WWN value in this order from `udevadm info`: `ID_WWN_WITH_EXTENSION` -> `ID_WWN` -> `DM_WWN` and SERIAL value in this order: `ID_SERIAL_SHORT` -> `ID_SERIAL` -> `DM_SERIAL`

2. **Device Filtering:** Implemented logic to distinguish multipath devices from other dm-devices (such as Longhorn v2 volumes) using `multipath -C dm-x` command-line for verification. It will return an exit code of 0 if the dm-x device belongs to multipath service; otherwise, it will return 1. Besides, we also need to check whether /dev/xxx is managed by multipath service, using `multipath -c /dev/xxx`. It will also return an exit code of 0 if the /dev/xxx is managed by multipath service; otherwise, it will return 1. Besides, we should have a way to filter out other multipath devices that are used in other CSI drivers. We have this issue https://github.com/harvester/harvester/issues/5059 to filter out block device. We can make use of that mechanism to filter out multipath devices.

3. **Improved Mount Handling:** Enhanced mount point detection to properly handle `/dev/mapper/xx` symlinks used by multipath devices. Because there isn't a `/dev/dm-x` mounting in `/proc/mounts`, there is only `/dev/mapper/xx` ([Ref](#Multipath-Device-Example)). Hence, we can only use `/dev/mapper/xx` to check.

4. **Installer Configuration Extension:** Extended Harvester installer to accept additional multipath configuration parameters including WWID-based identification or `blacklist_exception` for better usage. For example, below configuraiton is used for testing. We can use `wwid "!^0QEMU_QEMU_HARDDISK_disk[0-9]+"` in `blacklist` section or `wwid "^0QEMU_QEMU_HARDDISK_disk[0-9]+"` in `blacklist_exceptions`. Both are okay. However, I think it's more flexible to provide the different configuration for users.
    ```
    blacklist {
        device {
            vendor "!QEMU"
            product "!QEMU HARDDISK"
        }
        wwid ".*"
    }
    blacklist_exceptions {
        wwid "^0QEMU_QEMU_HARDDISK_disk[0-9]+"
    }
    ```

### Test plan

We should prepare two different clusters to test:

1. Cluster with multipath service enabled by default.
2. Cluster with multipath service disabled by default.

For the second cluster, after the cluster is up, we need to run `systemctl enable multipathd` and `systemctl start multipathd`. After that, continue with the following test steps.

Additionally, you need to prepare two LUNs pointing to the same disk. If you're not sure about the setup, it should look like this in the `virt-install` command line:

```
# 1. Same serial numbers, and same physical disk
# 2. Same serial numbers, same WWN, and same physical disk
--disk path=/var/lib/libvirt/images/mydisk1.img,bus=scsi,cache=none,shareable=on,serial=disk1 \
--disk path=/var/lib/libvirt/images/mydisk1.img,bus=scsi,cache=none,shareable=on,serial=disk1 \
```

Then, start testing:

1. In the settings, enable Longhorn V2 engine
2. Run `multipath -l` to verify that the multipath service is working properly
3. Add the specific multipath disk (for Longhorn V1) and another regular disk (for Longhorn V2) to Harvester through the GUI
4. Create two VM images using Longhorn V1 and V2 engines respectively
5. Start two virtual machines with the images and additional disks:
    - Longhorn V1 Image with Longhorn V1 extra disk
    - Longhorn V2 Image with Longhorn V2 extra disk
    - LVM Voluems
6. After starting the VMs, reboot the node. Verify that the virtual machines come back up successfully after the reboot

### Upgrade strategy

Some users might have already enabled multipathd and are using it. In such cases, the disks should be configured and added in the Longhorn UI by the users themselves. 

If those disks are available in the Harvester UI, adding them in the Harvester UI will cause failure. Therefore, NDM or GUI should filter out disks that are already mounted and added in the Longhorn UI.

By the way, if you add a normal disk and configure it in the Longhorn UI, it's still available in the Harvester UI. Adding it also causes failure. So, the behavior has been like this for a long time. The suitable way to migrate it from Longhorn to Harvester is to delete replica and backing images before adding the disks on the Harvester side.

For this part, it's tracked in https://github.com/harvester/harvester/issues/8990. We just need to focus on supporting multipath devices in this HEP.

## Note

None.

## Appendix

### Multipath Device Example

```
harvester1:/home/rancher # mount | grep dm-
harvester1:/home/rancher # mount | grep mapper
/dev/mapper/0QEMU_QEMU_HARDDISK_disk1 on /var/lib/harvester/extra-disks/74d8b8841cd87b60536d690a84c9a535 type ext4 (rw,relatime,errors=remount-ro)

harvester1:/home/rancher # multipath -l
0QEMU_QEMU_HARDDISK_disk1 dm-0 QEMU,QEMU HARDDISK
size=20G features='0' hwhandler='0' wp=rw
|-+- policy='service-time 0' prio=0 status=active
| `- 6:0:0:1 sdd 8:48 active undef running
`-+- policy='service-time 0' prio=0 status=enabled
  `- 6:0:0:0 sda 8:0  active undef running

harvester1:/home/rancher # multipath -C dm-0
74557.646276 | 0QEMU_QEMU_HARDDISK_disk1: adding new path sdd
74557.647273 | 0QEMU_QEMU_HARDDISK_disk1: adding new path sda

harvester1:/home/rancher # udevadm info /dev/sda
P: /devices/pci0000:00/0000:00:02.2/0000:03:00.0/virtio2/host6/target6:0:0/6:0:0:0/block/sda
N: sda
L: 0
S: disk/by-id/scsi-0QEMU_QEMU_HARDDISK_disk1
S: disk/by-path/pci-0000:03:00.0-scsi-0:0:0:0
S: disk/by-id/scsi-SQEMU_QEMU_HARDDISK_disk1
S: disk/by-uuid/d9f4d116-df7c-484f-9e7c-bdcaee3242a1
E: DEVPATH=/devices/pci0000:00/0000:00:02.2/0000:03:00.0/virtio2/host6/target6:0:0/6:0:0:0/block/sda
E: DEVNAME=/dev/sda
E: DEVTYPE=disk
E: DISKSEQ=2
E: MAJOR=8
E: MINOR=0
E: SUBSYSTEM=block
E: USEC_INITIALIZED=7457656
E: DONT_DEL_PART_NODES=1
E: ID_PATH=pci-0000:03:00.0-scsi-0:0:0:0
E: ID_PATH_TAG=pci-0000_03_00_0-scsi-0_0_0_0
E: SCSI_TPGS=0
E: SCSI_TYPE=disk
E: SCSI_VENDOR=QEMU
E: SCSI_VENDOR_ENC=QEMU\x20\x20\x20\x20
E: SCSI_MODEL=QEMU_HARDDISK
E: SCSI_MODEL_ENC=QEMU\x20HARDDISK\x20\x20\x20
E: SCSI_REVISION=2.5+
E: ID_SCSI=1
E: ID_SCSI_INQUIRY=1
E: SCSI_IDENT_SERIAL=disk1
E: SCSI_IDENT_LUN_VENDOR=disk1
E: ID_VENDOR=QEMU
E: ID_VENDOR_ENC=QEMU\x20\x20\x20\x20
E: ID_MODEL=QEMU_HARDDISK
E: ID_MODEL_ENC=QEMU\x20HARDDISK\x20\x20\x20
E: ID_REVISION=2.5+
E: ID_TYPE=disk
E: ID_BUS=scsi
E: ID_SERIAL=0QEMU_QEMU_HARDDISK_disk1
E: ID_SERIAL_SHORT=disk1
E: ID_SCSI_SERIAL=disk1
E: multipath=on
E: MPATH_SBIN_PATH=/sbin
E: DM_MULTIPATH_DEVICE_PATH=1
E: ID_FS_TYPE=ext4
E: SYSTEMD_READY=0
E: ID_FS_UUID=d9f4d116-df7c-484f-9e7c-bdcaee3242a1
E: ID_FS_UUID_ENC=d9f4d116-df7c-484f-9e7c-bdcaee3242a1
E: ID_FS_VERSION=1.0
E: ID_FS_USAGE=filesystem
E: COMPAT_SYMLINK_GENERATION=2
E: NVME_HOST_IFACE=none
E: DEVLINKS=/dev/disk/by-id/scsi-0QEMU_QEMU_HARDDISK_disk1 /dev/disk/by-path/pci-0000:03:00.0-scsi-0:0:0:0 /dev/disk/by-id/scsi-SQEMU_QEMU_HARDDISK_disk1 /dev/disk/by-uuid/d9f4d116-df7c-484f-9e7c-bdcaee3242a1
E: TAGS=:systemd:
E: CURRENT_TAGS=:systemd:

harvester1:/home/rancher # udevadm info /dev/sdd
P: /devices/pci0000:00/0000:00:02.2/0000:03:00.0/virtio2/host6/target6:0:0/6:0:0:1/block/sdd
N: sdd
L: 0
S: disk/by-path/pci-0000:03:00.0-scsi-0:0:0:1
S: disk/by-uuid/d9f4d116-df7c-484f-9e7c-bdcaee3242a1
S: disk/by-id/scsi-SQEMU_QEMU_HARDDISK_disk1
S: disk/by-id/scsi-0QEMU_QEMU_HARDDISK_disk1
E: DEVPATH=/devices/pci0000:00/0000:00:02.2/0000:03:00.0/virtio2/host6/target6:0:0/6:0:0:1/block/sdd
E: DEVNAME=/dev/sdd
E: DEVTYPE=disk
E: DISKSEQ=5
E: MAJOR=8
E: MINOR=48
E: SUBSYSTEM=block
E: USEC_INITIALIZED=7482490
E: DONT_DEL_PART_NODES=1
E: ID_PATH=pci-0000:03:00.0-scsi-0:0:0:1
E: ID_PATH_TAG=pci-0000_03_00_0-scsi-0_0_0_1
E: SCSI_TPGS=0
E: SCSI_TYPE=disk
E: SCSI_VENDOR=QEMU
E: SCSI_VENDOR_ENC=QEMU\x20\x20\x20\x20
E: SCSI_MODEL=QEMU_HARDDISK
E: SCSI_MODEL_ENC=QEMU\x20HARDDISK\x20\x20\x20
E: SCSI_REVISION=2.5+
E: ID_SCSI=1
E: ID_SCSI_INQUIRY=1
E: SCSI_IDENT_SERIAL=disk1
E: SCSI_IDENT_LUN_VENDOR=disk1
E: ID_VENDOR=QEMU
E: ID_VENDOR_ENC=QEMU\x20\x20\x20\x20
E: ID_MODEL=QEMU_HARDDISK
E: ID_MODEL_ENC=QEMU\x20HARDDISK\x20\x20\x20
E: ID_REVISION=2.5+
E: ID_TYPE=disk
E: ID_BUS=scsi
E: ID_SERIAL=0QEMU_QEMU_HARDDISK_disk1
E: ID_SERIAL_SHORT=disk1
E: ID_SCSI_SERIAL=disk1
E: multipath=on
E: MPATH_SBIN_PATH=/sbin
E: DM_MULTIPATH_DEVICE_PATH=1
E: ID_FS_TYPE=ext4
E: SYSTEMD_READY=0
E: ID_FS_UUID=d9f4d116-df7c-484f-9e7c-bdcaee3242a1
E: ID_FS_UUID_ENC=d9f4d116-df7c-484f-9e7c-bdcaee3242a1
E: ID_FS_VERSION=1.0
E: ID_FS_USAGE=filesystem
E: COMPAT_SYMLINK_GENERATION=2
E: NVME_HOST_IFACE=none
E: DEVLINKS=/dev/disk/by-path/pci-0000:03:00.0-scsi-0:0:0:1 /dev/disk/by-uuid/d9f4d116-df7c-484f-9e7c-bdcaee3242a1 /dev/disk/by-id/scsi-SQEMU_QEMU_HARDDISK_disk1 /dev/disk/by-id/scsi-0QEMU_QEMU_HARDDISK_disk1
E: TAGS=:systemd:
E: CURRENT_TAGS=:systemd:

harvester1:/home/rancher # udevadm info /dev/dm-0
P: /devices/virtual/block/dm-0
N: dm-0
L: 50
S: disk/by-id/scsi-0QEMU_QEMU_HARDDISK_disk1
S: disk/by-id/dm-name-0QEMU_QEMU_HARDDISK_disk1
S: disk/by-uuid/d9f4d116-df7c-484f-9e7c-bdcaee3242a1
S: disk/by-id/dm-uuid-mpath-0QEMU_QEMU_HARDDISK_disk1
S: disk/by-id/wwn-0xQEMU_QEMU_HARDDISK_disk1
S: mapper/0QEMU_QEMU_HARDDISK_disk1
E: DEVPATH=/devices/virtual/block/dm-0
E: DEVNAME=/dev/dm-0
E: DEVTYPE=disk
E: DISKSEQ=6
E: MAJOR=254
E: MINOR=0
E: SUBSYSTEM=block
E: USEC_INITIALIZED=4614372
E: DM_UDEV_DISABLE_LIBRARY_FALLBACK_FLAG=1
E: DM_UDEV_PRIMARY_SOURCE_FLAG=1
E: DM_UDEV_RULES_VSN=2
E: DM_ACTIVATION=1
E: DM_NAME=0QEMU_QEMU_HARDDISK_disk1
E: DM_UUID=mpath-0QEMU_QEMU_HARDDISK_disk1
E: DM_SUSPENDED=0
E: MPATH_DEVICE_READY=1
E: DM_TYPE=scsi
E: DM_WWN=0xQEMU_QEMU_HARDDISK_disk1
E: DM_SERIAL=0QEMU_QEMU_HARDDISK_disk1
E: ID_FS_UUID=d9f4d116-df7c-484f-9e7c-bdcaee3242a1
E: ID_FS_UUID_ENC=d9f4d116-df7c-484f-9e7c-bdcaee3242a1
E: ID_FS_VERSION=1.0
E: ID_FS_TYPE=ext4
E: ID_FS_USAGE=filesystem
E: DEVLINKS=/dev/disk/by-id/scsi-0QEMU_QEMU_HARDDISK_disk1 /dev/disk/by-id/dm-name-0QEMU_QEMU_HARDDISK_disk1 /dev/disk/by-uuid/d9f4d116-df7c-484f-9e7c-bdcaee3242a1 /dev/disk/by-id/dm-uuid-mpath-0QEMU_QEMU_HARDDISK_disk1 /dev/disk/by-id/wwn-0xQEMU_QEMU_HARDDISK_disk1 /dev/mapper/0QEMU_QEMU_HARDDISK_disk1
E: TAGS=:systemd:
E: CURRENT_TAGS=:systemd:
```