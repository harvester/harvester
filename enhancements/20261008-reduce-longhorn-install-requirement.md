# Defer Longhorn Installation for Fresh Clusters

## Summary

Harvester currently installs Longhorn on every cluster. The installer either creates the `HARV_LH_DEFAULT` partition on the OS disk or formats a separate `DataDisk`, and the Harvester chart deploys Longhorn and its default `harvester-longhorn` StorageClass. This makes the current 500 GiB root-disk requirement necessary even for users who do not intend to use Longhorn.

This feature is for **fresh installations only**. It adds an install-time `deferLonghornInstall` option that defers the default Longhorn installation and reduces the minimum root-disk size from 500 GiB to 250 GiB. **It does not configure another CSI driver**. CSI selection, installation, and configuration are Day-2 operations owned by the administrator. **Enabling Longhorn later in a deferred cluster is out of scope**: the option is install-time only and one-way.

### Related issues

- <https://github.com/harvester/harvester/issues/9098>

## Motivation

Users may already have an external storage solution or may want to defer storage configuration. They should be able to install Harvester without reserving a Longhorn disk or deploying Longhorn by default.

The installation choice must be explicit:

- **Default**: retain the current Longhorn installation and behavior.
- **`deferLonghornInstall`**: defer Longhorn installation until Day 2. Storage is configured later by the administrator.

The option does not turn an existing cluster into a non-Longhorn cluster. Existing PersistentVolumes retain their provisioner, and removal or replacement of Longhorn in an existing cluster requires a separate migration design.

## Goals

- Keep the current Longhorn installation as the default, with no behavior or disk-layout changes.
- Add a `deferLonghornInstall` option for a fresh initial installation:
  - Reduce the minimum root-disk size from 500 GiB to 250 GiB. The 250 GiB limit remains the minimum for OS and platform data; it is not a claim that the disk can be reduced indefinitely.
  - Do not create, mount, or format `HARV_LH_DEFAULT`; reject `DataDisk`.
  - Do not deploy Longhorn workloads or Longhorn StorageClasses such as `harvester-longhorn`, `longhorn-static`, and `vmstate-persistence`. The Longhorn CRDs and the `longhorn-system` namespace are still installed; no Longhorn workloads or CRs are created.
  - Clearly tell the administrator that no default CSI storage is available after installation and that storage-dependent features require Day-2 configuration.
- Support the option in both the interactive installer and automated installation configuration.

### Effects of deferring the Longhorn installation

The following table describes the behavior after a fresh installation with `deferLonghornInstall`. It does **not** make the installer configure an external CSI driver or StorageClass: the administrator performs that work in Day 2. "With external SC" means a compatible SC has subsequently been installed and configured by the administrator; "without SC" means no suitable SC is available.

| Component | Longhorn dependency | With external SC | Without SC |
|---|---|---|---|
| Rancher Monitoring | `harvester-longhorn` PVCs (Prometheus 50 Gi, Alertmanager 5 Gi, Grafana 2 Gi) | Use the external RWO SC | Run ephemerally on kubelet; reserve at least 57 Gi in `COS_PERSISTENT`; data is lost when the Pod restarts |
| CDI | `harvester-longhorn` as the scratch SC | Use the external SC | Reject scratch-dependent operations |
| k3k addon | `harvester-longhorn` SC | Use the external SC | Cannot be enabled |
| `vmstate-persistence` | Longhorn-backed TPM/UEFI state | Use the external SC | State is ephemeral on kubelet and is lost when the VM Pod restarts |
| Upgrade ISO workflow | `longhorn-static` RWX SC | Use an external RWX SC | Block upgrade during preflight |

Longhorn-native backup/restore, snapshots, storage network, and RWX network are unavailable in a deferred cluster.

The guest-cluster CSI driver needs a dedicated compatibility review. It must identify the Longhorn-specific defaults and validations—especially its default StorageClass, snapshot classes, and backup/restore settings—before this feature is implemented. This HEP does not add guest-cluster CSI-driver compatibility for another storage provider; existing Day-2 guest passthrough StorageClass configuration remains the mechanism for supported non-Longhorn host storage.

## Non-Goals

- Automatically installing, configuring, or certifying a third-party CSI driver or StorageClass.
- Selecting, validating, or modifying a CSI driver or StorageClass during installation.
- Making a third-party CSI driver the default StorageClass.
- Providing storage-network or RWX-network functionality without Longhorn.
- A supported Day-2 path to enable Longhorn in a deferred cluster (for example a UI page or editing the `harvester` ManagedChart).
- Handling upgrade cases from existing clusters, including replacing the provisioner of `harvester-longhorn`, `longhorn-static`, or `vmstate-persistence`, removing Longhorn, or migrating existing data.

## Proposal

### User Story

#### Install Harvester without the default Longhorn deployment

A user installs a new Harvester cluster and wants to defer the default Longhorn installation. They select `deferLonghornInstall`, install Harvester without a Longhorn disk or Longhorn resources, and later choose and configure a CSI driver according to their own requirements.

### User Experience

The installer presents a choice during the initial installation:

```text
Default storage
  [x] Install Longhorn (default)
  [ ] Defer Longhorn installation
      No CSI storage is configured during installation.
      Configure a CSI driver after installation.
      Minimum root disk: 250 GiB.
```

When `deferLonghornInstall` is selected:

- The installer requires acknowledgement that storage-dependent functionality will not be available until the administrator performs Day-2 storage configuration.
- `DataDisk` is hidden or disabled, and an automated-install `dataDisk` value is rejected.
- The installer does not create or mount `HARV_LH_DEFAULT`.
- Harvester deploys no Longhorn workloads or StorageClass. The Longhorn CRDs and `longhorn-system` namespace remain.

### Node Join Consistency

The first node records an immutable install profile that contains `deferLonghornInstall`. The profile is written once when the cluster is created and is never updated.

```go
// InstallProfile holds cluster-wide, install-time decisions.
// Written once at cluster creation; immutable afterwards.
type InstallProfile struct {
    DeferLonghornInstall bool `json:"deferLonghornInstall"`
}
```

Profile contains only `deferLonghornInstall` now. It can be extended with other cluster-wide, install-time decisions later.

No Rancherd change is needed. The installer fetches the profile during the join step, before the node's disk is prepared and before the first boot.

The disk page of the interactive installer comes before the network, server URL, and token pages, and a joining node has no Kubernetes API access. The installer therefore fetches the profile with the same authentication that Rancherd uses for `/cacerts`:

- The request carries a random nonce (`X-Cattle-Nonce`) and `Authorization: Bearer base64(sha256(token))`.
- The server answers with `X-Cattle-Hash`, an HMAC-SHA512 keyed with the token over the nonce and the response body.
- The installer recomputes the HMAC with the token the user entered and rejects the response on a mismatch or a missing header.

This proves that the server knows the join token, so a server that does not know the token cannot supply a forged profile.

The installer builds the request URL from the server URL the user entered (for example `https://192.168.1.100`):

```text
GET https://<server>/v1/harvester/installProfile
X-Cattle-Nonce: <random nonce>
Authorization: Bearer <base64(sha256(token))>

200 OK
X-Cattle-Hash: <base64(HMAC-SHA512(token, nonce \0 body \0))>
{"deferLonghornInstall": true}
```

The path is a proposal and sits next to the existing `/v1/harvester/ui-info` route. The installer handles the responses as follows:

| Response | Installer behavior |
|---|---|
| `200` with a valid `X-Cattle-Hash` | Use the profile. |
| `200` with a missing or wrong `X-Cattle-Hash` | Show a token mismatch or tampering error and do not continue. |
| `404` (older cluster without the route) | Treat Longhorn as not deferred and show a warning. |
| Network error or timeout | Retry as the existing `/ping` check does, then show the error. |

The join flow is:

1. The joiner enters the server URL and token on the existing panels.
2. The installer fetches the `InstallProfile` from the cluster and verifies the response as described above.
3. The installer sets `install.deferLonghornInstall` from the `InstallProfile`. The cluster value overrides any local value. If a `DataDisk` was already selected on the disk page, the installer clears it and shows a notice.
4. The installation continues as usual. Partitioning and mounting already read `install.deferLonghornInstall`, and nothing is written to disk until the user confirms the installation.

If the profile cannot be fetched (for example, because the cluster is older and does not serve it), the installer treats Longhorn as not deferred and shows a warning. It does not block the join.

Automated installation (PXE or a config file) is unaffected, because the flag arrives with the configuration.

### API

Add an explicit initial-installation option. An unset value preserves the current behavior.

```go
type Install struct {
    // DeferLonghornInstall defers Longhorn installation for a fresh
    // installation. An empty or false value installs Longhorn as today.
    DeferLonghornInstall bool `json:"deferLonghornInstall,omitempty"`
}
```

```yaml
# Existing/default behavior
install:
  deferLonghornInstall: false

# Fresh installation without a default Longhorn deployment
install:
  deferLonghornInstall: true
```

## Implementation

1. Add `deferLonghornInstall` to the interactive installer and automated-installation configuration. Preserve the current Longhorn deployment as the default.
2. Gate Longhorn manifests and bootstrap logic so this installation path leaves no Longhorn footprint: no StorageClasses.
3. Audit each storage-dependent platform feature and guest-cluster CSI path listed above. Ensure it either remains safely non-persistent or returns an actionable error that says storage must be configured in Day 2; it must not silently assume a Longhorn or cluster-default StorageClass.
4. Record the install profile when the cluster is created. Add the `GET /v1/harvester/installProfile` route to the Harvester server; it returns only the `InstallProfile` and signs the response as described in Node Join Consistency. Add an installer helper that fetches it after the token step, with retries like the existing `/ping` check, and applies it as described in Node Join Consistency.
5. Test the default-install regression, fresh installation with `deferLonghornInstall`, absence of Longhorn workloads and StorageClasses, the 250 GiB minimum, `DataDisk` rejection, and the profile fetch (including fetch failure).
