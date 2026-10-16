# Defer Longhorn Installation for Fresh Clusters

## Summary

Harvester currently installs Longhorn on every cluster. The installer either creates the `HARV_LH_DEFAULT` partition on the OS disk or formats a separate `DataDisk`, and the Harvester chart deploys Longhorn and its default `harvester-longhorn` StorageClass. This makes the current 500 GiB root-disk requirement necessary even for users who do not intend to use Longhorn.

This feature is for **fresh installations only**. It adds an install-time `deferLonghornInstall` option that defers the default Longhorn installation and reduces the minimum root-disk size from 500 GiB to 250 GiB. **It does not configure another CSI driver**. CSI selection, installation, configuration, and the decision to install Longhorn later are Day-2 operations owned by the administrator.

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
  - Do not deploy Longhorn StorageClasses such as `harvester-longhorn`, `longhorn-static`, and `vmstate-persistence`.
  - Clearly tell the administrator that no default CSI storage is available after installation and that storage-dependent features require Day-2 configuration.
- Support the option in both the interactive installer and automated installation configuration.
- Keep the Longhorn package available as a Day-2 option; disabling its default installation must not preclude an administrator from installing and configuring Longhorn later.
- Persist the initial installation choice as cluster configuration, and enforce the same choice when a node joins the cluster.

### Effects of deferring the Longhorn installation

The following table describes the behavior after a fresh installation with `deferLonghornInstall`. It does **not** make the installer configure an external CSI driver or StorageClass: the administrator performs that work in Day 2. "With external SC" means a compatible SC has subsequently been installed and configured by the administrator; "without SC" means no suitable SC is available.

| Component | Longhorn dependency | With external SC | Without SC |
|---|---|---|---|
| Rancher Monitoring | `harvester-longhorn` PVCs (Prometheus 50 Gi, Alertmanager 5 Gi, Grafana 2 Gi) | Use the external RWO SC | Run ephemerally on kubelet; reserve at least 57 Gi in `COS_PERSISTENT`; data is lost when the Pod restarts |
| CDI | `harvester-longhorn` as the scratch SC | Use the external SC | Reject scratch-dependent operations |
| k3k addon | `harvester-longhorn` SC | Use the external SC | Cannot be enabled |
| `vmstate-persistence` | Longhorn-backed TPM/UEFI state | Use the external SC | State is ephemeral on kubelet and is lost when the VM Pod restarts |
| Upgrade ISO workflow | `longhorn-static` RWX SC | Use an external RWX SC | Block upgrade during preflight |

Longhorn-native backup/restore, snapshots, storage network, and RWX network remain unavailable until the administrator installs and configures Longhorn in Day 2.

The guest-cluster CSI driver needs a dedicated compatibility review. It must identify the Longhorn-specific defaults and validations—especially its default StorageClass, snapshot classes, and backup/restore settings—before this feature is implemented. This HEP does not add guest-cluster CSI-driver compatibility for another storage provider; existing Day-2 guest passthrough StorageClass configuration remains the mechanism for supported non-Longhorn host storage.

## Non-Goals

- Automatically installing, configuring, or certifying a third-party CSI driver or StorageClass.
- Selecting, validating, or modifying a CSI driver or StorageClass during installation.
- Making a third-party CSI driver the default StorageClass.
- Providing storage-network or RWX-network functionality without Longhorn.
- Handling upgrade cases from existing clusters, including replacing the provisioner of `harvester-longhorn`, `longhorn-static`, or `vmstate-persistence`, removing Longhorn, or migrating existing data.

## Proposal

### User Story

#### Install Harvester without the default Longhorn deployment

A user installs a new Harvester cluster and wants to defer the default Longhorn installation. They select `deferLonghornInstall`, install Harvester without a Longhorn disk or Longhorn resources, and later choose and configure storage—either Longhorn or another CSI driver—according to their own requirements.

### User Experience

The installer presents a choice during the initial installation:

```text
Default storage
  [x] Install Longhorn (default)
  [ ] Defer Longhorn installation
      No CSI storage is configured during installation.
      Configure Longhorn or another CSI driver after installation.
      Minimum root disk: 250 GiB.
```

When `deferLonghornInstall` is selected:

- The installer requires acknowledgement that storage-dependent functionality will not be available until the administrator performs Day-2 storage configuration.
- `DataDisk` is hidden or disabled, and an automated-install `dataDisk` value is rejected.
- The installer does not create or mount `HARV_LH_DEFAULT`.
- Harvester deploys no Longhorn StorageClass.

### Node Join Consistency

The initial installation records `deferLonghornInstall` as immutable cluster configuration. When adding a node, the installer retrieves this configuration from the joining cluster and uses it as the required Longhorn/disk selection. The interactive installer displays the required value; automated installation validates its supplied value against the cluster setting.

The join must fail with an actionable error if the supplied node configuration conflicts with the cluster setting. In particular, a node must not create or mount `HARV_LH_DEFAULT` or accept `DataDisk` when joining a cluster whose initial installation deferred Longhorn.

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
4. Persist the initial `deferLonghornInstall` value as immutable cluster configuration. During node join, retrieve it, populate the required installer selection, and reject a conflicting interactive or automated configuration before disk preparation begins.
5. Test the default-install regression, fresh installation with `deferLonghornInstall`, absence of all Longhorn resources, the 250 GiB minimum, `DataDisk` rejection, matching and mismatched node joins, and Day-2 installation of Longhorn.

## Open Question
1. Do we need to remove all of the Longhorn CR, CRD, Namespace?
