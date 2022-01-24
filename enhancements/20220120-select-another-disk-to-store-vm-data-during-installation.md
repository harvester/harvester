# Select another disk to store VM data during installation

## Summary

Servers usually have multiple disks installed, and it's common to store application data in a disk
different from the one where the operating system lies. Therefore, during the installation,
we should provide the ability to let users choose which disk to store the VM data.

### Related Issues

- https://github.com/harvester/harvester/issues/1728

## Motivation

### Goals

- Allow users to specify the disk to store VM data via Harvester config.
- Allow users to choose the disk to store VM data via Installer UI.
- The VM data will be stored in the user-selected disk exclusively.
  The disk used to install the operating system would not hold any VM data.

### Non-goals [optional]

- Users won't be able to select **multiple disks** for storing VM data.
  See [Note](#supporting-select-multiple-disks-to-store-vm-data) for more information.

## Proposal

### User Stories

#### Easily configure a different disk to store VM data

Before this enhancement, users have to manually configure Longhorn and Harvester in order to store
VM data exclusively in a disk different from the one where the operating system lies.
Some of the operations even require users to access Longhorn UI.

After this enhancement,
users can **easily** select the disk to store VM data during the installation,
either via Installer UI or a config file.

### User Experience In Detail

#### Installer UI
We would list all the detected disks for users to choose from for storing VM data.

#### Harvester config
We would add a new config, e.g. `DataDisk` for users to designate the VM data disk.
The value should be a device path such as `/dev/sda` or `/dev/disk/by-id/wwn-XXXXX`.
If this config is not provided, we will use the operating system disk to store VM data by default.

### API changes

No API changes.

## Design

- One particular requirement is that Longhorn binaries should still be stored in the operating system
  disk to prevent Longhorn from failure if the data disk fails.

### Terms

- **Longhorn Binaries**:
  The binary executables stored in the Node for Longhorn to function correctly.
  They are stored in `/var/lib/longhorn/`.

- **Default Longhorn disks**:
  When a Node joins the cluster,
  Longhorn will try to set up zero, one, or multiple disks on the Node,
  depending on its setting and the Node's labels and annotations.

- **Default Data Path**:
  A setting of Longhorn,
  see [here](https://longhorn.io/docs/1.2.3/references/settings/#default-data-path)
  for more information.

- **VM disk**:
  The disk users selected for storing VM data.

### Implementation Overview

#### Fresh install
- Change Default Data Path to `/var/lib/harvester/defaultdisk`.
  It means Longhorn will use this path to set up the Default Longhorn disk other than the default
  `/var/lib/longhorn`, where the Longhorn binaries locate.
  We can modify the Longhorn chart values to deploy Longhorn with this setting.

- Make sure `/var/lib/longhorn/` is a persistent volume.
  In 1.0.0, `/var/lib/longhorn` could be persistent or not,
  depending on the config `NoDataPartition`. Now, because we will be using
  `/var/lib/harvester/defaultdisk` to store VM data, `/var/lib/longhorn` should always be persistent
  as it's where the Longhorn binaries locate.

- The installer should format the VM disk using EXT4 **partitionless**,
  with a filesystem label `HARV_LH_DEFAULT`.

- The VM disk should be mounted onto `/var/lib/harvester/defaultdisk`. In this case,
  we will use the filesystem label `HARV_LH_DEFAULT` to achieve this.

#### Upgrade

In v1.0.0, we didn't provide `default-data-path` setting so by default it's `/var/lib/longhorn/`.
Once the cluster is upgraded, we would have to change it to `/var/lib/harvester/defaultdisk`:

- It does not affect the existed Nodes, as they should already have default disks configured.
  Longhorn will not re-create default disks for these Nodes.


- While a freshly installed Node joins the cluster, Longhorn has to set up `/var/lib/harvester/defaultdisk`
  as the default disk on this Node, instead of `/var/lib/longhorn`.

Another thing is we should prohibit v1.0.0 Nodes from joining a cluster with this change.
See [Note](#version-incompatibility) for more detail.

### Test plan

1. Prepare a machine with two disks, let's say they will be `/dev/vda` and `/dev/sda`
  during the installation.

2. During the installation, select `/dev/vda` as the OS disk

3. At the step "Remote config", provide the following config to use `/dev/sda`
  as the default Longhorn disk:
    ```
    install:
      data_disk: /dev/sda
    ```

4. After the installation finished, wait for the system to reboot and verify the following:
    - `/dev/sda` is mounted to `/var/lib/harvester/defaultdisk` as the default disk.
      Create a VM and verify Longhorn is storing data into `/var/lib/harvester/defaultdisk`.
      You can create a huge file in the VM and see the space of `/dev/vda` is getting consumed.

    - `/var/lib/longhorn` still exists, and Longhorn binaries are stored here.
        ```
        $ ls -l /var/lib/longhorn/engine-binaries
        total 4
        drwxr-xr-x 2 root root 4096 Jan 19 08:14 longhornio-longhorn-engine-v1.2.3
        ```

5. Prepare another machine to join the cluster. Again you need two disks on this one.

6. Provide the same config as step 3 to use the other disk as the default Longhorn disk.

7. Perform the same verification as step 4 on this join Node.

### Upgrade strategy

No user intervention is required during the upgrade.

## Note

### Supporting select multiple disks to store VM data
Longhorn supports creating multiple default disks with Node-wise settings:

- Change the Longhorn setting "createDefaultDiskLabeledNodes" to `true`.

- Add a label to the Node: `node.longhorn.io/create-default-disk="config"`,
  indicating default disks on this Node should be created with a Node-wise config.

- Add an annotation to the Node: `node.longhorn.io/default-disks-config=<disks config in JSON>`
  to specify the Node-wise config.

Then Longhorn would follow the `<disks config in JSON>` to create default disks
[REF](https://longhorn.io/docs/1.2.3/advanced-resources/default-disk-and-node-config/#customizing-default-disks-for-new-nodes).

What matters here is we can't add annotations to the Node during installation.
It's possible to add labels, but not annotations.

Luckily, the labels and annotations could be added to the Node after it has joined the cluster.
Longhorn would follow the annotation to create default disks once it appears,
**as long as default disks have never been created on the Node.**
Therefore, we should be able to rely on NDM or other controllers to
help add the labels and annotations after it joined the cluster, but it requires extra effort.

### Version incompatibility
We should prohibit users from adding v1.0.0 Nodes into the cluster with this enhancement.
For example, if this enhancement is released in v1.0.1,
users should not add a v1.0.0 Node into a v1.0.1 cluster. The reason being:

- In v1.0.0, the default data path is `/var/lib/longhorn`.
  and this path is configured to be persistent across reboots.

- In v1.0.1, we would change the default data path to `/var/lib/harvester/defaultdisk`.
  When a fresh installed v1.0.0 node joins a v1.0.1 cluster,
  Longhorn will set up `/var/lib/harvester/defaultdisk` as the default disk on the Node,
  which does not configure as persistent volume for v1.0.0 Nodes.

> NOTE: **This only affects fresh installed Node (ISO or PXE)**.
> The upgrading process is not affected,
> as all the Nodes in an existing cluster should already have default disks configured.
