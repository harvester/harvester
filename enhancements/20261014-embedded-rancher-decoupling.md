# Embedded Rancher Decoupling

## Summary

Harvester heavily relies on an embedded Rancher on multiple aspects, including lifecycle management, upgrades, and some settings. Given that Rancher and Harvester share tight release cadence, often causing the release risk and unexpected behavior, this enhancement aims to decouple embedded Rancher by refactoring the existing capabilities.

### Related Issues

https://github.com/harvester/harvester/issues/10712

## Motivation

### Goals

- Manage core applications (ManagedChart) without embedded Rancher
- Upgrade without embedded Rancher
- Keep the standalone UI and integration UI

## Proposal

### User Stories

There are many areas related to embedded Rancher.

#### Story 1: Provision a Harvester cluster without embedded Rancher

- 1 node  (1 management)
- 2 nodes (1 management + 1 worker)
- 3 nodes (3 management)
- 4 nodes (3 management + 1 worker)

Provision a multi-node cluster covers the node promotion.

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


Following are settings that need to be refactored for the embedded Rancher Decoupling

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

#### Story 11: UI Setting should work without embedded Rancher

https://docs.harvesterhci.io/v1.9/advanced/index#ui-settings

### User Experience In Detail

Ideally, end users should notice most of the functionalities stay the same.

A few advance functionalities would be dropped.
- The embedded `kubectl` shell by pressing the ` (backtick / grave accent) key
- Embedded Rancher dashboard

## Design

### Implementation Overview

**Core Applications Lifecycle Management System**

We decide to build a standalone ManagedChart controller. This comes with serveral benefits,
1. It reduces our migration risk since new controller can hijack the existing ManagedCharts.
1. There is no upstream to follow. Allow us to tailor the setup to our needs
1. Less change required in code and documents since ManagedChart is kept

Other alternatives we consider

| System / CRD | Main Rejected Reason  |
| --- | --- |
| [HelmRelease from FluxCD](https://fluxcd.io/flux/components/helm/helmreleases/) | Controllered by 3rd party |
| [HelmOp from Fleet](https://fleet.rancher.io/how-tos-for-users/helm-ops) | Uncertain migration path from ManagedChart |

### Test plan

This enhancement is mostly refactoring. Tests can be performed story by story, and the behavior should be the same compared with the previous version.

### Upgrade strategy

Embedded Rancher currently plays an important role on node related tasks during upgrading. To achieve seamless
upgrade process without embedded Rancher, we'll ship an enhanced version of rancherd in a prioer version.


## Note [optional]

Additional notes.
