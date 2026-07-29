# Upgrade Helpers

A collection of useful helpers for Harvester upgrades.

subcommands are listed below:

- vm-live-migrate-detector
- version-guard
- image-volume-size-check

## vm-live-migrate-detector

A simple subcommand that helps detect if the VMs on a specific node can be live-migrated to other nodes considering the following:

- Node selectors: If a VM is running on a specific node and has a node selector configured, the VM is considered non-live-migratable
- PCIe devices: If a VM has PCIe devices attached, it is considered non-live-migratable
- Node affinities: If a VM has no place to go after the node affinities are deduced, it is considered non-live-migratable

Besides detecting the live-migratability of the VMs on the node, the tool can also shut down the VMs that are not live-migratable if the `--shutdown` flag is given.

### Usage

```shell
$ upgrade-helper vm-live-migrate-detector --help
A simple VM detector and executor for Harvester upgrades

The detector accepts a node name and inferences the possible places the VMs on top of it could be live migrated to.
If there is no place to go, it can optionally shut down the VMs.

Usage:
  upgrade-helper vm-live-migrate-detector NODENAME [flags]

Flags:
  -h, --help       help for vm-live-migrate-detector
      --shutdown   Shutdown non-migratable VMs

Global Flags:
      --debug   set logging level to debug
      --trace   set logging level to trace
```

Given a node name, it can iterate all the VMs that are running on top of the node, and return with a list of non-live-migratable VMs.

```shell
$ export KUBECONFIG=/tmp/kubeconfig

$ upgrade-helper vm-live-migrate-detector harvester-z5hd8
INFO[0000] Starting VM Live Migrate Detector
INFO[0000] Checking vms on node harvester-z5hd8...
INFO[0000] default/test-vm
INFO[0000] Non-migratable VM(s): [default/test-vm]
```

It can help you shut down those non-live-migratable VMs if you have the `--shutdown` flag specified.

```shell
$ upgrade-helper vm-live-migrate-detector harvester-z5hd8 --shutdown
INFO[0000] Starting VM Live Migrate Detector
INFO[0000] Checking vms on node harvester-z5hd8...
INFO[0000] default/test-vm
INFO[0000] Non-migratable VM(s): [default/test-vm]
INFO[0000] vm default/test-vm was administratively stopped
```

## version-guard

A subcommand that checks whether the version can be upgraded to. The validating criteria includes:

- Disallow any downgrade
- Disallow upgrades from any formal release version lower than the minimal upgrade requirement of the targeting formal release version
- Disallow upgrades from any dev version to any formal release version (if the strict mode is enabled)
- Disallow upgrades from any dev version to any prerelease version (if the strict mode is enabled)
- Allow upgrades from any lower prerelease version to any higher one within the same release version but not across different release versions

Any other upgrade paths not explicitly mentioned above are allowed.

### Usage

```shell
$ export KUBECONFIG=/tmp/kubeconfig

$ upgrade-helper version-guard hvst-upgrade-gqbg5
```

## image-volume-size-check

Volumes created from a VM image must not be smaller than the image's virtual size, or the guest OS can run into data integrity issues once it writes past the shrunk backing store.

This subcommand scans all PVCs cluster-wide and reports the ones created from a VM image (via the `harvesterhci.io/imageId` annotation, or the image's own golden-image PVC) that are smaller than their source image's virtual size. It is read-only and never modifies any resource: PVCs can never be shrunk, and expanding an in-use volume is a decision left to the cluster operator. For each violation it also reports whether the volume is currently expandable, to help the operator decide on a fix.

### Usage

```shell
$ export KUBECONFIG=/tmp/kubeconfig

$ upgrade-helper image-volume-size-check
WARN[0000] "default/test-pvc": current size is smaller than the required minimum of 10Gi; can be expanded
INFO[0010] image volume size check completed: scanned 42, violations 1
```

If run as part of an upgrade, pass `--upgrade` to also record a summary event on the `Upgrade` CR:

```shell
$ upgrade-helper image-volume-size-check --upgrade hvst-upgrade-gqbg5
```
