# CD-ROM Hotplug Volumes

## Summary

This enhancement proposal addresses the current limitation in Harvester where changing CD-ROM media for a Virtual Machine (VM) necessitates a VM restart. By leveraging KubeVirt's support for CD-ROM volume insertion and ejection (available since version 1.6), this HEP aims to introduce the capability to dynamically manage CD-ROM *volumes* on running VMs without interruption. This significantly improves user experience by allowing seamless swapping of ISO images or other CD-ROM content.

It is important to distinguish this feature from attaching or detaching the CD-ROM *device* itself, which still typically requires a VM restart. This proposal focuses solely on the live management of the content (volume) within an already attached CD-ROM device.

### Related Issues

https://github.com/harvester/harvester/issues/9261

## Motivation

### Goals

- Support inserting a CD-ROM volume to a running VM with an empty CD-ROM device without restarting
- Support ejecting a CD-ROM volume from a CD-ROM device of a running VM without restarting

### Non-goals

- Attach a CD-ROM device to a running VM without restarting
- Detach a CD-ROM device from a running VM without restarting

## Proposal

### User Stories

#### Story 1: Insert an image into an empty CD-ROM device

Users shall be able to insert an ISO image into an empty CD-ROM device of a running Virtual Machine. This enables dynamic loading of installation media or utilities without requiring a VM restart.

#### Story 2: Eject an image from an occupied CD-ROM device

Users shall be able to eject an ISO image from an occupied CD-ROM device of a running Virtual Machine. This allows for safe removal of media and prepares the drive for new content without interrupting the VM's operation.

#### Story 3: Replace an image in an occupied CD-ROM device

Users shall be able to replace an existing ISO image in an occupied CD-ROM device of a running Virtual Machine by first ejecting the current volume and then inserting a new one. This provides flexibility for multi-stage software installations or troubleshooting.

#### Story 4: Create a Virtual Machine with an empty CD-ROM device

Users shall be able to create a Virtual Machine with an empty CD-ROM device. This allows for insertion of additional media later.

### User Experience In Detail

#### Via UI

The following steps outline the user workflow for managing CD-ROM volumes on a running VM:

1. Navigate to the details page of a running VM.
2. On the **Volumes** tab, the user can find the list of attached CD-ROM devices.
3. Empty CD-ROM devices connected via the SATA bus will display an "Insert" action button. Clicking this button will open a dialog for the user to select an image to insert.
4. Occupied CD-ROM devices connected via the SATA bus will display an "Eject" action button. Clicking this button will eject and delete the corresponding volume.

#### Via Terraform

In the nested block `disk` of `harvester_virtualmachine` resource, user can already specified whether the disk is hotpluggable by `hot_plug`. `hot_plug` would automatically set to `true` if unspecified when the `type` is `cd-rom` and `bus` is `sata`.

to define a VM with empty CD-ROM devices, they can define a nested `disk` block without `image` field like
```
  disk {
    name       = "cdrom-disk"
    type       = "cd-rom"
    bus        = "sata"
    boot_order = 2
  }
```

to insert the image, they can simply add the `image`, `size`, and `hot_plug` fields (optional: `auto_delete`)
```diff
  disk {
    name       = "cdrom-disk"
    type       = "cd-rom"
    bus        = "sata"
    boot_order = 2

+   size        = "10Gi"
+   hot_plug    = true
+   image       = harvester_image.k3os.id
+   auto_delete = true
  }
```

to eject the image, they can remove the `image`, `size`, and `hot_plug` fields (optional: `auto_delete`)
```diff
  disk {
    name       = "cdrom-disk"
    type       = "cd-rom"
    bus        = "sata"
    boot_order = 2

-   size        = "10Gi"
-   hot_plug    = true
-   image       = harvester_image.k3os.id
-   auto_delete = true
  }
```

to replace the image, they can change the `image` and `size` fields

```diff
  disk {
    name       = "cdrom-disk"
    type       = "cd-rom"
    bus        = "sata"
    boot_order = 2

-   size       = "10Gi"
+   size       = "15Gi"
    hot_plug   = true
-   image       = harvester_image.k3os.id
+   image       = harvester_image.opensuse.id
    auto_delete = true
  }
```

### API changes

Two new actions will be added to the `VirtualMachine` resource to handle CD-ROM volume operations.

**Insert a CD-ROM volume**

This action inserts an image into a specified empty CD-ROM device.

```
POST /v1/harvester/vm/<vm>?action=insertCdRomVolume
Content-Type: application/json

{
  "deviceName": "cdrom-vol-abc",
  "imageName": "default/image-pvvwf"
}

# Response
- status code: 204
```

**Eject a CD-ROM volume**

This action ejects the volume from a specified CD-ROM device.

```
POST /v1/harvester/vm/<vm>?action=ejectCdRomVolume
Content-Type: application/json

{
  "deviceName": "cdrom-vol-abc"
}

# Response
- status code: 204
```

**Deprecation of `ejectCdRom` Action**

The existing `ejectCdRom` action ("Eject CD-ROM" on UI), which detaches the entire CD-ROM *device* and requires a VM restart, will be deprecated. The new `ejectCdRomVolume` action is preferred as it only ejects the volume without causing VM downtime. The old action will be removed.

## Design

### Implementation Overview

#### KubeVirt Feature Gate Migration

As noted in the KubeVirt documentation[^1], this feature relies on the `DeclarativeHotplugVolumes` feature gate. This will replace the older `HotplugVolumes` gate. According to the announcement[^2], this is a breaking change. Fortunately, ephemeral hotplug volumes isn't used in Harvester, making this a safe and direct replacement.

#### CD-ROM Related VM Actions

The Harvester API server will expose two new actions on `VirtualMachine` resources. These actions will modify the `VirtualMachine` resource by adding or removing the volume definition from `spec.template.spec.volumes` and updating the corresponding PVC information in the `harvesterhci.io/volumeClaimTemplates` annotation.

- **`insertCdRomVolume`**: This action will patch the target `VirtualMachine` by adding a new `persistentVolumeClaim` to the `spec.template.spec.volumes` list and a corresponding entry to the `harvesterhci.io/volumeClaimTemplates` annotation. The PVC will be marked with `hotpluggable: true`.

```diff
 apiVersion: kubevirt.io/v1
 kind: VirtualMachine
 metadata:
   annotations:
     harvesterhci.io/volumeClaimTemplates: >-
-      [{"metadata":{"name":"vm-ws-1-rootdisk-giua3"},"spec":{"accessModes":["ReadWriteMany"],"resources":{"requests":{"storage":"32Gi"}},"volumeMode":"Block","storageClassName":"harvester-longhorn"}}]
+      [{"metadata":{"name":"vm-ws-1-cdrom-disk-9sfws","annotations":{"harvesterhci.io/imageId":"default/image-pvvwf"}},"spec":{"accessModes":["ReadWriteMany"],"resources":{"requests":{"storage":"5Gi"}},"volumeMode":"Block","storageClassName":"longhorn-image-pvvwf"}},{"metadata":{"name":"vm-ws-1-rootdisk-giua3"},"spec":{"accessModes":["ReadWriteMany"],"resources":{"requests":{"storage":"32Gi"}},"volumeMode":"Block","storageClassName":"harvester-longhorn"}}]
 ...
 spec:
   template:
     spec:
       domain:
         devices:
           disks:
             - bootOrder: 1
               cdrom:
                 bus: sata
               name: cdrom-disk
             - bootOrder: 2
               disk:
                 bus: virtio
               name: rootdisk
             - bootOrder: 3
               cdrom:
                 bus: sata
               name: virtio-container-disk
             - disk:
                 bus: virtio
               name: cloudinitdisk
 ...
       volumes:
+        - name: cdrom-disk
+          persistentVolumeClaim:
+            claimName: vm-ws-1-cdrom-disk-9sfws
+            hotpluggable: true
         - name: rootdisk
           persistentVolumeClaim:
             claimName: vm-ws-1-rootdisk-giua3
         - containerDisk:
             image: registry.suse.com/suse/vmdp/vmdp:2.5.4.3
           name: virtio-container-disk
         - cloudInitNoCloud:
             networkDataSecretRef:
               name: vm-ws-1-rmeb7
             secretRef:
               name: vm-ws-1-rmeb7
           name: cloudinitdisk
 ...
```

- **`ejectCdRomVolume`**: This action will patch the `VirtualMachine` by removing the target volume from `spec.template.spec.volumes` and its entry from the `harvesterhci.io/volumeClaimTemplates` annotation. The corresponding PVC will also be deleted.

```diff
 apiVersion: kubevirt.io/v1
 kind: VirtualMachine
 metadata:
   annotations:
     harvesterhci.io/volumeClaimTemplates: >-
-      [{"metadata":{"name":"vm-ws-1-cdrom-disk-9sfws","annotations":{"harvesterhci.io/imageId":"default/image-pvvwf"}},"spec":{"accessModes":["ReadWriteMany"],"resources":{"requests":{"storage":"5Gi"}},"volumeMode":"Block","storageClassName":"longhorn-image-pvvwf"}},{"metadata":{"name":"vm-ws-1-rootdisk-giua3"},"spec":{"accessModes":["ReadWriteMany"],"resources":{"requests":{"storage":"32Gi"}},"volumeMode":"Block","storageClassName":"harvester-longhorn"}}]
+      [{"metadata":{"name":"vm-ws-1-rootdisk-giua3"},"spec":{"accessModes":["ReadWriteMany"],"resources":{"requests":{"storage":"32Gi"}},"volumeMode":"Block","storageClassName":"harvester-longhorn"}}]
 ...
 spec:
   template:
     spec:
       domain:
         devices:
           disks:
             - bootOrder: 1
               cdrom:
                 bus: sata
               name: cdrom-disk
             - bootOrder: 2
               disk:
                 bus: virtio
               name: rootdisk
             - bootOrder: 3
               cdrom:
                 bus: sata
               name: virtio-container-disk
             - disk:
                 bus: virtio
               name: cloudinitdisk
 ...
       volumes:
-        - name: cdrom-disk
-          persistentVolumeClaim:
-            claimName: vm-ws-1-cdrom-disk-9sfws
-            hotpluggable: true
         - name: rootdisk
           persistentVolumeClaim:
             claimName: vm-ws-1-rootdisk-giua3
         - containerDisk:
             image: registry.suse.com/suse/vmdp/vmdp:2.5.4.3
           name: virtio-container-disk
         - cloudInitNoCloud:
             networkDataSecretRef:
               name: vm-ws-1-rmeb7
             secretRef:
               name: vm-ws-1-rmeb7
           name: cloudinitdisk
 ...
```

- **Deprecate `ejectCdRom` VM action**: The old `ejectCdRom` action, which detaches the entire device, will be removed.

#### Handling Existing VMs with CD-ROMs

For existing VMs with attached CD-ROMs, the ability to eject the media without a restart will be enabled upon the next VM shutdown. When the VM's VMI is deleted (during a shutdown), the controller will patch the `spec.template.spec.volumes` of any `cd-rom` devices connected via SATA bus to include the `hotpluggable: true` flag.

#### Relaxing the Live Migration Feasibility Check

https://github.com/harvester/harvester/issues/9779

Currently, Harvester blocks the live migration of any VM with a CD-ROM attached, assuming the underlying volume access mode is `ReadWriteOnce` (RWO), which is not migratable. This check is overly restrictive, as CD-ROMs backed by `ReadWriteMany` (RWX) images are safe to migrate.

This enhancement will relax the feasibility check. Instead of blocking migration based on the disk type (`cd-rom`), the validation logic will inspect the status of `LiveMigratable` condition from the VM. This will allow live migration for VMs with CD-ROMs.

#### UI-Related Changes

The following changes will be made to the Harvester UI:
- The VM details page will be fixed to correctly display VMs that have empty CD-ROM devices.
- In the **Volumes** tab of a running VM's detail page:
  - An "Insert" button will be displayed for empty SATA CD-ROM devices.
  - An "Eject" button will be displayed for occupied SATA CD-ROM devices.
- The old "Eject CD-ROM" action, which requires a restart, will be removed.
- On the VM's "Edit Config" page, modifying an attached CD-ROM volume will be forbidden; only its removal will be permitted.
  - To change the media of a running VM, users should use the "Insert" and "Eject" actions on the **Volumes** tab in the detail page.
  - To change the media of a stopped VM, users can remove the existing CD-ROM volume and add a new one in the "Edit Config" page.


### Test plan

This section outlines the testing strategy to ensure the feature is working correctly.

1. **Feature Gate Verification**: After enabling the `DeclarativeHotplugVolumes` feature gate, perform basic checks to ensure existing volume hot-plug functionalities are not regressed.
2. **Empty CD-ROM Creation**: Verify that a VM can be created with one or more empty CD-ROM devices and that the VM's detail page in the UI displays them correctly.
3. **Volume Insertion**: For a running VM with an empty CD-ROM device, verify that a new image can be successfully inserted and that the content is accessible from within the guest OS.
4. **Volume Ejection**: For a running VM with an occupied CD-ROM device, verify that the volume can be successfully ejected and is no longer accessible from within the guest OS.
5. **Live Migration with CD-ROM**: Verify that a running VM with a CD-ROM can be successfully live-migrated.
6. **Existing VM Compatibility**: For a VM created before this enhancement, verify that the existing CD-ROM volume remains un-hotpluggable.

### Upgrade strategy

Since this is a new feature, CD-ROM volumes of the existing VMs would remain as-is no matter they are stopped, running, or restarted. Although, New CD-ROM devices added to existing VMs would be hotpluggable.

## Note

N/A

[^1]: https://kubevirt.io/user-guide/storage/hotplug_volumes/
[^2]: https://groups.google.com/g/kubevirt-dev/c/-tRpvU6CvVY
