# Centralized Image Content Library for Multiple Harvester Clusters

## Summary

Operators running more than one Harvester cluster have no central place to publish VM
images. Every cluster downloads or uploads its own copy, versions drift between
clusters, and there is no way to say "this team may consume these images on these
clusters". vSphere solves this with the Content Library; Harvester has no equivalent.

This HEP proposes a **Content Library**: a versioned image repository that lives
outside any single Harvester cluster, that clusters **subscribe** to, and whose content
is cached in each cluster's existing image store (`VirtualMachineImage`). Permissions
are expressed once (per library, per group) and enforced through per-cluster, read-only
credentials. Rancher, when present, is the cross-cluster control plane for libraries and
subscriptions; standalone Harvester clusters can subscribe directly.

The HEP deliberately presents **backend options** (S3, OCI registry such as Harbor /
SUSE Private Registry, Artifactory) with a recommendation, because the choice drives
most of the implementation cost. Reviewer input on that choice is the main ask of this
draft.

### Related Issues

- https://github.com/harvester/harvester/issues/423 — Centralized image repository for multiple Harvester clusters
- https://github.com/harvester/harvester/pull/11763 — VM Catalog AddOn (complementary: the catalog *consumes* images already on a cluster and lists an image downloading daemon as a non-goal; the Content Library is the distribution layer that *feeds* it)

## Motivation

- An administrator wants one place to store golden images (qcow2, raw, ISO) and use the
  same, identical images on many Harvester clusters, including remote/edge clusters with
  slow or intermittent links.
- A provider wants to publish images to its own tenants, with tenants only able to see
  the libraries they were granted.
- Users want to create VMs from a known-good, versioned image without knowing where it
  came from, and want new versions to roll out without long downtime.

Today the only options are per-cluster `download` (plain HTTP, no authentication),
`upload`, or GitOps (Fleet) distributing `VirtualMachineImage` manifests that each
download the image separately. None provide versioning, subscription, or permissions.

### Goals

- A library abstraction holding versioned image items (qcow2, raw, ISO), identified by an
  immutable digest and human-readable version tags.
- Clusters subscribe to a library (or a subset of items); new versions are imported
  automatically or on demand, and are stored as regular `VirtualMachineImage` objects
  (the local image store is the cache — a VM never reads from the remote library).
- Retention: keep the last *N* versions per item per cluster, garbage-collect the rest
  when no VM/template references them.
- Permissions: library admin / publisher / consumer roles mapped to identity-provider
  groups; each cluster receives its own pull-only credential scoped to the libraries it
  subscribes to (a compromised dev cluster cannot read prod libraries or push content).
- Manage items (upload, new version, delete, sync) from the UI and the CLI.
- Publish a cluster-local image or VM volume into a library.
- Work for both image backends (`backingimage`/Longhorn v1 and `cdi`).
- Work for edge sites: a per-site replica/cache and tolerance to interrupted transfers.

### Non-goals

- Re-imaging *running* VMs when a new image version is published. Existing VMs keep
  their cloned disks; only new VMs use the new version.
- Replacing the existing image sources (`download`, `upload`, ...). They remain.
- Shipping or operating the object store / registry itself. The library uses
  customer-provided storage (or SUSE Private Registry).
- VM template / OVF items in the first iteration (see Phase 4).

## Proposal

### User Stories

#### Story 1: Publish once, consume on every cluster

**Before:** An administrator builds a hardened SLES image and has to upload it to each
of 12 clusters. Months later the clusters run four different "SLES golden" builds.

**After:** The administrator pushes `sles16:2026.10` to the `golden` library. All 12
clusters subscribed to `golden` import it, and it appears in each cluster's image list
as `golden/sles16 (2026.10)`. Every cluster has the same digest.

#### Story 2: Tenant-scoped libraries

**Before:** A provider has no way to give tenant A images that tenant B cannot see.

**After:** The provider creates libraries `tenant-a` and `tenant-b`, grants the tenant
groups the *consumer* role, and subscribes each tenant's clusters/namespaces only to
their library. Each cluster gets a pull-only credential for exactly those libraries.

#### Story 3: Rolling out a new image version

**Before:** Updating an image means deleting and re-creating it, breaking references in
VM templates.

**After:** A publisher pushes `win2025:2026.11`. Subscribed clusters import it in the
background while `2026.10` stays available. Templates that reference the stable item
name (`golden/win2025`) start using the new version once the import completes; existing
VMs are untouched. The oldest version is garbage-collected when unused.

#### Story 4: Edge site with a slow link

**Before:** A 40 GiB Windows image download fails halfway over a WAN link and restarts
from zero, on every cluster at that site.

**After:** The site has a library replica/cache. The image crosses the WAN once, and
local clusters import from the replica.

### User Experience In Detail

Rancher (multi-cluster):

1. *Virtualization Management → Content Libraries → Create*: name, backend endpoint,
   credentials, visibility.
2. *Members*: assign groups as Library Admin / Publisher / Consumer.
3. *Subscriptions*: select Harvester clusters (and target namespace/StorageClass per
   cluster), mode `automatic` or `on-demand`, versions to keep.
4. Per-cluster sync status (imported, importing, failed, bytes transferred) is rolled up
   on the library page.

Harvester (per cluster):

1. *Images* shows library items alongside local images, with a library badge and
   version; *Create VM* defaults to library images when subscriptions exist.
2. *Images → Libraries* (standalone mode): add a subscription with an endpoint and pull
   secret, without Rancher.
3. *Image → Publish to library* (publisher role): pushes a local image or volume as a new
   item version.

CLI: the same objects are plain CRs (`kubectl apply`), plus the backend's own tooling for
uploads (e.g. `oras push`, `aws s3 cp`).

### API changes

Harvester (per cluster), indicative:

```yaml
# New image source type, usable on its own
apiVersion: harvesterhci.io/v1beta1
kind: VirtualMachineImage
spec:
  sourceType: library            # new
  library:
    url: oci://registry.example.com/golden/sles16@sha256:...   # or s3://bucket/golden/sles16/2026.10.qcow2
    secretRef: {name: golden-pull}
---
# Subscription: watches a library item and materializes versions as VirtualMachineImages
apiVersion: harvesterhci.io/v1beta1
kind: ImageSubscription
metadata: {name: golden-sles16, namespace: images}
spec:
  url: oci://registry.example.com/golden/sles16
  secretRef: {name: golden-pull}
  mode: automatic                # automatic | on-demand
  versionsToKeep: 2
  storageClassName: harvester-longhorn
status:
  current: {version: "2026.10", digest: sha256:..., image: images/golden-sles16-2026-10}
  versions: [...]
```

Rancher (management cluster), indicative: `ContentLibrary` (endpoint, credentials,
members) and `ContentLibrarySubscription` (library, target clusters/namespaces, mode).
Rancher renders them into per-cluster `ImageSubscription` objects plus a per-cluster
pull secret.

## Design

### Backend options (input requested)

Note that registries such as Harbor and Artifactory usually store blobs in S3 anyway.
The real question is which API sits in front of the bytes: a raw object API or an
artifact-management API.

| | **S3** (AWS, MinIO, Ceph RGW) | **OCI registry** (Harbor / SUSE Private Registry) | **Artifactory** |
|---|---|---|---|
| Very large files (50–500 GiB) | Best: objects up to 5 TiB, parallel multipart upload, file stored as-is | Works with care: disk becomes one large layer; avoid gzip (`oras`, uncompressed); ingress body-size/timeout tuning on push | Good (generic repositories) |
| Byte-level resume | Native (HTTP Range) | Per-blob Range, client-dependent | Native (generic) |
| Versioning / stable "latest" | Opaque object version IDs; index format must be invented | Tags + immutable digests, tag immutability, retention policies | Good |
| Subscribe / auto-update in-cluster | Must build a poller | CDI `DataImportCron` already does it | OCI repositories only |
| Per-cluster scoped, pull-only credential | Possible via bucket policies / OIDC STS; vendor-specific | Robot accounts per project, pull-only, with expiry | Scoped access tokens |
| Group RBAC with the same IdP as Rancher | DIY | OIDC/LDAP group → project role | LDAP/SAML/OIDC |
| Edge replica / cache | Bucket replication | Replication rules + proxy-cache projects | Federated repositories |
| Signing / audit | No / partial | cosign / notation, audit log | Yes |
| Supported by SUSE | Customer-provided | SUSE Private Registry | No (third party, commercial) |

**Summary:**

- Raw large-object mechanics favor **S3**.
- Library semantics (versions, retention, subscriptions) and multi-cluster permissions
  favor an **OCI registry**. Building them on S3 would mean re-implementing a large part
  of a registry.

**Recommendation:** design the Harvester side against a small backend interface
(resolve item → versions → digest, open a stream), implement the **OCI registry
backend first** (Harbor / SUSE Private Registry; can itself be S3-backed), and an
**S3 backend second** for customers who only have object storage. Artifactory works
through its OCI repositories with the first backend.

### Implementation Overview

Building blocks that already exist:

- `VirtualMachineImage` with `backingimage` and `cdi` backends; sources `download`,
  `upload`, `export-from-volume`, `restore`, `clone`.
- CDI is bundled with Harvester and can import from `http`, `s3` and `registry`
  (containerDisk) sources. `DataImportCron` + `DataSource` poll a registry tag, import
  new digests, keep *N* versions and repoint a stable name.

New work:

1. **`library` source type (cdi backend).** The image controller creates a CDI
   `DataVolume` with a `registry` or `s3` source and the referenced pull secret.
2. **`library` source type (backingimage backend).** Longhorn backing images only
   download over plain HTTP. Options to decide:
   (a) CDI imports into a temporary PVC, then the existing `export-from-volume` path
   creates the backing image (no Longhorn change, extra copy and temporary capacity);
   (b) teach the backing-image manager to pull OCI/S3 directly.
3. **`ImageSubscription` controller.** For the OCI backend, this wraps `DataImportCron`
   and maps each imported version to a `VirtualMachineImage` (labels: library, item,
   version, digest). For the S3 backend, the same controller polls an index object.
   Retention deletes versions not referenced by VMs, templates or snapshots.
4. **Stable references for templates.** `VirtualMachineTemplateVersion` can reference
   `library: golden/sles16` (resolved to the current version at VM creation) in
   addition to a fixed image name, so templates are portable across clusters.
5. **Publish.** A job exports a volume/image (`export-from-volume` or KubeVirt
   `VirtualMachineExport`) and pushes it to the backend (`oras push` for OCI, multipart
   upload for S3). CDI only pulls, so this is net new.
6. **Rancher integration.** `ContentLibrary` / `ContentLibrarySubscription` CRDs in the
   management cluster. A controller creates per-cluster pull credentials (Harbor robot
   accounts via API, or STS/scoped keys for S3), and distributes `ImageSubscription` +
   secret to downstream clusters (Fleet or the existing Harvester cluster agent). Rancher
   roles map to library roles; status is aggregated back.
7. **UI** (`harvester-ui-extension`, Rancher dashboard): library list/members/
   subscriptions, library badge and version picker on images, "Publish to library", and
   *Create VM* defaulting to library images.

### Phasing

- **Phase 0 (no code, documented pattern):** Fleet distributes `VirtualMachineImage`
  (`download`) and template manifests to many clusters. Gives consistency, not versioning
  or permissions.
- **Phase 1:** `library` source type on the `cdi` backend (OCI), then the backingimage
  bridge.
- **Phase 2:** `ImageSubscription` with retention; stable template references.
- **Phase 3:** Rancher `ContentLibrary`, RBAC mapping, per-cluster credentials, UI,
  publish.
- **Phase 4:** VM template / OVF items stored as OCI artifacts in the same library; S3
  backend.

### Security considerations

- Per-cluster, pull-only, expiring credentials. Clusters never hold push credentials;
  publishing runs with the publisher's own identity.
- Items are referenced and verified by digest; optional signature verification (cosign)
  before import.
- Libraries for different environments (dev/prod) or tenants are separate
  projects/buckets, so a compromised cluster cannot read other libraries.

### Test plan

- Import qcow2, raw and ISO items from an OCI registry and from S3, on the `cdi` and
  `backingimage` backends; verify the digest matches.
- Subscription: push a new version → automatic import → template resolves to the new
  version → retention removes the oldest version only when unreferenced.
- Interrupted transfer (network cut mid-import) for large items; measure whether the
  import resumes or restarts (open question, see notes).
- RBAC: a consumer cannot push; cluster A's credential cannot read library B; credential
  expiry and rotation.
- Rancher: subscribe 2+ clusters, aggregated status, unsubscribe cleans up secrets but
  leaves imported images in use.
- Air-gapped: a site replica / proxy cache serves imports with no WAN access.

### Upgrade strategy

Purely additive: new source type and new CRDs. Existing images and VMs are unaffected.
Rancher CRDs are only installed when the feature is enabled.

## Note

Open questions for reviewers:

1. Is an OCI registry (Harbor / SUSE Private Registry) acceptable as the primary
   backend, or must "bring your own S3" come first?
2. Must the library work without Rancher (standalone subscription), or is Rancher the
   only control plane?
3. Edge: is a per-site replica/cache sufficient, or is byte-level resume inside the
   cluster import required? (CDI's behavior on interrupted large imports needs to be
   measured for both `s3` and `registry` sources.)
4. Backingimage backend: CDI import + `export-from-volume` bridge, or native OCI/S3
   pulls in Longhorn's backing-image manager?
5. Should template/OVF items be in scope for the first release?
6. Relationship with the VM Catalog AddOn (#11763): should the catalog list library
   items directly, or only images already imported on the cluster?
