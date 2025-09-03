# Migrate from Wicked to NetworkManager

## Summary

Harvester up to and including version v1.6.x is built on SLE Micro 5.x. While SLE Micro 5.x includes both Wicked and NetworkManager network managment stacks, Harvester has always used Wicked. With Harvester v1.7.0 we need to bump the base OS to SL Micro 6.x, which drops support for Wicked and uses NetworkManager exclusively. Consequently we need to teach Harvester how to configure NetworkManager instead of Wicked during installation, and we also need to make sure correct NetworkManager configuration is generated during upgrades.

### Related Issues

- https://github.com/harvester/harvester/issues/3418
- https://github.com/harvester/harvester/issues/9300
- https://github.com/harvester/harvester/issues/9301
- https://github.com/harvester/harvester/issues/9581

## Motivation

### Goals

- Generate Network Manager connection profiles instead of Wicked `ifcfg-*` files during initial installation.
- Generate appropriate Network Manager connection profiles during upgrade from Harvester v1.6.x.
- All currently supported networking options (DHCP, static IP, VLAN, bond options, etc.) must continue to operate correctly.

### Non-goals

- Actually replacing Harvester's base OS with SL Micro 6.x. This will be done as a separate work item for Harvester v1.7 once the NetworkManager configuration is implemented and known to be working correctly against SLE Micro 5.5.

## Proposal

Ideally, from the users's perspective, nothing really changes, provided the user isn't manually customizing the network configuration. Installation should still work as it does now, with the same UI and configuration options. The difference is that behind the scenes we'll have a different network management stack installed, and will be writing out NetworkManager connection profiles directly, instead of going through YAML files in `/oem/90_custom.yaml`.

If the user does need to make manual changes to the network configuration, this can be done using the `nmcli` tool rather than editing YAML.

### User Stories

#### Story 1

I'm installing Harvester v1.7.0 for the first time. I expect networking to operate correctly.

#### Story 2

I'm upgrading from Harvester v1.6.x to v1.7.0. I have _not_ made any post-installation changes to network configuration by manually editing `/oem/*.yaml` files. I expect networking to continue to operate correctly after the upgrade.

#### Story 3

I'm upgrading from Harvester v1.6.x to v1.7.0, but I _have_ made some post-installation changes to network configuration by manually editing `/oem/*.yaml` files. I expect networking to continue to operate correctly after the upgrade.

#### Story 4

I've made a mistake with some manual changes to network configuration, and want to revert back to the settings as they were at install time.

### User Experience In Detail

#### Initial Installation / Upgrades Without Post-installation Configuration Changes

No special user action is required. Everything should just appear to work as it currently does.

#### Upgrades Where Post-installation Configuration Changes Have Been Made

Upgrade should proceed as it currently does, but there may be additional manual steps required depending on what post-installation configuration changes were made.

For example, if bonding options have been changed as described in [the docs], there will have been manual edits made to the `/etc/sysconfig/network/ifcfg-mgmt-bo` file in `/oem/90_custom.yaml`. Those `ifcfg-*` file aren't used by NetworkManager, so the user will need to either:

1. Before upgrade, make appropriate changes to `/oem/harvester.config`, so the upgrade process will generate the expected NetworkManager configuration automatically, or,
2. After upgrade, use the `nmcli` tool to apply the desired configuration to the NetworkManager connection profiles that were generated during upgrade.

[the docs]: https://docs.harvesterhci.io/v1.6/install/update-harvester-configuration/#configuration-persistence-2

#### Reverting Manual Changes to NetworkManager Connection Profiles

The user can run `harvester-installer generate-network-config` to re-generate NetworkManager connection profiles based on the contents of `/oem/harvester.config`.

### API changes

Add a new subcommand to the `harvester-installer` binary to generate NetworkManager connection profiles during upgrade.

## Design

### Implementation Overview

#### Changes to [baseos](https://build.opensuse.org/package/show/isv:Rancher:Harvester:OS:Dev/baseos)

- Don't install `wicked`
- Don't remove `NetworkManager` (it's installed by default in SL Micro, but we're removing it in current Harvester releases)

#### Changes to [os2](https://github.com/harvester/os2)

- Add `files/etc/NetworkManager/conf.d/harvester.conf` specifying `no-auto-default=*` to avoid automatically bringing up all interfaces on the host in DHCP mode (see [NetworkManager for administrators])
- Override the above in `files/system/oem/92_iso_repo.yaml` so the ISO repo still uses DHCP automatically.
- Replace the `ifcfg` dracut module with `network-manager` so `ip=...` kernel command line arguments continue to work.

[NetworkManager for administrators]: https://networkmanager.dev/docs/admins/#server-like-behavior

#### Changes to [harvester-installer](https://github.com/harvester/harvester-installer)

- Remove the old `os.wifi` config code (it's not documented and it doesn't work anyway).
- Generate NetworkManager connection profiles instead of `ifcfg-*` files during installation.
- Add `/var/lib/NetworkManager` and `/etc/NetworkManager` to persistent paths instead of `/var/lib/wicked`.
- Add a subcommand (`harvester-installer generate-network-config`) to generate NetworkManager connection profiles as needed during upgrades.
- Publish the `harvester-installer` binary in a container so it can be used during the upgrade process.
- Find any other remaining occurences of `ifcfg` or `wicked` and replace or remove them as appropriate.

#### Changes to [harvester](https://github.com/harvester/harvester)

- Pull the harvester-installer binary into the upgrade container, then run `harvester-installer generate-network-config` at an appropriate point in `package/upgrade/upgrade_node.sh` to generate NetworkManager connection profiles based on the current `/oem/harvester.config`.

#### Changes to [docs](https://github.com/harvester/docs)

- Anywhere we describe how to tweak network configuration must be updated to talk about NetworkManager instead of wicked.
- We need to add a special section about possible manual changes required during upgrade (see "Upgrade strategy" below).

### Test plan

- Install Harvester and verify that the various different ways of configuring networking all work:
  - Static IP
  - DHCP
  - VLAN
  - MTU
  - Bond Options
  - Management interface MAC address
- Install Harvester v1.6.x then upgrade to v1.7.0, and verify that the upgrade succeeds and networking continues to operate correctly.

### Upgrade strategy

With new installs, NetworkManager connection profiles will be created in `/etc/NetworkManager/system-connections` at install time.

With upgrades, the existing `/oem/90_custom.yaml` file will still include the old `ifcfg-*` files, but these will be ignored by NetworkManager.

During upgrades, we will automatically generate NetworkManager connection profiles in `/etc/NetworkManager/system-connections`.  These will be generated based on the content of `/oem/harvester.config`, so will exactly match the network configuration from when the system was originally installed.

If the user has made manual changes to the networking config in `/oem/90_custom.yaml` after installation, they will need to either update `/oem/harvester.config` before upgrade, or, potentially use `nmcli` to update the connection profiles after upgrade as mentioned [above](#upgrades-where-post-installation-configuration-changes-have-been-made).

In the worst case, if there's trouble after an upgrade and a system comes up with no network configured, it will still be possible to log in via the console, edit `/oem/harvester.config` and then run `harvester-installer generate-network-config` to generate NetworkManager connection profiles.

## References

- https://networkmanager.dev/docs/
- https://networkmanager.dev/docs/api/latest/nm-settings-keyfile.html
- https://www.suse.com/releasenotes/x86_64/SLE-Micro/5.5/index.html#general-network-manager
