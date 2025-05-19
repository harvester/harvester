# VM Migration Network

## Summary

Currently, Harvester only supports VM migration over the mgmt network. The mgmt network is used as control plane. The VM migration is data transfer. Users should be able to run it on a different network. This enhancement proposes to support VM migration over different networks.

### Related Issues

https://github.com/harvester/harvester/issues/5848

## Motivation

### Goals

- Users can choose a cluster network for VM migration.
- Users can choose a VLAN ID for VM migration.
- Users can choose a CIDR for VM migration.
- Users can exclude some IPs in CIDR range for VM migration.

### Non-goals [optional]

- Setup a cluster network. Users need to set up a cluster network before using this feature.
- Detect the cluster network is well configured and network connectivity is expected.
- Setup the vm migration network in the installation stage.

## Proposal

#### Story 1

Users want to use a different network for VM migration.
- Users setup a cluster network with network interfaces.
- Users provide the cluster network name, VLAN ID, CIDR range and excluded IPs in the setting.
- The controller creates a Network-Attachment-Definition based on the setting and updates it to the KubeVirt configuration.

#### Story 2

Users want to rollback to use mgmt network for VM migration.
- Users use the default value in the setting.
- The controller will clean up KubeVirt configuration and remove the Network-Attachment-Definition.

### User Experience In Detail

1. Users setup a cluster network with network interfaces.
2. Users provide the cluster network name, VLAN ID, CIDR range and excluded IPs in the setting.
3. The controller create a Network-Attachment-Definition based on the setting and update it to the KubeVirt configuration.
4. KubeVirt automatically restarts the virt-controller and virt-handler pods.
5. During virt-controller and virt-handler pods restarting, users cannot migrate VM. The API server rejects migration requests.
6. After KubeVirt has the Available condition with reason AllComponentsReady. Users can migrate VM on the VM migration network.

### API changes

- Add a new setting `vm-migration-network`.
- Add KubeVirt status check to migrate API.

## Design

### Implementation Overview

#### Webhook

- Verify that the cluster network is present on all nodes.
- Check whether VLAN ID is valid.
- Check whether CIDR range is valid.
- If there is excluded IPs, check whether the IPs are valid and in the CIDR range.
- Check whether available IPs in the CIDR range are bigger or equal to node count, because one node needs one IP.

#### Controller

A new controller `harvester-vm-migration-network-controller`.
- Watch the setting `vm-migration-network` change.
- Check whether the setting has same hash value on `vm-migration-network.settings.harvesterhci.io/hash` annotation.
  - If yes, do nothing and return.
- If no, check whether the setting value is empty.
  - If yes, remove the `vm-migration-network` from KubeVirt configuration and remove the Network-Attachment-Definition.
  - Update the `vm-migration-network.settings.harvesterhci.io/hash` annotation and return.
- If no, create a Network-Attachment-Definition based on the setting and update it to the KubeVirt configuration.
- If there is old `vm-migration-network` on KubeVirt configuration. Remove related Network-Attachment-Definition.
- Update the `vm-migration-network.settings.harvesterhci.io/hash` annotation and return.

### Test plan

- Test webhook can validate cluster network are on all nodes.
- Test webhook can validate VLAN ID is valid.
- Test webhook can validate CIDR range is valid.
- Test webhook can validate excluded IPs are valid and in the CIDR range.
- Test webhook can validate available IPs in the CIDR range are bigger or equal to node count.
- Test controller can create a Network-Attachment-Definition based on the setting and update it to the KubeVirt configuration.
- Test controller can remove the `vm-migration-network` from KubeVirt configuration and remove the Network-Attachment-Definition.

### Upgrade strategy

N/A

## Note [optional]

[KubeVirt document - Using a different network for migrations](https://kubevirt.io/user-guide/compute/live_migration/#using-a-different-network-for-migrations)
