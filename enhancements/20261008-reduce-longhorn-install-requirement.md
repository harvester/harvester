# Install Fresh Clusters Without the Default Longhorn Data Disk

## Summary

Harvester currently installs Longhorn on every cluster. The installer creates a `HARV_LH_DEFAULT` partition (or formats a `DataDisk`), and the Harvester chart deploys Longhorn and its StorageClasses. This prevents users that supply their own storage from installing Harvester without reserving capacity for Longhorn.

This HEP adds an opt-in, **fresh-install-only** installer setting, `install.skipLonghornDefaultDisk`. It applies to every source of installer configuration: the TUI, `harvester.install.config_url`, and kernel parameters.

When enabled on a node:
- The installer does not create the `HARV_LH_DEFAULT` partition, and gives the space to `COS_PERSISTENT`.
- Longhorn does not create a default disk on that node.

Longhorn itself is **still installed** (workloads and CRDs are unchanged). When `skipLonghornDefaultDisk` is set, the installer does not create the Longhorn StorageClasses (`harvester-longhorn`, `longhorn-static`, `vmstate-persistence`), so any reference to them fails immediately with a Kubernetes-level `not found` error rather than a silent hang. The choice is **reversible**: an administrator can add a Longhorn disk later and the StorageClasses are re-created.

The default installation is unchanged. Changing the partition layout of an existing node is out of scope.

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
  - leaves the Longhorn deployment and CRDs unchanged for now;
  - records the choice in the cluster, so joining nodes use the same disk layout;
  - lets an administrator supply the names of existing StorageClasses for each supported system consumer, through that consumer's own configuration path (add-on, Helm values, or CDI configuration), without manufacturing StorageClasses.
- Keep the root-disk minimum at 250 GiB for the first release. The recovered Longhorn space belongs to `COS_PERSISTENT`; reducing the minimum disk size is a separate sizing proposal.

## Non-goals

- Installing, configuring, certifying, or health-checking a third-party CSI driver during Harvester installation.
- Creating a StorageClass from a CSI provisioner name, or making an external StorageClass the Kubernetes default automatically.
- Converting an existing Longhorn-backed cluster into a cluster without Longhorn, replacing existing PV provisioners, or migrating data.
- Providing Longhorn storage-network, RWX-network, backup/restore, snapshots, or guest-cluster CSI compatibility through another storage backend.
- Removing the Longhorn workloads or changing how the Longhorn CRDs are installed. This is deferred to a follow-up, together with the CRD-only install question under [Open questions](#open-questions).

## Proposal

### User Story

#### Install Harvester without the default Longhorn data disk

A user installs a new Harvester cluster on nodes with no room for a Longhorn disk. They select **Skip Longhorn default disk** in the installer. Harvester installs with no `HARV_LH_DEFAULT` partition, and Longhorn has no disk to use. After installation, the user installs their own CSI driver and tells each Harvester component which existing StorageClass to use. The components below currently hard-code a Longhorn StorageClass:

| Consumer | Hard-coded value | Where it is defined | Configuration |
|---|---|---|---|
| Rancher Monitoring | `harvester-longhorn` (Prometheus, Alertmanager, Grafana) | `addons/built-in/rancher-monitoring/addon-template.yaml` | Set StorageClass in the add-on before enabling it |
| K3K | `harvester-longhorn` | `addons/standalone/rancher-k3k/addon.yaml` | Set StorageClass in the add-on before enabling it |
| CDI | `harvester-longhorn` (scratch space) | `scratchSpaceStorageClass` in `deploy/charts/harvester/values.yaml` | Select from existing StorageClasses, written to `scratchSpaceStorageClass` |
| KubeVirt VM state (vTPM / UEFI) | `vmstate-persistence` | `vmStateStorageClass` in `deploy/charts/harvester/values.yaml` | VM creation blocked if StorageClass does not exist; no admin configuration required (see [Webhook](#webhook)) |
| Upgrade repository | `longhorn-static` (`Filesystem`, via the upgrade ISO image) | `pkg/controller/master/upgrade/upgrade_repo.go` | Administrator selects StorageClass at upgrade time (see [Upgrade](#upgrade)) |

### User Experience

```text
Longhorn default disk
  (*) Create Longhorn default disk (default)
  ( ) Skip Longhorn default disk

If you skip the Longhorn default disk, no HARV_LH_DEFAULT partition is created on this node.
Longhorn is still installed but cannot store volumes until you add a disk on the Hosts page.
Until then, you must install your own CSI driver and configure:

  - Monitoring and K3K: set the StorageClass in the add-on before you enable it.
  - CDI: set scratchSpaceStorageClass as described in the documentation.
  - Upgrade: select the StorageClass before starting an upgrade.
  - VMs with vTPM or UEFI are rejected until Longhorn has a disk or vmstate-persistence is replaced with a working StorageClass.

Minimum root disk: 250 GiB.
```

The installer does not ask for any CSI provisioner or StorageClass name.

When `skipLonghornDefaultDisk` is selected:

- `DataDisk` is hidden, and an automated-install `dataDisk` value is rejected.
- The installer does not create or mount `HARV_LH_DEFAULT`.
- The node does not get the `node.longhorn.io/create-default-disk=true` label, so Longhorn creates no default disk on it.
- The confirmation page prints the resulting `skipLonghornDefaultDisk` value.

A joining node does not see this choice. It follows the cluster (see [Node Join Consistency](#node-join-consistency)).

### Root-disk layout

```text
Default                                   Skip Longhorn default disk

sda  250G                                 sda  250G
├─sda1 vfat  COS_GRUB          64M        ├─sda1 vfat  COS_GRUB          64M
├─sda2 ext4  COS_OEM           50M        ├─sda2 ext4  COS_OEM           50M
├─sda3 ext4  COS_STATE         15G        ├─sda3 ext4  COS_STATE         15G
├─sda4 ext4  COS_RECOVERY       8G        ├─sda4 ext4  COS_RECOVERY       8G
├─sda5 ext4  COS_PERSISTENT   150G        └─sda5 ext4  COS_PERSISTENT  ~226G
└─sda6 ext4  HARV_LH_DEFAULT  ~77G
```

The layout is decided at install time; reclaiming the Longhorn partition later is destructive. The recovered ~77 GiB goes to `COS_PERSISTENT`. The 250 GiB minimum and the 150 GiB floor for `COS_PERSISTENT` are unchanged.

### Node Join Consistency

A joining node must use the same disk layout as the cluster. Before preparing disks, the installer requests the setting from the cluster with the server URL and join token:

```text
GET https://<server>/v1/harvester/installProfile

200 OK
{"skipLonghornDefaultDisk": true}
```

The installer overwrites `install.skipLonghornDefaultDisk` with the cluster's value. When it is `true`, the installer does not create the `HARV_LH_DEFAULT` partition in the [root-disk layout](#root-disk-layout), so the joining node matches the cluster.

Failure handling, so the node never guesses a disk layout:

- `404` means an older cluster that does not know the setting. Longhorn is installed there, so the installer uses `false`.
- If the server cannot be verified or the token is rejected, the installer stops and does not retry.
- Network errors and `5xx` responses are retried with backoff. If they persist, the installer stops before it touches any disk.

### Upgrade

Today the upgrade repository is a two-replica nginx Deployment that mounts the upgrade ISO from a `ReadWriteMany` `longhorn-static` volume. This HEP changes it to `ReadWriteOnce`, so most StorageClasses work:

- The Deployment is reduced to **one replica** with the `Recreate` strategy. The ISO volume is `ReadWriteOnce`, `Filesystem`. When the node running the repository is drained, the pod moves and the volume reattaches on another node.
- On Longhorn, the ISO **volume** keeps its default data replica count (2 or more) — this is independent of the Deployment replica count. Draining a node removes the pod, but Longhorn keeps the data on the remaining replicas, so the volume reattaches successfully.
- On a third-party network-attached StorageClass (e.g., NFS), the volume is accessible from any node by design, so a single Deployment replica is always sufficient.

Harvester does not create a replacement StorageClass. The administrator chooses the StorageClass before starting the upgrade, through the upgrade UI or API.

## Delivery phases

### Phase 1 — Fresh install without Longhorn data disk (this release)

Everything in Phase 1 ships in the same release. A fresh install without Longhorn is not supported on its own: without the storage consumers and the upgrade changes, the cluster cannot run monitoring, persistent VM state, or upgrade. The webhook changes nothing on existing clusters and can be merged first.

#### Webhook

In the VM validator, if `spec.domain.devices.tpm` or `spec.domain.firmware.bootloader.efi` is set, verify that the `vmstate-persistence` StorageClass exists. If it does not, reject the VM.

#### Installer

1. Add `install.skipLonghornDefaultDisk`. Valid only for a fresh install; `install.dataDisk` is rejected when it is `true`. Applies to the TUI, `config_url`, and kernel parameters.
2. Add the TUI choice and confirmation text.
3. Omit `HARV_LH_DEFAULT` during disk preparation and let `COS_PERSISTENT` take the remaining space. Keep the 250 GiB minimum.
4. Write the choice to the cluster-level value used by the Node join API. Longhorn workloads and CRDs remain deployed.
5. Pass `createDefaultDiskLabeledNodes: true` as a Longhorn Helm value and add the default-disk node label as described in [Preventing the Longhorn default disk](#preventing-the-longhorn-default-disk), including the upgrade handling for existing clusters.
6. When `skipLonghornDefaultDisk` is `true`, pass a Helm value that skips creating `harvester-longhorn`, `longhorn-static`, and `vmstate-persistence` StorageClasses.

#### Node join

1. Add `GET /v1/harvester/installProfile`, authenticated with the join token. It returns the cluster-level value.
2. The joining installer fetches the profile before partitioning and overwrites `install.skipLonghornDefaultDisk` with the cluster's value.

#### Upgrade repository

1. Switch the upgrade repository Deployment to one replica, `Recreate`, and a `ReadWriteOnce`, `Filesystem` volume (see [Upgrade](#upgrade)).
2. The administrator selects the StorageClass through the upgrade UI or API before the upgrade starts. When the Longhorn StorageClasses are absent, the upgrade validating webhook requires a non-empty StorageClass and verifies it exists.

### Phase 2 — Remove Longhorn (future, not committed)

Not part of this release. Requires separate design review and Product Management decision.

- **Remove the Longhorn workloads** (`longhorn.enabled: false`). Every Harvester controller, webhook, and API that references Longhorn CRDs must be able to run without Longhorn present. The main open question is CRD ownership: Longhorn CRDs are currently managed by the Longhorn subchart; if Longhorn is disabled, a single owner must take over each CRD before re-enabling Longhorn later, otherwise Helm rejects the install with an ownership conflict. Options are an upstream flag in the Longhorn chart, extending `createCRDs` in Harvester, or vendoring the CRD manifests into `harvester-crd` — each with trade-offs around drift and release coupling.
- **Day-2 Longhorn installation flow** for clusters initially installed without it.

If Phase 2 is never done, Phase 1 is still complete: Longhorn stays deployed and functional.

## Test plan

### StorageClass and webhook

- [ ] Default cluster (Longhorn SCs present): all existing validators are no-ops.
- [ ] `skipLonghornDefaultDisk` install: `harvester-longhorn`, `longhorn-static`, `vmstate-persistence` are not created; any PVC referencing them fails with `not found`.
- [ ] VM with vTPM or UEFI: rejected when `vmstate-persistence` is absent; accepted once it exists (any provisioner).

### Installer and node join

- [ ] `skipLonghornDefaultDisk: true` + `dataDisk` set → rejected. Flag works from TUI and `config_url`.
- [ ] Fresh install with flag: no `HARV_LH_DEFAULT`, `COS_PERSISTENT` takes the rest, control plane ready.
- [ ] No disk label on skipped node; label present and Longhorn creates default disk on normal node. Witness nodes never get the label.
- [ ] Upgrade from older release: `create-default-disk-labeled-nodes` becomes `true`, existing disks unchanged.
- [ ] Join: cluster value propagated, disk layout matches; handles `404` (use `false`), rejected token (stop), retryable errors (backoff then stop before touching disk).

### Storage consumers

Run with an external StorageClass and **no cluster default**.

- [ ] Monitoring, K3K, CDI each use only their configured StorageClass; no PVC lands on `harvester-longhorn` or `longhorn-static`.

### Upgrade repository

- [ ] Upgrade without Longhorn succeeds with an external `ReadWriteOnce` `Filesystem` StorageClass (NFS, LVM), including node drain.
- [ ] On Longhorn: repository volume keeps default replica count; node drain does not block.
- [ ] Missing or preflight-failing StorageClass: rejected with recorded reason, upgrade does not start.
- [ ] `skipLonghornDefaultDisk` preserved after upgrade.

### Regression

With the flag absent, the existing install, join, and upgrade suites pass unchanged.

