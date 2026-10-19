# Defer Longhorn Installation for Fresh Clusters

## Summary

Harvester installs Longhorn on every cluster: the installer creates the `HARV_LH_DEFAULT` partition (or formats a `DataDisk`), and the chart deploys Longhorn and its `harvester-longhorn` StorageClass. Users with external storage still have to reserve that disk.

This HEP adds an install-time `deferLonghornInstall` option, for **fresh installations only**. When set, the installer creates no Longhorn disk (no `HARV_LH_DEFAULT`, no `DataDisk`) and the chart deploys no Longhorn workloads. The default install is unchanged.

With `deferLonghornInstall`, the administrator names two CSI provisioners in the install config. Harvester creates the fixed StorageClasses `harvester-system-rwo` (RWO) and `harvester-upgrade-rwx` (RWX, Filesystem) from them. It does not install, configure, or verify the CSI drivers; that is a Day-2 task. See [Implementation](#implementation).

The root disk then holds only the OS partitions (ESP, `COS_OEM`, `COS_STATE`, `COS_RECOVERY`, `COS_PERSISTENT`); see [Root-disk layout](#root-disk-layout).

### Related issues

- <https://github.com/harvester/harvester/issues/9098>

## Motivation

Users may already have an external storage solution or may want to defer storage configuration. They should be able to install Harvester without reserving a Longhorn disk or deploying Longhorn by default.

The installation choice must be explicit:

- **Default**: retain the current Longhorn installation and behavior.
- **`deferLonghornInstall`**: defer Longhorn installation. Storage is provided by CSI drivers that the administrator installs after installation.

The option does not turn an existing cluster into a non-Longhorn cluster. Existing PersistentVolumes retain their provisioner, and removal or replacement of Longhorn in an existing cluster requires a separate migration design.

## Goals

- Keep the current Longhorn installation as the default, with no behavior or disk-layout changes.
- Add a `deferLonghornInstall` option for a fresh initial installation:
  - Keep the 250 GiB minimum root-disk size, and give all space after the fixed partitions to `COS_PERSISTENT` (see [Root-disk layout](#root-disk-layout)).
  - Do not create, mount, or format `HARV_LH_DEFAULT`; reject `DataDisk`.
  - Do not deploy Longhorn workloads or the Longhorn StorageClasses `harvester-longhorn`, `longhorn-static`, and `vmstate-persistence`. The Longhorn CRDs and the `longhorn-system` namespace are still installed; no Longhorn workloads or CRs are created.
  - Require two CSI provisioner names and create the fixed StorageClasses `harvester-system-rwo` and `harvester-upgrade-rwx` from them (see [System StorageClasses](#system-storageclasses)).
  - Clearly tell the administrator that no default CSI storage is available after installation and that the named CSI drivers must be installed and configured in Day 2.
- Support the option in both the interactive installer and automated installation configuration.

### System StorageClasses

When Longhorn is installed (the default), Harvester creates `harvester-system-rwo` and `harvester-upgrade-rwx` with the Longhorn provisioner, so every system workload uses the same StorageClass names on every cluster.

When Longhorn is deferred, Harvester creates both classes with the provisioners the administrator supplied:

| StorageClass | Access mode | Used by | Provisioner setting |
|---|---|---|---|
| `harvester-system-rwo` | RWO | Rancher Monitoring PVCs, CDI scratch space, k3k addon | `harvester.storageClass.system.provisioner` |
| `harvester-upgrade-rwx` | RWX | Upgrade repository (reserved, see [Upgrade](#upgrade)) | `harvester.storageClass.upgrade.provisioner` |

Both settings are required when `deferLonghornInstall` is true. The installer checks only that each value is a valid DNS-1123 subdomain. It does not check that the CSI driver exists or works.

The administrator must install the matching CSI drivers after installation. Until they do, PVCs that use these classes stay `Pending`.

### Root-disk layout

A deferred install changes the partition layout up front, because changing it later needs destructive disk operations. The extra space goes to `COS_PERSISTENT` (it is the last partition and fills the disk); nothing is left unallocated.

Example: single 250 GiB disk, sizes from `pkg/installer/config/constants.go` and `pkg/installer/util/disk.go`.

Default install:

```text
NAME   FSTYPE  LABEL            SIZE   MOUNTPOINT
sda                             250G
├─sda1 vfat    COS_GRUB          64M   /boot/efi
├─sda2 ext4    COS_OEM           50M   /oem
├─sda3 ext4    COS_STATE         15G   /run/initramfs/cos-state
├─sda4 ext4    COS_RECOVERY       8G   /run/initramfs/cos-recovery
├─sda5 ext4    COS_PERSISTENT   150G   /usr/local
└─sda6 ext4    HARV_LH_DEFAULT  ~77G   /var/lib/harvester/defaultdisk
```

With `deferLonghornInstall`: `HARV_LH_DEFAULT` is not created. `COS_PERSISTENT` takes the remaining space.

```text
NAME   FSTYPE  LABEL            SIZE   MOUNTPOINT
sda                             250G
├─sda1 vfat    COS_GRUB          64M   /boot/efi
├─sda2 ext4    COS_OEM           50M   /oem
├─sda3 ext4    COS_STATE         15G   /run/initramfs/cos-state
├─sda4 ext4    COS_RECOVERY       8G   /run/initramfs/cos-recovery
└─sda5 ext4    COS_PERSISTENT  ~226G   /usr/local
```

#### Why 250 GiB

#3842 set the minimum size of `COS_PERSISTENT` to 150 GiB, and adding the other partitions in the disk layout (ESP, OEM, STATE, RECOVERY, about 23 GiB) brings the minimum to about 175 GiB. A 250 GiB disk therefore leaves a safe margin, so keeping 250 GiB as the deferred minimum requires no change to installer validation.

### Effects of deferring the Longhorn installation

| Component | Behavior in a deferred cluster |
|---|---|
| Rancher Monitoring | PVCs (Prometheus 50 Gi, Alertmanager 5 Gi, Grafana 2 Gi) use `harvester-system-rwo`. |
| CDI | Scratch space uses `harvester-system-rwo`. |
| k3k addon | Uses `harvester-system-rwo`. |
| `vmstate-persistence` | Not created. Persistent TPM/UEFI state is unavailable until the administrator provides storage. |
| Upgrade | Not supported. The upgrade webhook rejects every upgrade (see [Upgrade](#upgrade)). |
| Cluster readiness | `/v1/harvester/readyz` skips the longhorn-manager check. |

Longhorn-native backup/restore, snapshots, storage network, and RWX network are unavailable in a deferred cluster.

The guest-cluster CSI driver needs a dedicated compatibility review. It must identify the Longhorn-specific defaults and validations (especially its default StorageClass, snapshot classes, and backup/restore settings) before this feature is implemented. This HEP does not add guest-cluster CSI-driver compatibility for another storage provider; existing Day-2 guest passthrough StorageClass configuration remains the mechanism for supported non-Longhorn host storage.

## Non-Goals

- Automatically installing, configuring, or certifying a third-party CSI driver.
- Verifying at install time that a named CSI provisioner exists or works.
- Making a third-party CSI driver the default StorageClass.
- Providing storage-network or RWX-network functionality without Longhorn.
- A supported Day-2 path to enable Longhorn in a deferred cluster (for example a UI page or editing the `harvester` ManagedChart).
- Upgrading a deferred cluster.
- Handling upgrade cases from existing clusters, including replacing the provisioner of `harvester-longhorn`, `longhorn-static`, or `vmstate-persistence`, removing Longhorn, or migrating existing data.

## Proposal

### User Story

#### Install Harvester without the default Longhorn deployment

A user installs a new Harvester cluster and wants to defer the default Longhorn installation. They select `deferLonghornInstall` and name the CSI provisioners for the system RWO and upgrade RWX classes. Harvester installs without a Longhorn disk or Longhorn workloads. The user then installs and configures those CSI drivers.

### User Experience

For a new cluster, the installer asks about default storage after the password page and before the disk page:

```text
Choose default storage
  ( ) Install Longhorn (default)
  (*) Defer Longhorn installation

  Longhorn is not installed and no data disk or HARV_LH_DEFAULT partition is created.
  No default CSI storage is available after installation. Storage-dependent features
  (monitoring persistence, k3k, VM state, upgrade) need the CSI drivers you name next.
  Install and configure them after installation. Minimum root disk: 250 GiB.
```

Selecting "Defer Longhorn installation" acknowledges that note. The installer then asks for two values, one per page:

```text
System provisioner     CSI provisioner for harvester-system-rwo (RWO)
Upgrade provisioner    CSI provisioner for harvester-upgrade-rwx (RWX)
```

When `deferLonghornInstall` is selected:

- `DataDisk` is hidden, and an automated-install `dataDisk` value is rejected.
- The installer does not create or mount `HARV_LH_DEFAULT`.
- Harvester deploys no Longhorn workloads and no Longhorn StorageClasses. The Longhorn CRDs and `longhorn-system` namespace remain.
- The confirmation page prints the resulting `deferLonghornInstall` and provisioner values.

A joining node does not see this choice. It follows the cluster (see [Node Join Consistency](#node-join-consistency)).

### Node Join Consistency

The first node records an immutable install profile that contains `deferLonghornInstall`. The profile is stored in the ConfigMap `harvester-install-profile` in `harvester-system`. The chart renders it with `immutable: true`. On a chart upgrade, the chart keeps the existing value instead of recomputing it from `longhorn.enabled`. A cluster that was created before this feature has no such ConfigMap and is treated as not deferred.

```go
// InstallProfile holds cluster-wide, install-time decisions.
// Written once at cluster creation; immutable afterwards.
type InstallProfile struct {
    DeferLonghornInstall bool `json:"deferLonghornInstall"`
}
```

The profile contains only `deferLonghornInstall` now. It can be extended with other cluster-wide, install-time decisions later.

No Rancherd change is needed. The installer fetches the profile during the join step, before the node's disk is prepared and before the first boot.

The disk page of the interactive installer comes before the network, server URL, and token pages, and a joining node has no Kubernetes API access. The installer therefore fetches the profile after the token page, with the same authentication that Rancherd uses for `/cacerts`:

- The request carries a random nonce (`X-Cattle-Nonce`) and `Authorization: Bearer base64(sha256(token))`.
- The server compares the bearer value with `base64(sha256(token))` of its own join token, in constant time.
- The server answers with `X-Cattle-Hash`, an HMAC-SHA512 keyed with the token over the nonce and the response body.
- The installer recomputes the HMAC with the token the user entered and rejects the response on a mismatch or a missing header.

This proves that the server knows the join token, so a server that does not know the token cannot supply a forged profile.

The installer builds the request URL from the server URL the user entered (for example `https://192.168.1.100`). The installer skips TLS certificate verification for this request; the HMAC provides the authenticity.

```text
GET https://<server>/v1/harvester/installProfile
X-Cattle-Nonce: <random nonce>
Authorization: Bearer <base64(sha256(token))>

200 OK
X-Cattle-Hash: <base64(HMAC-SHA512(token, nonce \0 body \0))>
{"deferLonghornInstall": true}
```

The route sits next to the existing `/v1/harvester/ui-info` route and is registered before the generic `/v1/harvester/{type}` route. It returns only the `InstallProfile`.

The installer handles the responses as follows:

| Response | Installer behavior |
|---|---|
| `200` with a valid `X-Cattle-Hash` | Use the profile. |
| `200` with a missing or wrong `X-Cattle-Hash` | Show a token mismatch or tampering error and do not continue. Not retried. |
| `404` (older cluster without the route) | Treat Longhorn as not deferred and show a notice on the confirmation page. The join continues. |
| Other status, network error, or timeout | Retry up to 3 times, 2 seconds apart, like the existing `/ping` check. Then show the error and do not continue. |

The join flow is:

1. The joiner enters the server URL and token on the existing panels.
2. The installer fetches the `InstallProfile` from the cluster and verifies the response as described above.
3. The installer sets `install.deferLonghornInstall` from the `InstallProfile`. The cluster value overrides any local value. If a `DataDisk` was already selected on the disk page and the cluster is deferred, the installer clears it and shows a notice on the confirmation page.
4. The installation continues as usual. Partitioning and mounting already read `install.deferLonghornInstall`, and nothing is written to disk until the user confirms the installation.

Automated installation (PXE or a config file) is unaffected, because the flag arrives with the configuration.

The joiner does not need the provisioner settings: the two StorageClasses are cluster objects created once by the first node.

### Upgrade

Upgrading a deferred cluster is not supported. The upgrade validating webhook rejects any `Upgrade` create request when the `harvester` ManagedChart has `longhorn.enabled: false`. The `skip-webhook` annotation does not bypass this check, because the current upgrade workflow needs Longhorn-backed RWX storage.

`harvester-upgrade-rwx` is created now so that a future upgrade workflow for third-party RWX storage has a stable StorageClass name. No current workflow uses it.

### API

Add an explicit initial-installation option. An unset value preserves the current behavior.

```go
type Install struct {
    // DeferLonghornInstall defers Longhorn installation for a fresh
    // installation. An empty or false value installs Longhorn as today.
    DeferLonghornInstall bool `json:"deferLonghornInstall,omitempty"`
}

type StorageClass struct {
    ReplicaCount uint32              `json:"replicaCount,omitempty"`
    System       SystemStorageClass  `json:"system,omitempty"`
    Upgrade      UpgradeStorageClass `json:"upgrade,omitempty"`
}

type SystemStorageClass struct {
    // Provisioner of harvester-system-rwo. Required when deferLonghornInstall is true.
    Provisioner string `json:"provisioner,omitempty"`
}

type UpgradeStorageClass struct {
    // Provisioner of harvester-upgrade-rwx. Required when deferLonghornInstall is true.
    Provisioner string `json:"provisioner,omitempty"`
}
```

```yaml
# Existing/default behavior
install:
  deferLonghornInstall: false

# Fresh installation without a default Longhorn deployment
install:
  deferLonghornInstall: true
harvester:
  storageClass:
    system:
      provisioner: rook-ceph.rbd.csi.ceph.com
    upgrade:
      provisioner: rook-ceph.cephfs.csi.ceph.com
```

The Harvester chart maps these to `longhorn.enabled: false`, `storageClass.system.provisioner`, and `storageClass.upgrade.provisioner`. The chart fails to render when `longhorn.enabled` is false and either provisioner is empty.

Validation (installer config):

- `install.dataDisk` must be empty when `install.deferLonghornInstall` is true.
- Both provisioners are required when `install.deferLonghornInstall` is true, and each must be a valid DNS-1123 subdomain.

## Implementation

### Install config and chart rendering

With `deferLonghornInstall`, the administrator names two CSI provisioners in the install config:

```yaml
install:
  defer_longhorn_install: true
harvester:
  storage_class:
    system:
      provisioner: rook-ceph.rbd.csi.ceph.com      # RWO
    upgrade:
      provisioner: rook-ceph.cephfs.csi.ceph.com   # RWX, Filesystem
```

The installer renders these values into the Harvester chart (`rancherd-10-harvester.yaml`). The two provisioner blocks are emitted only when `deferLonghornInstall` is true:

```yaml
storageClass:
  {{- if .Harvester.StorageClass.ReplicaCount }}
  replicaCount: {{ .Harvester.StorageClass.ReplicaCount }}
  {{- end }}
  {{- if .Install.DeferLonghornInstall }}
  system:
    provisioner: "{{ .Harvester.StorageClass.System.Provisioner }}"
  upgrade:
    provisioner: "{{ .Harvester.StorageClass.Upgrade.Provisioner }}"
  {{- end }}
```

The chart then creates two fixed StorageClasses:

| StorageClass | Provisioner from | Requirement |
|---|---|---|
| `harvester-system-rwo` | `storage_class.system.provisioner` | `ReadWriteOnce` |
| `harvester-upgrade-rwx` | `storage_class.upgrade.provisioner` | `ReadWriteMany`, `volumeMode: Filesystem` |

Harvester only creates the StorageClasses. It does not install, configure, or verify the CSI drivers; that is a Day-2 task for the administrator.

### Steps

1. Add `deferLonghornInstall` and the two provisioner settings to the interactive installer and automated-installation configuration. Preserve the current Longhorn deployment as the default.
2. Gate Longhorn manifests and bootstrap logic so this installation path deploys no Longhorn workloads and no Longhorn StorageClasses. Create `harvester-system-rwo` and `harvester-upgrade-rwx` on every cluster: with Longhorn when it is installed, with the administrator's provisioners when it is deferred. Point Rancher Monitoring, CDI scratch space, and the k3k addon at `harvester-system-rwo`.
3. Audit each remaining Longhorn dependency (controllers, webhooks, and guest-cluster CSI paths). Ensure it either degrades safely when Longhorn is absent or returns an actionable error; it must not silently assume a Longhorn or cluster-default StorageClass. `/v1/harvester/readyz` skips the longhorn-manager check on a deferred cluster.
4. Record the install profile when the cluster is created (immutable ConfigMap `harvester-install-profile`). Add the `GET /v1/harvester/installProfile` route to the Harvester server; it returns only the `InstallProfile` and signs the response as described in Node Join Consistency. Add an installer helper that fetches it after the token step, with retries like the existing `/ping` check, and applies it as described in Node Join Consistency.
5. Reject upgrades of a deferred cluster in the upgrade webhook.
6. Test the default-install regression, fresh installation with `deferLonghornInstall`, absence of Longhorn workloads and Longhorn StorageClasses, the 250 GiB minimum, `DataDisk` rejection, provisioner validation, upgrade rejection, readiness, and the profile fetch (valid hash, wrong hash, missing hash, 404, timeout).
