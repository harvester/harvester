# KubeVirt Migration Setting

## Summary

This HEP is to exposed KubeVirt migration settings in Harvester UI, so that users can configure the migration settings via Harvester.

### Related Issues

https://github.com/harvester/harvester/issues/8581

## Motivation

### Goals

- Expose KubeVirt migration configuration as the `kubevirt-migration` setting in Harvester UI.
- Don't break upgrade scripts. The upgrade scripts rely on `nodeDrainTaintKey`, so users cannot change it.
- Don't overwrite `network`. The migration network is set by `vm-migration-network` setting.

### Non-goals [optional]

N/A

## Proposal

### User would like to increase parallel migrations

The default parallel migrations is 5, which means only five VMs can be migrated at a time. If user has many VMs and want to migrate them faster, they may want to increase the parallel migrations.

* Set `parallelMigrationsPerCluster` in `kubevirt-migration` setting to a larger number, e.g. 10.

### API changes

N/A

## Design

### Controller

* User update `kubevirt-migration` setting via Harvester UI.
* The controller watches the `kubevirt-migration` setting and updates the KubeVirt `harvester-system/kubevirt` CR accordingly.
* Ignore changes to `nodeDrainTaintKey` in the setting to avoid breaking upgrade scripts.
* Ignore changes to `network` in the setting to avoid breaking `vm-migration-network` setting.
* Update the KubeVirt CR only when the setting is changed.

### Webhook

* Reject changes if `kubevirt-migration` setting cannot be decoded as KubeVirt migration configuration.
* Reject changes if there is VM migration in progress.
* Reject changes if `nodeDrainTaintKey` is changed.
* Reject changes if `network` is changed.

### Test plan

1. The wrong schema for `kubevirt-migration` setting is rejected.
2. Changes to `nodeDrainTaintKey` in `kubevirt-migration` setting is rejected.
3. Changes to `network` in `kubevirt-migration` setting is rejected.
4. Changes to `kubevirt-migration` setting is rejected if there is VM migration in progress.
5. Changes to `kubevirt-migration` setting is accepted if it's not rejected by the above rules and there is real changes in the `kubevirt-migration` setting.

### Upgrade strategy

N/A
