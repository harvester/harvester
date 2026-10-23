# Embedded Rancher Decoupling

## Summary

Harvester relies heavily on embedded Rancher in multiple aspects, including lifecycle management, upgrades, and certain settings. Given that Rancher and Harvester share a tight release cadence, which often introduces release risks and unexpected behavior, this enhancement aims to decouple Harvester from embedded Rancher by refactoring existing capabilities.

### Related Issues

https://github.com/harvester/harvester/issues/10712

## Motivation

### Goals

- Manage core applications (ManagedChart) without embedded Rancher
- Upgrade without embedded Rancher
- Keep the standalone UI and integration UI

## Proposal

### User Stories

There are many areas related to embedded Rancher. Here we categorize them into several stories:

#### Story 1: Provision a Harvester cluster without embedded Rancher

- 1 node  (1 management)
- 2 nodes (1 management + 1 worker)
- 3 nodes (3 management)
- 4 nodes (3 management + 1 worker)

Provisioning a multi-node cluster covers node promotion.

#### Story 2: Upgrade from an existing cluster to this new version without embedded Rancher

Existing cluster:
- 1 node  (1 management)
- 2 nodes (1 management + 1 worker)
- 3 nodes (3 management, 2 management + 1 witness)
- 4 nodes (3 management + 1 worker)

#### Story 3: Upgrade a cluster without embedded Rancher

Existing cluster:
- 1 node  (1 management)
- 2 nodes (1 management + 1 worker)
- 3 nodes (3 management, 2 management + 1 witness)
- 4 nodes (3 management + 1 worker)

#### Story 4: Remove a node from a Harvester cluster without embedded Rancher


#### Story 5: External Rancher integration


The following settings need to be refactored for the embedded Rancher decoupling:

#### Story 6: Setting additional-ca should work without embedded Rancher

https://docs.harvesterhci.io/v1.9/advanced/index#additional-ca

#### Story 7: Setting auto-rotate-rke2-certs should work without embedded Rancher

https://docs.harvesterhci.io/v1.9/advanced/index#auto-rotate-rke2-certs

#### Story 8: Setting containerd-registry should work without embedded Rancher

https://docs.harvesterhci.io/v1.9/advanced/index#containerd-registry

#### Story 9: Setting http-proxy should work without embedded Rancher

https://docs.harvesterhci.io/v1.9/advanced/index#http-proxy

#### Story 10: Setting ssl-certificates should work without embedded Rancher

https://docs.harvesterhci.io/v1.9/advanced/index#ssl-certificates

#### Story 11: UI Settings should work without embedded Rancher

https://docs.harvesterhci.io/v1.9/advanced/index#ui-settings

### User Experience In Detail

Ideally, end users should find that most functionality remains the same.

A few advanced features will be dropped:
- The embedded `kubectl` shell accessed by pressing the ` (backtick / grave accent) key
- Embedded Rancher dashboard

## Design

### Implementation Overview

**Core Applications Lifecycle Management System**

We decided to build a standalone ManagedChart controller. This comes with several benefits:
1. It reduces our migration risk since the new controller can hijack existing ManagedCharts.
1. There is no upstream to follow, allowing us to tailor the setup to our needs.
1. Fewer changes are required in code and documentation since ManagedChart is kept.

Other alternatives we considered:

| System / CRD | Main Rejection Reason |
| --- | --- |
| [HelmRelease from FluxCD](https://fluxcd.io/flux/components/helm/helmreleases/) | Controlled by a third party |
| [HelmOp from Fleet](https://fleet.rancher.io/how-tos-for-users/helm-ops) | Uncertain migration path from ManagedChart |

### Test plan

This enhancement is mostly a refactoring effort. Tests can be performed story by story, and the behavior should remain the same compared to the previous version.

### Upgrade strategy

Embedded Rancher currently plays an important role in node-related tasks during upgrades. To achieve a seamless upgrade process without embedded Rancher, we will ship an enhanced version of rancherd in a prior version.


## Note [optional]

Additional notes.
