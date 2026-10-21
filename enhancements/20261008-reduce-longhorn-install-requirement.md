# Install Fresh Clusters Without the Default Longhorn Data Disk

## Summary

Harvester currently installs Longhorn on every cluster. The installer creates a `HARV_LH_DEFAULT` partition (or formats a `DataDisk`), and the Harvester chart deploys Longhorn and its StorageClasses. This prevents users that supply their own storage from installing Harvester without reserving capacity for Longhorn.

This HEP adds an opt-in, **fresh-install-only** setting, `install.skipLonghornInstall`, that is the single source for the behavior below. It applies to every source of installer configuration: the TUI, `harvester.install.config_url`, and kernel parameters.

When enabled:
- The installer does not provision a Longhorn data disk.
- Harvester blocks Longhorn StorageClasses and Longhorn-only features, so no workload can use Longhorn storage.

In this first step the Longhorn workloads and CRDs are still deployed by the chart (`longhorn.enabled` stays `true`).

The default installation is unchanged. Upgrading an existing cluster into or out of this mode is out of scope.

Harvester does not install or manage the replacement CSI driver and never creates a StorageClass for an administrator's provisioner. Administrators supply the names of existing StorageClasses instead.

### Related issue

- <https://github.com/harvester/harvester/issues/9098>

## Motivation

Users can already run workloads on external storage, but they must currently reserve a Longhorn disk even when they do not intend to use Longhorn. On a single 250 GiB root disk, the default Longhorn partition consumes about **77 GiB**; on larger disks it consumes all space beyond the fixed partition sizes, more details in [Root-disk layout](#root-disk-layout).

Removing Longhorn in fresh clusters makes better use of constrained hardware and lets administrators bring their own storage. However, this does not imply that every external CSI driver provides Longhorn features such as share-manager-backed RWX, storage networks, snapshots, or backup integration.

## Goals

- Preserve the current Longhorn installation and disk layout as the default.
- Provide an explicit fresh-install choice that:
  - creates no `HARV_LH_DEFAULT` partition and rejects `DataDisk`;
  - blocks Longhorn StorageClasses and Longhorn-only settings, APIs, and UI entries through Harvester admission;
  - leaves the Longhorn deployment and CRDs unchanged for now;
  - records the choice in the cluster, so joining nodes use the same disk layout;
  - lets an administrator supply the names of existing StorageClasses for each supported system consumer, through that consumer's own configuration path (add-on, Helm values, or CDI configuration), without manufacturing StorageClasses.
- Keep the root-disk minimum at 250 GiB for the first release. The recovered Longhorn space belongs to `COS_PERSISTENT`; reducing the minimum disk size is a separate sizing proposal.
- Webhooks provide centralized validation to block unsupported operations when Longhorn is not usable because it has no data disk.
- Enable each storage consumer only after its storage requirements and failure behavior have been validated.

## Non-goals

- Installing, configuring, certifying, or health-checking a third-party CSI driver during Harvester installation.
- Creating a StorageClass from a CSI provisioner name, or making an external StorageClass the Kubernetes default automatically.
- Converting an existing Longhorn-backed cluster into a cluster without Longhorn, replacing existing PV provisioners, or migrating data.
- Providing Longhorn storage-network, RWX-network, backup/restore, snapshots, or guest-cluster CSI compatibility through another storage backend.
- Removing the Longhorn workloads or changing how the Longhorn CRDs are installed. This is deferred to a follow-up, together with the CRD-only install question under [Open questions](#open-questions).

## Proposal

### User Story

#### Install Harvester without the default Longhorn data disk

A user installs a new Harvester cluster and does not want Longhorn. They select `skipLonghornInstall` in the installer, and Harvester installs without a Longhorn data disk and blocks the use of Longhorn StorageClasses.

After installation, the user installs their own CSI driver and tells each Harvester component which existing StorageClass to use. The components below currently hard-code a Longhorn StorageClass:

| Consumer | Hard-coded value | Where it is defined |
|---|---|---|
| Rancher Monitoring | `harvester-longhorn` (Prometheus, Alertmanager, Grafana) | `addons/built-in/rancher-monitoring/addon-template.yaml` |
| K3K | `harvester-longhorn` | `addons/standalone/rancher-k3k/addon.yaml` |
| CDI | `harvester-longhorn` (scratch space) | `scratchSpaceStorageClass` in `deploy/charts/harvester/values.yaml` |
| KubeVirt VM state | `vmstate-persistence` | `vmStateStorageClass` in `deploy/charts/harvester/values.yaml` |
| Upgrade repository | `longhorn-static` (RWX, `Filesystem`, via the upgrade ISO image) | `pkg/controller/master/upgrade/upgrade_repo.go` |

### User Experience

```text
Choose default storage
  (*) Install Longhorn (default)
  ( ) Do not install Longhorn

If you choose not to install Longhorn, no data disk or HARV_LH_DEFAULT partition will be created, Longhorn StorageClasses are blocked, and Longhorn cannot be enabled later. No default CSI storage is available after installation. You must install your own CSI driver and configure a StorageClass:

  - Monitoring and K3K: set the StorageClass in the add-on before you enable it.
  - VM state and upgrade: set the StorageClass through Helm values.

Minimum root disk: 250 GiB.
```

The installer does not ask for any CSI provisioner or StorageClass name.

When `skipLonghornInstall` is selected:

- `DataDisk` is hidden, and an automated-install `dataDisk` value is rejected.
- The installer does not create or mount `HARV_LH_DEFAULT`.
- Harvester blocks Longhorn StorageClasses and Longhorn-only features (see [Blocking Longhorn StorageClasses](#blocking-longhorn-storageclasses)). The Longhorn deployment itself is unchanged.
- The confirmation page prints the resulting `skipLonghornInstall` value.

A joining node does not see this choice. It follows the cluster (see [Node Join Consistency](#node-join-consistency)).

### Root-disk layout

The disk layout changes at installation time, because reclaiming the Longhorn partition later is destructive. The extra space goes to `COS_PERSISTENT`, the last partition, and no capacity is left unallocated.

For a single 250 GiB disk, using the current installer constants:

```text
Default installation

NAME   FSTYPE  LABEL            SIZE   MOUNTPOINT
sda                             250G
├─sda1 vfat    COS_GRUB          64M   /boot/efi
├─sda2 ext4    COS_OEM           50M   /oem
├─sda3 ext4    COS_STATE         15G   /run/initramfs/cos-state
├─sda4 ext4    COS_RECOVERY       8G   /run/initramfs/cos-recovery
├─sda5 ext4    COS_PERSISTENT   150G   /usr/local
└─sda6 ext4    HARV_LH_DEFAULT  ~77G   /var/lib/harvester/defaultdisk

Install without Longhorn

NAME   FSTYPE  LABEL            SIZE   MOUNTPOINT
sda                             250G
├─sda1 vfat    COS_GRUB          64M   /boot/efi
├─sda2 ext4    COS_OEM           50M   /oem
├─sda3 ext4    COS_STATE         15G   /run/initramfs/cos-state
├─sda4 ext4    COS_RECOVERY       8G   /run/initramfs/cos-recovery
└─sda5 ext4    COS_PERSISTENT  ~226G   /usr/local
```

The 250 GiB threshold is retained. `COS_PERSISTENT` still has its 150 GiB minimum, and the fixed OS partitions consume roughly 23 GiB. Changing this validation is outside this HEP.

### Blocking Longhorn StorageClasses

With no `HARV_LH_DEFAULT` partition, Longhorn has no intended data disk, so Harvester must prevent anything from using it. When the install profile has `skipLonghornInstall: true`, Harvester admission:

- rejects creating or updating a StorageClass whose provisioner is `driver.longhorn.io`;
- rejects a PVC, VM volume, or image that references a Longhorn StorageClass, including the chart-created `harvester-longhorn`, `longhorn-static`, and `vmstate-persistence`;
- hides Longhorn StorageClasses in the UI capability response.

The checks use the shared `LonghornAvailable` predicate defined in [Phase 1a](#1a--centralized-longhorn-availability-gate), and the error says the feature requires a Longhorn-enabled cluster. How the three chart-created classes are hidden or disabled (for example by gating them in the chart later) is an implementation decision for Phase 1b.

Risk to verify: Longhorn still runs and its default disk path, `/var/lib/harvester/defaultdisk`, would then sit on `COS_PERSISTENT`. A Longhorn volume that bypasses admission would write to the system partition. Phase 2a step 5 must confirm how that path is created and whether Longhorn scheduling on it must be disabled.

### Node Join Consistency

A joining node must use the same disk layout as the cluster. Before preparing disks, the installer requests the setting from the cluster with the server URL and join token:

```text
GET https://<server>/v1/harvester/installProfile

200 OK
{"skipLonghornInstall": true}
```

The installer overwrites `install.skipLonghornInstall` with the cluster's value. When it is `true`, the installer does not create the `HARV_LH_DEFAULT` partition in the [root-disk layout](#root-disk-layout), so the joining node matches the cluster.

Failure handling, so the node never guesses a disk layout:

- `404` means an older cluster that does not know the setting. Longhorn is installed there, so the installer uses `false`.
- If the server cannot be verified or the token is rejected, the installer stops and does not retry.
- Network errors and `5xx` responses are retried with backoff. If they persist, the installer stops before it touches any disk.

### Upgrade (TBD)

The upgrade repository needs a StorageClass that supports `ReadWriteMany` access mode and `Filesystem` volume mode. Today the upgrade controller hard-codes `longhorn-static` for the upgrade ISO image. Harvester does not create a replacement StorageClass. How the administrator provides one is undecided and is tracked in [Phase 4](#phase-4--upgrade-tbd-the-details). The candidates are:

- **Helm value (candidate):** the administrator sets `upgradeStorageClass`. When `skipLonghornInstall: true`, the upgrade validating webhook requires a non-empty value and verifies that the named StorageClass exists. The `skip-webhook` annotation does not bypass this validation.
- **At upgrade time (candidate):** the administrator chooses the StorageClass before the upgrade starts.

Whichever candidate is chosen, the following preflight applies. Before beginning the upgrade, the upgrade controller runs a storage preflight. It provisions a temporary `ReadWriteMany`, `Filesystem` PVC using the configured StorageClass and verifies that two test pods can mount it concurrently. If the preflight fails, the controller records an actionable failure in the Upgrade status, cleans up the temporary resources, and does not start the upgrade.

## Delivery phases

### Phase 1 Centralize Webhook (no behavior change)
#### 1a — centralized Longhorn availability gate

1. Record `skipLonghornInstall` as a cluster-level value (set through TUI or read from control-plane node), written once at bootstrap.
2. Add a `LonghornAvailable` predicate that reads this value. A missing value means Longhorn is available, so existing clusters are unaffected.
3. Add a central webhook entry point that calls the predicate. Validators call this gate instead of doing their own Longhorn checks.

**Exit criterion:** the gate is merged and is a no-op on every existing cluster.

#### 1b — deny rules, added incrementally

Each rule is a small, independent change that calls the gate. When Longhorn is unavailable, Harvester admission rejects:

1. creating or updating a StorageClass with provisioner `driver.longhorn.io`;
2. a PVC, VM volume, or VM image that references a Longhorn provisioner;
3. the Longhorn-only settings:
   - `storage-network`
   - `rwx-network`
   - `longhorn-v2-data-engine-enabled`
   - `longhorn-v2-data-engine-hugepage-enabled`
   - `longhorn-v2-data-engine-memory-size`

The UI capability response hides the same features. Every error says the feature requires a Longhorn-enabled cluster. An implementation audit adds every remaining setting, API action, and UI entry that assumes a Longhorn manager or a Longhorn volume.

How the three chart-created StorageClasses are hidden or disabled (for example by gating them in the chart later) is an implementation decision.

**Exit criterion:** each rule has a positive and a negative test.

> [!IMPORTANT]
> Phases 2, 3, and 4 must ship in the same release. A fresh install without Longhorn (Phase 2) is not supported on its own: without the storage consumers (Phase 3) and upgrade (Phase 4), the cluster cannot run monitoring, persistent VM state, or upgrade.

### Phase 2 Fresh install without Longhorn data disk (user-visible)

#### 2a — installer: skip the Longhorn data disk

1. Add `install.skipLonghornInstall`. Valid only for a fresh install; `install.dataDisk` is rejected when it is `true`. Applies to the TUI, `config_url`, and kernel parameters.
2. Add the TUI choice and confirmation text.
3. Omit `HARV_LH_DEFAULT` during disk preparation and let `COS_PERSISTENT` take the remaining space. Keep the 250 GiB minimum.
4. Write the choice to the cluster-level value defined in Phase 1a. Longhorn workloads and CRDs remain deployed.
5. Ensure Longhorn creates no default disk on these nodes, so a volume that bypasses admission fails to schedule instead of writing to `COS_PERSISTENT`. Candidate: `createDefaultDiskLabeledNodes` without the default-disk label; must be verified.

**Exit criterion:** a fresh cluster installs with no `HARV_LH_DEFAULT` partition, Longhorn reports no schedulable disk, the Phase 1 rules reject Longhorn use, and the control plane is ready. The default installation has no regression.

#### 2b — node join consistency

1. Add `GET /v1/harvester/installProfile`, authenticated with the join token. It returns the cluster-level value.
2. The joining installer fetches the profile before partitioning and overwrites `install.skipLonghornInstall` with the cluster's value.

**Exit criterion:** join tests cover a valid response, a failed verification, an older cluster (`404`), retryable failures, and data-disk clearing.

### Phase 3 — administrator-configured storage consumers

Each consumer is a separate change and is supported only after its end-to-end test passes. Harvester only stores and wires StorageClass **names**; it never creates a StorageClass. Each consumer has its own configuration path:

| Consumer | Where the administrator sets the StorageClass |
|---|---|
| Monitoring, K3K | In the add-on's configuration, before the add-on is enabled. |
| VM state | Through the Harvester chart's Helm values (`vmStateStorageClass`). |
| CDI | Selected from the existing StorageClasses, then written to `scratchSpaceStorageClass`. |

1. **Monitoring / K3K:** the add-on lets the administrator **specify an existing StorageClass**.
2. **VM state:** `vmStateStorageClass` is already a chart value that defaults to `vmstate-persistence`. The administrator overrides it, and Harvester validates that the named StorageClass exists before using it.
3. **CDI:** the administrator selects an existing StorageClass, which Harvester applies to `scratchSpaceStorageClass`.

**Exit criterion:** each enabled consumer has an end-to-end test with an external StorageClass and a negative test for a missing StorageClass.

### Phase 4 — upgrade (TBD the details)

The upgrade repository needs an RWX, `Filesystem` StorageClass. The way the administrator provides it is still under discussion; the candidates are not exclusive:

- **Helm value:** the administrator sets `upgradeStorageClass` through Helm values (see [Upgrade](#upgrade-tbd)).
- **At upgrade time:** the administrator chooses the StorageClass before the upgrade process starts.

**Exit criterion:** an upgrade of a Phase 1-3 cluster succeeds with an external RWX class, and the Longhorn-enabled upgrade regression suite still passes.

### Phase 5 — follow-up (not committed)

Not part of this release. Each item needs its own design review and a Product Management decision:

- **Remove the Longhorn workloads** (`longhorn.enabled: false`). Blocked by the CRD-only install question under [Open questions](#open-questions).
- Day-2 Longhorn installation flow.



## Test plan

Each phase's tests map to its exit criterion.

### Phase 1: webhook

- [ ] `LonghornAvailable` returns `true` when the value is missing or `false`, and `false` when it is `true`.
- [ ] On a cluster without the value, every existing validator behaves as before (no-op).
- [ ] When Longhorn is unavailable, each deny rule rejects (positive) and the same request is allowed when Longhorn is available (negative):
  - StorageClass with provisioner `driver.longhorn.io`
  - PVC, VM volume, or VM image that references a Longhorn provisioner
  - the five Longhorn-only settings
- [ ] The UI capability response hides Longhorn features.
- [ ] Every rejection says the feature requires a Longhorn-enabled cluster.

### Phase 2: fresh install and node join

- [ ] `skipLonghornInstall: true` with `dataDisk` set is rejected; the flag works from the TUI.
- [ ] Fresh install with the flag on a 250 GiB disk: no `HARV_LH_DEFAULT`, `COS_PERSISTENT` takes the rest, control plane is ready.
- [ ] Longhorn reports no schedulable disk, and a volume that bypasses admission fails to schedule instead of writing to `COS_PERSISTENT`.
- [ ] Joining node follows the cluster value, the disk layout has no `HARV_LH_DEFAULT` and `COS_PERSISTENT` takes the rest
- [ ] Join handles: valid response, rejected token or unverifiable server (stop, no retry), older cluster (`404` uses `false`), retryable failures (backoff, then stop before touching any disk)

### Phase 3: storage consumers

Run with an external StorageClass and **no cluster default**, so a silent fallback is visible.

- [ ] Monitoring, K3K, VM state, and CDI each use only the StorageClass the administrator set.
- [ ] No PVC uses `harvester-longhorn`, `longhorn-static`, or `vmstate-persistence`.

### Phase 4: upgrade (finalize once the design is decided)

- [ ] Upgrade of a Phase 1-3 cluster succeeds with an external RWX `Filesystem` StorageClass.
- [ ] A missing or incompatible (RWO) StorageClass is rejected, with a recorded reason, and the upgrade does not start.
- [ ] `skipLonghornInstall` is preserved after upgrade.

### Regression in all Phases

With the flag absent, the existing install, join, and upgrade suites pass unchanged.

## Open questions

### Should Longhorn provide an upstream "CRDs only" install mode?

Deferred: while the Longhorn deployment is unchanged, the CRDs keep coming from the existing install path. This question applies only to a later step that removes the Longhorn workloads.

Shared premise: a CRD object has one owner. Helm records it in the `meta.helm.sh/release-name` annotation, and `createCRDs` applies CRDs through its own wrangler apply set. If more than one owner manages the same Longhorn CRD, for example `harvester-crd`, the Longhorn subchart (`templates/crds.yaml`), or `createCRDs`, Helm rejects the install with an "invalid ownership metadata" error, or the owners overwrite each other and drift when their versions differ. Options 2 and 3 are only safe if exactly one owner is defined per CRD and handed over explicitly when Longhorn is later enabled.

Options:

1. **Upstream flag in the Longhorn chart.** Single source of truth and no CRD drift, but it depends on the Longhorn release cycle and acceptance.
2. **Extend `createCRDs` in Harvester.** No external dependency, but the schemas are generated from Go structs. That already fails for `BackingImage`, so this risks drifting from the real Longhorn CRDs.
3. **Vendor the Longhorn CRD manifests into `deploy/charts/harvester-crd`.** Version-pinned and independent of the Longhorn chart, but it must be refreshed on every Longhorn bump.
