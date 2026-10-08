# Reduce resources required by Longhorn as an install option

## Summary

Harvester currently require minimal 250Gi of disk to install

### Related Issues

<https://github.com/harvester/harvester/issues/9098>

## Motivation

### Goals

- Allow Harvester to install on a single disk of ~100 GiB or larger (reduced from 250 GiB)
- Introduce a longhornFootprint: minimal install config option, when it set
  - Reduce replica to 1
  - Zero out CPU reservations for Longhorn manager and instance manager.
  - Remove reservation space for longhorn in installation.
- Support both interactive (TUI) and automated (config file) install paths

### Non-goals [optional]

- Completely removing Longhorn as a dependency, ship Longhorn as an addon.

## Proposal

### User Stories

The experience details should be in the `User Experience In Detail` later.

#### Story 1

Users re-utilizing hardware who use 3rd-party storage (majority of deployments) — they have no extra disks, and Harvester's 250 GiB requirement blocks them.

### User Experience In Detail

When installing Harvester on a single disk, the user sees a new option in the TUI: "Longhorn footprint: standard / minimal". If they choose minimal, the disk requirement drops from 250 GiB to 100 GiB. The installer shows a warning: "No data redundancy — single replica only. Not for production use." Install proceeds normally on any disk ≥ 100 GiB.

### API changes

pkg/installer/config/config.go — Add new field to Install struct:

```go
type Install struct {
    // ... existing fields ...
    LonghornFootprint string `json:"longhornFootprint,omitempty"` // "" (default) or "minimal"
}
```

pkg/installer/config/constants.go — Add new constants:

```go
const (
    LonghornFootprintDefault = ""
    LonghornFootprintMinimal = "minimal"

    MinimalDiskMinSizeGiB      uint64 = 100
    MinimalReplicaManagerCPU = 0
    MinimalEngineManagerCPU = 0
    MinimalInstanceManagerCPU = 0
    
    MinimalReplicaCount = 1
)
```

pkg/installer/config/cos.go — Branch Longhorn defaults on LonghornFootprint:
• replicaCount: 1 (minimal) vs 3 (default)
• GuaranteedEngineManagerCPU: 0 (minimal) vs 12 (default)
• GuaranteedReplicaManagerCPU: 0 (minimal) vs 12 (default)
• GuaranteedInstanceManagerCPU: 0 (minimal) vs 12 (default)
pkg/installer/console/validator.go — Branch disk size check on LonghornFootprint:
• Single disk minimum: 100 GiB (minimal) vs 250 GiB (default)

## Design

### Implementation Overview

1. `config.go` — add `LonghornFootprint string` to Install; validate in `validator.go`
2. `validator.go` / `util.go` — disk threshold: 100 GiB (minimal) vs 250 GiB (default)
3. `cos.go` — persistent partition min: 25 GiB (minimal) vs 150 GiB (default)
4. `cos.go` — Longhorn defaults: replicaCount 1, all CPU reservations 0 (minimal)
5. TUI — warning on minimal mode selection (scope TBD)

### Test plan

TBD — needs input from QA.

#### Fresh install

1. `minimal` + 100 GiB disk: passes disk check, Longhorn replica = 1, CPU reservations = 0, VM boots
2. `minimal` + 99 GiB disk: rejected at disk validation step
3. `standard` + 250 GiB disk: behaviour unchanged from today

#### Upgrade

1. `minimal` cluster upgrades successfully (`upgrade_node.sh` robustness check does not false-positive on replica = 1)
2. `standard` cluster upgrades successfully: no regression

### Upgrade strategy

#### Existing installs

During upgrade, check whether any VM or PVC is backed by the Longhorn storage class:

- **Longhorn in use** — keep current settings unchanged
- **Longhorn not in use** — apply minimal settings: `default-replica-count: 1`, all CPU reservations set to `0`

Disk partitioning is not modified in either case.

## Note [optional]

Additional notes.
