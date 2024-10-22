# Support the Longhorn V2 Data Engine (v1.4.0, Experimental)

## Summary

The [Longhorn V2 Data Engine] has been available for preview use since Longhorn v1.5.0 and provides significantly better performance than the V1 Data Engine. Conseqently there is interest in being able to enable the V2 Data Engine in Harvester v1.4.0, which will be based on Longhorn v1.7.1. The V2 Data Engine will still be in preview status at that time (see the [Longhorn Roadmap]) so use of it in Harvester will also initially be considered a preview feature, and it will not be enabled by default.

[Longhorn V2 Data Engine]: https://longhorn.io/docs/1.6.2/v2-data-engine/
[Longhorn Roadmap]: https://github.com/longhorn/longhorn/wiki/Roadmap

### Related Issues

https://github.com/harvester/harvester/issues/5274

## Motivation

To give users who wish to enable the V2 Data Engine the ability to do so easily, via the Harvester GUI and/or installer, rather than having to make a series of potentially complicated and error-prone manual changes to the cluster.

### Goals

- Allow the V2 Data Engine to be enabled/disabled globally for the whole cluster
- Allow the V2 Data Engine to be enabled/disabled on specific nodes
- Allow disks to be allocated for use by the V2 Data Engine
- Allow volumes to be created backed by the V2 Data Engine

### Non-goals

- Allow VM images to be created backed by the V2 Data Engine - this will not be possible until the V2 Data Engine [supports backing images], which is currently scheduled for Longhorn v1.8.0
- Allow the default data disk (/var/lib/harvester/defaultdisk) to use the V2 Data Engine (we still need this to use the V1 Data Engine to store VM images)
- Allow the V2 Data Engine to be enabled during initial system installation

[supports backing images]: https://github.com/longhorn/longhorn/issues/8048

## Proposal

Currently, to use the V2 Data Engine, one has to:
- Load some kernel modules and allocate huge pages on each node
- Turn on the V2 Data Engine using the Longhorn GUI
- Add disks to each node via the Longhorn GUI or `kubectl`
- Create a new storage class
- Create volumes via `kubectl` or the Longhorn GUI, instead of using the Harvester GUI (we need RWO PVCs, as the v2 data engine does not yet support live migration, but the Harvester GUI will create RWX PVCs by default)

With this enhancement, users will be able to effect all the above via the Harvester GUI. Kernel module loading and huge page allocation will happen automatically when the V2 Engine is Enabled.

### User Stories

#### Story 1

I have an extra disk or disks on all my Harvester nodes, and I want to be able to create volumes backed by the V2 Data Engine on all those disks.  I have plenty of RAM and one CPU core spare on each node.

#### Story 2

I have an extra disk or disks on some of my Harvester nodes and want to be able to create volumes backed by the V2 Data Engine on those disks.  On nodes that do not have additional disks (or that are otherwise resource constrained) I do not wish to lose 2GiB of RAM and a CPU core to the V2 Data Engine.

### User Experience In Detail

#### 1. Enable the V2 Data Engine and Add Disks
1. If there are any nodes on which you do _not_ wish the V2 Data Engine to be enabled, go to the Harvester Hosts page, then:
    1. Click the "Edit Config" button.
    2. Select the "Labels" tab.
    3. Add a label with key "node.longhorn.io/disable-v2-data-engine" and value "true".
2. Go to the Harvester Settings page and set the "longhorn-v2-data-engine-enabled" setting to "true" to enable the V2 Data Engine.
3. Go to the Harveser Hosts page, then, for each node that has extra disks you want to use:
   1. Click the "Edit Config" button.
   2. Select the "Storage" tab.
   3. Select the disk to add by clicking the "Add Disk" button.
   4. Choose the "Longhorn V2 (CSI)" provisioner on the disk panel that appears.
   5. Click the "Save" button.

#### 2. Create a Storage Class (or Storage Classes)

1. Go to the Harveser Storage Classes page and click the "Create" button.
2. Set a Name for the storage class.
3. Provisioner must be set to "Longhorn V2 (CSI)".

#### 3. Create Volume(s) Backed by the V2 Data Engine

- Use the Harvester Volumes page, or create volumes as part of VM creation as usual.  The only differece is that you need to choose a storage class as created above that has Data Engine set to V2.

### API changes

- Add a new boolean setting "longhorn-v2-data-engine-enabled" to enable the V2 Data Engine, named .
- Add a new "spec.longhornConfig.enableV2DataEngine" property to the NodeConfig CRD to indicate whether the V2 Data Engine should be enabled on a given node.

## Design

### Implementation Overview

#### Enabling the V2 Data Engine

When the user enables the V2 Data Engine, we need to do the following:
1. On each node:
    - Load the relevant kernel modules: `modprobe vfio_pci ; modprobe uio_pci_generic ; modprobe nvme_tcp`.
    - Allocate 1024 hugepages: `echo 1024 > /sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages`
    - The above can be done at runtime (saves rebooting each node immediately), but these settings need to persist after reboot, so must be set in `/oem/99_settings.yaml`.
2. Set Longhorn's `lhs/v2-data-engine` value to `true`.

There are some kinks:

- Ideally, it would be possible to make the above changes to all nodes without requring a reboot, but hugepage allocation isn't guaranteed to succeed if the system doesn't have enough available unfragmented memory. So we need to first try to allocate hugepages on each node, and if any fail (i.e. if `cat /sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages` would print a number less than 1024), report that to the user and request the user reboot the affected node(s), so that hugepages can be allocated at boot time from `/oem/99_settings.yaml`.
- We actually don't want to always make these changes on every node, because this will allocate 1024 huge pages (i.e. it will eat 2GiB RAM), even if the V2 Data Engine is not enabled.
    - We can solve this by teaching node-manager to modify `/oem/99_settings.yaml`, depending on whether the V2 Data Engine is enabled, and whether or not a given node has the `node.longhorn.io/disable-v2-data-engine` label set to `true`.

To implement all the above, we can:
- In harvester:
    - Register a syncer for the `longhorn-v2-data-engine-enabled` setting. When the value of the setting changes, we do two things:
        1. Update the NodeConfig for each node to indicate that the V2 Data Engine should be enabled/disabled, taking into account the value of the node's `node.longhorn.io/disable-v2-data-engine` label.  This change will then be picked up by node-manager, much as is currently done for NTP config.
        2. Write the setting to Longhorn's `lhs/v2-data-engine` setting.
    - Watch for changes to node labels.  If `node.longhorn.io/disable-v2-data-engine` changes, again, update the NodeConfig for each node appropriately.
- In node-manager:
    - When a given NodeConfig changes to enable the V2 engine:
        - Update `/oem/99_settings.yaml`
        - Run `modprobe vfio_pci ; modprobe uio_pci_generic ; modprobe nvme_tcp`
        - Run `echo 1024 > /sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages`
        - When harvester sets `lhs/v2-data-engine`, Longhorn automatically checks the above and the user will see an appropriate error in the GUI on the settings screen if anything has failed.
    - If the V2 engine becomes disabled:
        - Update `/oem/99_settings.yaml`
        - Run `modprobe -r vfio_pci ; modprobe -r uio_pci_generic ; modprobe -r nvme_tcp`
        - Run `echo 0 > /sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages`

#### Adding Disks ####

This is largely covered by Vicente's [node-disk-manager refactor HEP], which proposes adding a Longhorn V2 provisioner to NDM.

Note that we will also need to teach NDM to ignore V2 volumes attached to the harvester host, much as it currently ignores V1 volumes by looking for the string "longhorn" in the block device's bus path.  For V2 volumes, we end up with a bunch of `/dev/nvme*` and `/dev/dm-*` devices.  We can ignore the latter by device path.  For nvme devices, we can skip anything with model set to "SPDK bdev Controller".  Here's an example of the udev data:

```
# cat /var/run/udev/data/b259:2
S:disk/by-id/nvme-uuid.c67ce251-57bc-4dd4-b2a4-60cccbc5f63b
S:disk/by-id/nvme-SPDK_bdev_Controller_00000000000000000000_1
S:disk/by-path/nvme-1
S:disk/by-id/nvme-SPDK_bdev_Controller_00000000000000000000
I:1713513798
E:MPATH_SBIN_PATH=/sbin
E:DM_MULTIPATH_DEVICE_PATH=0
E:ID_SERIAL_SHORT=00000000000000000000
E:ID_WWN=uuid.c67ce251-57bc-4dd4-b2a4-60cccbc5f63b
E:ID_MODEL=SPDK bdev Controller
E:ID_REVISION=23.05
E:ID_NSID=1
E:ID_SERIAL=SPDK_bdev_Controller_00000000000000000000_1
E:ID_PATH=nvme-1
E:ID_PATH_TAG=nvme-1
E:COMPAT_SYMLINK_GENERATION=2
E:ID_FS_TYPE=
G:systemd
Q:systemd
V:1
```

[node-disk-manager refactor HEP]: https://github.com/harvester/harvester/pull/6015

#### Creating Storage Classes

We need to make the GUI automatically set the "dataEngine" parameter appropriately for Longhorn V1 and V2 Storage Classes.  For V2 volumes Migratable and Volume Encryption must be automatically set to "No", and those fields disabled so the user can't change them (the V2 data engine does not yet support live migration or volume encryption).

#### Creating Volumes #####

The Harvester GUI needs to detect when a Longhorn V2 storage class is selected during volume creation, and use this as a trigger for setting `accessMode: ReadWriteOnce` on the PVC that will be created (again, this is because the V2 data engine does not yet support live migration).

### Test plan

- Deploy a Harvester cluster with at least 2 nodes, ensuring that there's at least one extra disk on each of those nodes. The extra disks must have WWNs in order for node-disk-manager to see them and add them for V2 usage.
- Go to Harvester's Advanced: Settings screen and set `longhorn-v2-data-engine-enabled` to `true`.
    - You will likely see a "not enough hugepages-2Mi capacity" for some nodes. If there's enough free memory this error will clear automatically after a little while. If there's not enough free memory, reboot the affected node. You can verify by sshing to the node and running `cat /sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages`. If this prints `1024`, everything is fine. If the value is lower, you'll need to reboot for the setting to take effect.
    - Once the V2 engine is successfully enabled, verify that there are two longhorn instance-managers running per node. One is for the V1 data engine, and one is for the V2 data engine. Try `kubectl -n longhorn-system get pods -o wide|grep instance` or a similar command.
    - Verify the `/oem/99_settings.yaml` file contains the relevant modprobe and sysctl settings after the usual NTP settings:
    ```
    name: oem_settings
    stages:
        initramfs:
            - name: ntp
                systemctl:
                enable:
                    - systemd-timesyncd
                    - systemd-time-wait-sync
                timesyncd:
                NTP: 0.suse.pool.ntp.org 1.suse.pool.ntp.org
            - commands:
                - modprobe vfio_pci
                - modprobe uio_pci_generic
                - modprobe nvme_tcp
                name: Runtime SPDK Prerequisites
                sysctl:
                vm.nr_hugepages: "1024"
    ```
- Verify that the V2 data engine can be disabled on a per-node basis:
    - Go to the Hosts screen and edit one host to add the label `node.longhorn.io/disable-v2-data-engine` with value `true`.
    - Verify that there is now only one longhorn instance-manager running on that node.
    - Verify that the above modprobe and sysctl settings are removed from `/oem/99_settings.yaml` on the affected node.
    - Remove the `node.longhorn.io/disable-v2-data-engine` label from the node, and verify that the second longhorn instance-manager starts up again, and the modprobe and sysctl settings reappear in `/oem/99_settings.yaml`
- Go to the Hosts screen and edit each node to add a disk for use by LHv2. Set the Provisioner to "Longhorn V2"
    - You will likely see an admission webhook error. This is a known issue and can be ignored (see https://github.com/harvester/harvester/issues/6709)
    - Use the Longhorn GUI to verify that the disk is successfully added to the node:
    - Go to the Node screen and expand each node. The LHv2 disks should show as "block" as opposed to "filesystem" (the latter is for V1).
- Go to Advanced: Storage Classes and create a Longhorn V2 storage class, by setting Provisioner to "Longhorn V2 (CSI)". The "Migratable" and "Volume Encryption" radio buttons should be automatically set to "No".
- Use the Harvester Volumes screen, and/or create volumes as part of VM creation as usual, but using the Longhorn V2 storage class.
- Ensure VMs start up with LHv2 volumes attached successfully.
- Verify backup and restore works for a VM with a LHv2 volume attached.


### Upgrade strategy

Upgrading an older version of Harvester to include this enhancement shouldn't have any special requirements.

Upgrading from this to a future version of Longhorn which supports live migration raises the question of how to handle existing V2 volumes.  Will they be somehow able to have live migration enabled automatically?  Or is this a case of needing to create a new migratable V2 storage class, then manually taking snapshots of the existing volumes and restoring those snapshots to the new migratable storage class?
