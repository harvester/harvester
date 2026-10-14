# External Storage Installation Mode

## Summary

Harvester currently installs Longhorn on every cluster. The installer either creates the `HARV_LH_DEFAULT` partition on the OS disk or formats a separate `DataDisk`, and the Harvester chart deploys Longhorn and its default `harvester-longhorn` StorageClass.

This proposal adds an install-time **external-storage** mode for fresh clusters. In this mode Harvester does not install Longhorn, does not create or format a Longhorn data disk, and uses an administrator-provided third-party CSI StorageClass for features that need persistent storage. The existing Longhorn installation remains the default **standard** mode and is unchanged.

### Related issues

- <https://github.com/harvester/harvester/issues/9098>
## Motivation

Users with an existing storage system do not want Harvester to install a second local storage system or reserve a dedicated disk for it. Reducing Longhorn replicas or resource reservations does not solve this: Longhorn still requires its disk layout and its components remain installed.

The installation choice must therefore be explicit:

- **standard**: current Longhorn installation and behavior.
- **external-storage**: no Longhorn installation; persistent workloads use a compatible third-party CSI driver.

Longhorn must not be replaced in place. Existing PersistentVolumes retain their provisioner; conversion of an existing cluster is a separate migration problem.

## Goals

Auditing the codebase reveals the following components that depend on Longhorn. Each must have a defined behavior in external-storage mode:

| Component | Longhorn dependency | With external SC | Without SC |
|---|---|---|---|
| Rancher Monitoring | `harvester-longhorn` (Prometheus 50Gi, Alertmanager 5Gi, Grafana 2Gi) | Use external RWO SC | Ephemeral on kubelet; reserve ≥57Gi in `COS_PERSISTENT`; data lost on Pod restart |
| CDI | `harvester-longhorn` as scratch SC | Use external SC | Reject; scratch-dependent operations fail |
| k3k addon | `harvester-longhorn` SC | Use external SC | Cannot be enabled |
| `vmstate-persistence` | Longhorn-backed TPM/UEFI state | Use external SC | Ephemeral on kubelet; state lost on VM Pod restart |
| `longhorn-static` | Longhorn RWX SC for upgrade ISO | Use external RWX SC | Block upgrade at preflight |

Based on this audit, the goals are:

- Keep the existing standard Longhorn installation as the default, with no behavior or disk-layout changes.
- Add **external-storage** mode for fresh installations:
  - Do not install Longhorn or Longhorn-only chart resources.
  - Do not create, mount, or format `HARV_LH_DEFAULT`; reject `DataDisk`.
  - Use a selected third-party CSI StorageClass for persistent platform and VM workloads.
  - Retain CDI for supported third-party CSI image workflows.
  - Run Monitoring without persistence when no StorageClass is available.
  - Block features that cannot operate without compatible persistent storage, with clear and actionable errors.
- Support both interactive installer and automated configuration.

## Non-Goals

- Automatically installing, configuring, or certifying a third-party CSI driver.
- Providing storage-network or RWX-network functionality without Longhorn.
- Handling upgrade cases from existing clusters, including:
  - Replacing the provisioner of `harvester-longhorn`, `longhorn-static`, or `vmstate-persistence`.
  - Automatically converting an existing standard cluster to external-storage mode.
  - Automatically migrating VM images, VM volumes, monitoring data, VM state, or upgrade PVCs.
- In external-storage mode, `harvester-longhorn`, `longhorn-static`, and `vmstate-persistence` are never created; removing them from existing clusters is out of scope.

## Proposal

### User Story

#### Install Harvester without Longhorn using an external CSI driver

A user with an existing third-party storage system installs Harvester. They do not want Harvester to provision a separate Longhorn disk or install Longhorn components. They select **external-storage** at install time, skip Longhorn provisioning, and configure their own CSI driver after installation.

### User Experience

The installer presents a storage mode choice:

```text
Storage mode
  [x] Standard          — install built-in Longhorn storage (default)
  [ ] External storage  — do not install Longhorn; configure a third-party
                          CSI StorageClass for persistent workloads
```

When external-storage mode is selected:

- The installer explains that Longhorn backup/restore, snapshots, storage-network, and RWX-network are unavailable.
- `DataDisk` is hidden or disabled; a supplied automated-install `dataDisk` is rejected.
- The installer does not create or mount `HARV_LH_DEFAULT`.
- Harvester does not create `harvester-longhorn`, `longhorn-static`, or `vmstate-persistence`.
- If no compatible class is configured, each affected feature is disabled or rejected instead of silently falling back to a default class or local disk.

### API

Add an explicit mode. An unset value defaults to `standard` for compatibility.

```go
const (
    StorageModeStandard = "standard"
    StorageModeExternal = "external-storage"
)

type Install struct {
    // StorageMode controls storage installation for a new cluster.
    // An empty value defaults to standard.
    StorageMode string `json:"storageMode,omitempty"`
}
```

```yaml
# Existing/default behavior
install:
  storageMode: standard

# No bundled Longhorn; persistent features require third-party CSI storage
install:
  storageMode: external-storage
```

Harvester-owned components must not silently use the cluster default StorageClass; the external class must be explicit per workload.

## Implementation

WIP

## Open Questions

### Should upgrade cases be covered in this issue?

This HEP targets fresh installations only. However, existing clusters will eventually need a migration path. The following cases have been identified:

| Case | Scenario | Action |
|---|---|---|
| 1 | Existing non-Longhorn user → keep Longhorn | No action required |
| 2 | Existing non-Longhorn user → remove Longhorn | (1) Normal upgrade → (2) Migrate Monitoring/vmstate PVCs to new SC → (3) Switch upgrade SC to third-party |
| 3 | Existing Longhorn user → keep Longhorn | No action required |
| 4 | Existing Longhorn user → remove Longhorn | (1) Normal upgrade → (2) Storage-migrate VM volumes → (3) Switch upgrade SC to third-party |

Cases 2 and 4 are non-trivial. Open questions before committing to scope:

- Is there an existing, safe migration path for Monitoring and vmstate PVCs?
- Is it safe to remove Longhorn after storage migration completes? What is the validation criteria?
- Should this be tracked as a separate issue given the complexity?