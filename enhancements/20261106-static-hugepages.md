# Support configuring static hugepages

## Summary

Currently, Harvester takes over managment of 2MiB hugepages in order to support the Longhorn V2 data engine. If the V2 data engine is enabled, by default Harvester will allocate 1024 x 2MiB hugepages, and if the V2 data engine is disabled, Harvester will delallocate those hugepages. This is problemaic when hugepages are needed for purposes other than the Longhorn V2 data engine. We need to allow users to allocate hugepages on a per-node basis for purposes outside Longhorn (e.g. for use by VMs), while ensuring that there are sufficient hugepages available for Longhorn if the V2 data engine is enabled.

### Related Issues

- https://github.com/harvester/harvester/issues/9459 (this feature)
- https://github.com/harvester/harvester/issues/9460 (GUI work)
- https://github.com/harvester/harvester/issues/9390 (existing LHv2 specific hugepages settings)
- https://github.com/harvester/harvester/issues/12021 (problem with current LHv2 hugepage handling)
- https://github.com/harvester/harvester/issues/5006 (previous work on transparent hugepages, which introduced the hugepages CR, which the current work builds on)

## Motivation

### Goals

- Allow the user to specify exactly how many 2MiB hugepages to allocate on each node
- Automatically allocate sufficient 2MiB hugepages for Longhorn if the LHv2 data engine is enabled
- Prevent the user from shrinking hugepage allocation below what Longhorn needs if the V2 data engine is enabled

### Non-goals

- Provision of cluster-wide hugepage settings
- Configuration of 1GiB (or any other size other than 2MiB) hugepages

## Proposal

### User Stories

#### Story 1

I want to use the Longhorn V2 data engine with hugepages enabled, and let the system take care of allocating the correct amount for me.

#### Story 2

I want to be able to allocate a certain number of static hugepages for use by workloads, reagardless of whether the Longhorn V2 data engine is enabled or not.

### User Experience In Detail

In the case of enabling the Longhorn V2 data engine, the existing behaviour is essentially preserved.  When the user sets the  `longhorn-v2-data-engine-enabled` setting to `true`, Harvester will ensure that there's at least as many 2MiB hugepages as Longhorn needs allocated, taking into account the existing `longhorn-v2-data-engine-hugepage-enabled` and  `longhorn-v2-data-engine-memory-size` settings.

When users wish to specifically allocate hugepages on a per-node basis, they can visit the Hosts screen in the Harvester GUI, then click on an individual host, edit its configuration and set the number of static hugepages to allocate on the Hugepages tab.

If the Longhorn V2 data engine is enabled, Harvester checks the current hugepage allocation on each host.  If it's already greater than or equal to what Longhorn needs, then no further action is taken.  If the hugepage allocation is less than what Longhorn needs, it is increased to that amount, e.g. if there's already 512 hugepages allocated and Longhorn needs 1024, the allocation will be increased to 1024.

If the Longhorn V2 data engine was enabled and is then later disabled, no automatic deallocation of hugepages occurs. The user needs to set hugepages back to 0 on each node if they no longer require them for any other purpose.  This avoids any possibility of changes to Longhorn V2 settings messing up possible manual hugepage allocations.

When the hugepage allocation is changed, once the target number of hugepages is actually set, RKE2 will be restarted on the affected node, in order for the allocation to be reflected in k8s node CRs. All containers will continue to run, but there will be a brief moment during this restart where e.g. calls to validating webhooks may fail with a connection refused error (at least, this is true when `rke2-server` restarts, it may not be noticeable when restarting `rke2-agent` on worker nodes). This needs to be documented.

When enabling the Longhorn V2 data engine, the hugepage allocation on every node will potentially be changed, which means RKE may be restarted on all nodes at about the same time. This will not affect running workloads but may still potentially be slightly disruptive. This was the existing behaviour prior to this HEP and remains unchanged. Enabling the Longhorn V2 data engine is something that's done infrequently on any given cluster, so this is considered low risk and is easy to plan for.

Due to the way hugepage allocation works on Linux, it's possible that a sufficiently large hugepages allocation will never be satisfied due to memory fragmentation.  In this case, RKE2 won't be restarted automatically (we never reach the target allocation), and the user needs to reboot, or try a smaller allocation, to proceed. The user can track the hugepage allocation on the Hugepages tab of the Host details screen in the Harvester GUI. If the value of Total Hugepages under the Meminfo heading matches the number of static hugepages allocated, everthing's fine.  If Total Hugepages is lower than expected, the user needs to either wait a bit for the allocation to succeeed (node-manager will retry the allocation every 30 seconds), or they need to reboot to fix the memory fragmentation. This is similar to the case that's already documented for the [Longhorn V2 data engine](https://docs.harvesterhci.io/v1.9/advanced/longhorn-v2#using-the-longhorn-v2-data-engine) where the user is encouraged to reboot if Longorn raises the "not enough hugepages-2Mi capacity" error.

The user needs to be aware not to try to allocate so many hugepages that the node would be starved of memory.  Very large hugepage allocations will never succeed at runtime, but will still be persisted and attempted again on boot, at which point it's possible to eat so much memory the system will become unresponsive.

### API changes

Add `spec.static.hugepages` and `status.static.hugepages` to the existing hugepages CR. This is a uint64.

## Design

### Implementation Overview

This is all implemented in node-manager:
- Move hugepage allocation and config persistence from the nodeconfig controller to the hugepages controller.
- When the nodeconfig changes (where the number of hugepages Longhorn requires is set), check if there's enough hugepages in the relevant hugepages CR and increase it if necessary.
- Add an admission webhook to ensure the user can't reduce the hugepage allocation below what Longhorn needs if the V2 data engine is enabled.

The PR is https://github.com/harvester/node-manager/pull/251

### Test plan

- On a fresh install:
  - Allocate some hugepages via the GUI and/or by editing hugepages CR
  - Verify allocation is applied (check `/sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages`)
  - Verify allocation is persisted (check `/oem/99_settings.yaml`)
  - Verify kubelet sees allocation (`kubectl get node -o yaml|grep hugepages-2Mi`)
    - This may take a little time if memory is fragmented. If memory is too fragmented or allocation is too high it may require a reboot.
  - Enable the V2 data engine
    - Verify hugepages allocation goes up appropriately
- For upgrades:
  - Install an older version with the V2 data engine enabled
  - Upgrade to this version and verify that the appropriate hugepage allocation is present in the hugepage CRs and is persisted correctly (note different stanzas in `/oem/99_settings.yaml` pre and post upgrade)

TBD: flesh the above out with more specific details

### Upgrade strategy

No special handling is required in the upgrade process outside of node-manager.

The `nodeconfig.OnNodeConfigChange()` function always runs at least once when node-manager starts, and this is where the code that checks the current hugepage allocation and LHv2 data engine enablement status resides. If the LHv2 data engine is enabled prior to upgrade, this code will then kick in after upgrade and automatically set the correct number of hugepages in the relevant hugepages CR.  The hugepages controller will then persist and apply that configuration.

The one weird edge case is if `hugepage.OnChange()` runs before `nodeconfig.OnNodeConfigChange()` when node-manager starts up. If this occurs, it could potentially persist and apply an allocation of zero.  We avoid this by having `hugepage.OnChange()` checks the hugepage CR, the current runtime allocation (`/sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages`) and whether or not LHv2 is enabled. If the hugepage CR says 0, but the runtime allocation is > 0, and LHv2 requires a hugepage allocation, the event is requeued in order to give `nodeconfig.OnNodeConfigChange()` time to apply the necessary change. That change will be applied almost instantly once node-manager starts after upgrade.
