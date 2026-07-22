# Image and Template Catalog AddOn

## Summary

Add a new optional Harvester AddOn — `harvester-catalog-addon` — that provides a curated, community-maintained catalog of cloud-init-ready Linux images and pre-defined VM templates in three t-shirt sizes (small, medium, large). When the AddOn is enabled, the Harvester Dashboard shows two new sections:

- **Images → Catalog** — a card grid of curated OS images (Ubuntu, Fedora, Debian, CentOS Stream, AlmaLinux, Rocky Linux, openSUSE Leap / Tumbleweed / MicroOS). One click downloads the image into the user's chosen namespace with a deterministic name.
- **Virtual Machines → From Catalog** — a new tab alongside the existing *Single Instance* / *Multiple Instance* creation flows. Users pick an OS, pick a size, name the VM, and get a running, cloud-init-configured VM without editing YAML or hunting for image URLs.

The AddOn is fully optional and ships with sensible defaults. Existing Harvester behavior is unchanged when the AddOn is disabled.

### Related Issues

- https://github.com/harvester/harvester/issues/11101

## Motivation

Today, provisioning a new VM in Harvester requires several manual steps that assume domain knowledge the user often does not have on day one:

1. Locate a cloud-init-ready cloud image URL for the desired distro. Discovering the "right" URL is non-trivial — many distros publish multiple variants (Cloud, GenericCloud, Minimal, JeOS, ISO), and only some ship cloud-init pre-installed.
2. Create the `VirtualMachineImage` from that URL and wait for Longhorn to import it.
3. Manually pick CPU, memory, disk, and network configuration for the VM.
4. Write cloud-init user-data to bootstrap SSH access.

Users who spin up VMs frequently repeat this dance for every cluster and every namespace, often maintaining private notes with the "known good" URLs. Newcomers routinely end up with images that lack cloud-init, non-bootable images (wrong architecture, wrong format), or VMs sized incorrectly for their workload.

Peer projects in the KubeVirt ecosystem (`kubevirt/common-templates`, `kubevirt-ui/kubevirt-plugin`, CDI `DataImportCron`) address this with a curated template + auto-boot-source model. Harvester currently offers no equivalent.

### Goals

- Ship a curated, community-maintained list of popular Linux images with verified download URLs and cloud-init support.
- Provide pre-defined t-shirt-sized VM templates per OS so users can go from "I want Ubuntu" to "running VM" in two clicks.
- Package everything as an optional Harvester `Addon` — no changes to core Harvester behavior.
- Add discoverable UI surfaces (Images → Catalog, Virtual Machines → From Catalog) that appear only when the AddOn is enabled, using the same `registerAddonSideNav` pattern the VM Import controller uses.
- Provide a stable, deterministic identifier for each catalog image so pre-provisioned templates reliably resolve their base image after download.
- Provide a CLI surface (via `harvester-cli`) for scripted / GitOps consumption of the same catalog.

### Non-goals

- Not a replacement for the existing `Images → Create` flow. Users can still upload arbitrary images from URL or file.
- Not an offline/air-gapped image mirror. The catalog metadata (names, URLs, icons) is shipped with the AddOn, but image bytes are still downloaded from upstream sources by Harvester nodes at request time.
- Not an automatic image updater in the initial release. When a distro publishes a new build, users re-download the catalog entry to refresh; a `DataImportCron`-style auto-refresh may follow in a later HEP.
- Not opinionated about which OSes belong in the catalog. Additions and removals are community-driven via PRs to the AddOn repo.

## Proposal

### User Stories

#### Story 1: First-time Harvester user provisions a VM

**Before:** A new user installs Harvester, wants to try a VM. They open the Dashboard, click *Virtual Machines → Create*, and are presented with a form that requires them to select an image. There are no images. They open a second browser tab to find an Ubuntu cloud image URL, discover that "cloud" images ship in multiple variants (server/minimal/cloud), pick one, copy the URL, go to *Images → Create*, paste, wait for the download, return to *Virtual Machines*, size the VM by guesswork, hand-write cloud-init user-data for an SSH key, and finally create the VM. First-run experience: ~15 minutes with three browser tabs open.

**After:** The user enables the `harvester-catalog-addon` from *Advanced → Addons*. Navigates to *Virtual Machines → From Catalog*. Sees a card grid: Ubuntu 24.04, Fedora 44, Rocky Linux 10.2, openSUSE Leap 16.0, .... Clicks Ubuntu 24.04 → picks *Medium* → gives the VM a name → clicks *Create*. VM boots. First-run experience: under 60 seconds.

#### Story 2: Platform team standardizes on curated images

**Before:** Every team in the organization maintains its own list of "good" cloud image URLs. Some are stale (pointing to EOL builds), some point to variants without cloud-init, some point to non-x86_64 builds. Support tickets result.

**After:** The platform team enables the AddOn once per cluster. The organization gets a single, verified list of images. When a new distro release lands upstream, one PR to the AddOn repo updates the catalog for everyone.

#### Story 3: Sensible defaults for VM sizing

**Before:** Users often mis-size VMs (2 GB RAM for a desktop that needs 4, 20 GB disk for a workload that grows past 50). Sizing is invisible until things break.

**After:** T-shirt sizes (small / medium / large) per OS encode community-recommended minimums. Users who need custom sizes still have the regular *Create → Single Instance* flow available.

### User Experience In Detail

**Enabling the AddOn:**
1. Dashboard → Advanced → Addons.
2. Locate `harvester-catalog-addon` (shipped as disabled by default, following the same convention as `vm-import-controller`, `pcidevices-controller`, etc.).
3. Click *Enable*. The Addon controller reconciles the Helm chart, installs CRDs, and seeds the initial catalog entries.
4. New sections appear in the left navigation within a few seconds.

**Downloading a catalog image:**
1. Navigate to *Images → Catalog*.
2. Browse the card grid; each card shows the OS icon, display name, version, approximate size, and a *Download* action.
3. Click *Download*. A modal prompts for target namespace and StorageClass (with graceful text-input fallback when the caller lacks cluster-wide `list` permissions, as is common with Rancher-proxied kubeconfigs).
4. Confirm. A `VirtualMachineImage` is created with `metadata.name: catalog-<image-key>` in the chosen namespace. Standard Harvester image-import progress is visible in *Images*.

**Creating a VM from the catalog:**
1. Navigate to *Virtual Machines*. A third tab, *From Catalog*, joins *Single Instance* and *Multiple Instance*.
2. The card grid mirrors the image catalog. Click a card.
3. A modal prompts for: t-shirt size (small / medium / large), VM name, target namespace, StorageClass, and optional cloud-init overrides (SSH key, hostname).
4. If the base image is not yet present in the target namespace, an inline banner offers *Download and create* which performs both steps.
5. Confirm. A `VirtualMachine` is instantiated from the corresponding `TemplateCatalogEntry`, referencing the deterministic image name.

**CLI parity:**
- `harvester template catalog list [os]` — lists available catalog templates, optionally filtered by OS.
- `harvester template catalog create <os>/<size> --name NAME [--namespace NS] [--storage-class SC]` — instantiates a VM from a catalog template, transparently downloading the base image if missing.

The CLI operates against the same `TemplateCatalogEntry` and `ImageCatalogEntry` CRs the UI consumes.

### API changes

Two new CRDs under a new group, `catalog.harvesterhci.io/v1beta1`. No changes to existing Harvester CRDs.

```yaml
apiVersion: catalog.harvesterhci.io/v1beta1
kind: ImageCatalogEntry
metadata:
  name: ubuntu-24-04                                # deterministic key, drives the downloaded image name
spec:
  displayName: "Ubuntu 24.04 LTS (Noble Numbat)"
  os: ubuntu
  osVersion: "24.04"
  osFamily: linux
  icon: icon-ubuntu
  description: "Long-term support server image with cloud-init pre-installed."
  sourceURL: "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
  checksumURL: "https://cloud-images.ubuntu.com/noble/current/SHA256SUMS"
  cloudInitReady: true
  approxSizeGiB: 4
status:
  installedNamespaces: [default, dev]                # namespaces where the corresponding VirtualMachineImage exists
```

```yaml
apiVersion: catalog.harvesterhci.io/v1beta1
kind: TemplateCatalogEntry
metadata:
  name: ubuntu-24-04-medium
spec:
  imageRef: ubuntu-24-04                             # matches ImageCatalogEntry.metadata.name
  displayName: "Ubuntu 24.04 (Medium)"
  size: medium                                       # small | medium | large
  workload: server                                   # server | desktop | highperformance
  resources:
    cpu: 2
    memoryGiB: 4
    diskGiB: 40
  defaultCloudInit: |
    #cloud-config
    ssh_pwauth: false
    package_update: true
```

Deterministic image identity is the load-bearing design decision. When a user "downloads" an `ImageCatalogEntry` into namespace `X`, the controller creates a `VirtualMachineImage` with `metadata.name: catalog-<entry-key>` in `X` (using an explicit `name` rather than `generateName`). Because `VirtualMachineImage` is namespaced, the same catalog key can coexist in multiple namespaces. `TemplateCatalogEntry` references the image by this deterministic name, so templates resolve reliably in whatever namespace the user chooses at VM creation time.

## Design

### Implementation Overview

**Repository layout** (proposed new repo, `harvester/harvester-catalog-addon`):

```
harvester-catalog-addon/
├── charts/harvester-catalog-addon/                  # Helm chart, same shape as vm-import-controller
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
│       ├── crds/                                    # ImageCatalogEntry, TemplateCatalogEntry
│       ├── deployment.yaml                          # the controller
│       ├── rbac.yaml
│       ├── serviceaccount.yaml
│       └── seed/                                    # initial catalog entries as manifests
│           ├── images-ubuntu.yaml
│           ├── images-fedora.yaml
│           └── templates-*.yaml
├── pkg/apis/catalog.harvesterhci.io/v1beta1/        # CRD Go types (wrangler-generated)
├── pkg/controllers/
│   ├── imagecatalogentry_controller.go              # reconciles Download requests → VirtualMachineImage
│   └── templatecatalogentry_controller.go           # reconciles Create-VM-from-catalog
├── main.go
└── package/Dockerfile
```

The Helm chart is published to the standard `harvester-cluster-repo` service so a stock `Addon` CR (`spec.repo: http://harvester-cluster-repo.cattle-system.svc/charts`) can install it — matching every other shipped Harvester AddOn.

**Reconciliation contract** for `ImageCatalogEntry`:
- The CR itself is a static description; it does not trigger a download by existing.
- Downloads are triggered by the UI/CLI creating a lightweight `ImageCatalogInstall` sub-resource (or an annotation on the CR: `catalog.harvesterhci.io/install-in-namespaces: default,dev`). The controller reconciles this by creating one `VirtualMachineImage/catalog-<entry-key>` per requested namespace with `spec.url` and `spec.checksum` copied from the entry, and `spec.sourceType: download`.

**VM creation from `TemplateCatalogEntry`:**
- MVP does not introduce a Harvester `VirtualMachineTemplateVersion` object. The UI/CLI reads the `TemplateCatalogEntry`, substitutes user-supplied fields (name, namespace, SSH key, cloud-init overrides), and directly creates a `VirtualMachine`.
- The generated `VirtualMachine` references `catalog-<entry-key>` in its data-volume template, which is guaranteed to exist because the UI checks and offers to download first.

**Dashboard UI integration** (in `harvester/harvester-ui-extension`):
- Add resource types `HCI.CATALOG_IMAGE` and `HCI.CATALOG_TEMPLATE` to `pkg/harvester/types.ts`.
- Add `ADD_ONS.CATALOG` to `pkg/harvester/config/harvester-map.js`.
- In `pkg/harvester/config/harvester-cluster.js`, define a `weightGroup('catalog', …, false)` initially hidden, then call:
  ```js
  registerAddonSideNav(store, PRODUCT_NAME, {
    addonName:    ADD_ONS.CATALOG,                   // 'harvester-catalog-addon'
    resourceType: HCI.ADD_ONS,
    navGroup:     'catalog',
    types:        [HCI.CATALOG_IMAGE, HCI.CATALOG_TEMPLATE],
  });
  ```
  This is the identical mechanism used by `vm-import-controller` — the UI section appears only when the Addon CR exists and is enabled.
- New Vue components: `pkg/harvester/list/catalog.harvesterhci.io.imagecatalogentry.vue`, `pkg/harvester/list/catalog.harvesterhci.io.templatecatalogentry.vue`, plus a new *From Catalog* tab on the VM create page.
- L10n keys under `harvester.addons.catalog.*` in `pkg/harvester/l10n/en-us.yaml`.

**Initial catalog contents** (mirrors the already-verified list from `abonillabeeche/harvester-cli` `image-metadata.json`):
- Ubuntu 25.10, 25.04, 24.10, 24.04 LTS, 22.04 LTS, 20.04 LTS
- Fedora Cloud 44, 43, 42
- Debian 13 (trixie), 12 (bookworm)
- CentOS Stream 10, 9
- AlmaLinux 10, 9, 8
- Rocky Linux 10.2, 10.1, 9.8, 9.7, 8.10, 8.9
- openSUSE Leap 16.0, 15.6, 15.5, 15.4
- openSUSE Tumbleweed (rolling)
- openSUSE MicroOS (rolling)

All URLs verified to return HTTP 206 Partial Content on a 1 MiB range GET at the time of writing. A verification script accompanying the AddOn re-runs this check on every release.

### Test plan

1. Install the AddOn on a fresh Harvester test cluster. Verify CRDs install and the initial seed data appears as `ImageCatalogEntry` / `TemplateCatalogEntry` objects.
2. Verify *Images → Catalog* and *Virtual Machines → From Catalog* sections are visible in the Dashboard.
3. Download `ubuntu-24-04` into the `default` namespace. Verify `VirtualMachineImage/catalog-ubuntu-24-04` is created and reaches `Ready`.
4. Create a VM from *Virtual Machines → From Catalog* using Ubuntu 24.04 → Medium. Verify the VM boots with 2 CPU / 4 GB / 40 GB, cloud-init runs, SSH is reachable with the supplied key.
5. Download the same catalog entry into a second namespace. Verify a second `VirtualMachineImage` is created and does not conflict with the first.
6. Disable the AddOn. Verify the two UI sections disappear. Verify existing catalog-derived `VirtualMachine` and `VirtualMachineImage` resources remain intact.
7. Re-enable the AddOn. Verify UI sections reappear and existing catalog-derived resources are still enumerated.
8. In a namespace where the caller lacks cluster-wide `list` permission (Rancher-proxied kubeconfig), verify text-input fallback prompts for namespace and StorageClass.
9. Bad-source path: point an `ImageCatalogEntry.spec.sourceURL` at an unreachable host, request a download, verify a clear user-facing error surfaces on the CR status and in the Dashboard.
10. Upgrade path: install AddOn v0.1.0, then upgrade to a hypothetical v0.2.0 that adds a new `ImageCatalogEntry`. Verify the new entry is present and pre-existing entries and user data are untouched.

### Upgrade strategy

- The AddOn is optional and additive. Clusters that never enable it are unaffected.
- Upgrading the AddOn's Helm chart re-applies the seed manifests; user-modified `ImageCatalogEntry` fields are preserved via server-side apply semantics, and new entries are added.
- Disabling the AddOn stops the controller but leaves CRDs and existing `VirtualMachineImage` / `VirtualMachine` objects in place — no data loss.
- Uninstalling the AddOn requires manual CRD removal (`kubectl delete crd imagecatalogentries.catalog.harvesterhci.io templatecatalogentries.catalog.harvesterhci.io`) if the operator wants a clean slate.

## Note

- Design inspiration draws on open-source KubeVirt ecosystem patterns: `DataSource` indirection in `kubevirt/common-templates`, `VirtualMachineClusterInstancetype` and `VirtualMachineClusterPreference` from KubeVirt Instance Types, and the boot-source auto-import model in `kubevirt-ui/kubevirt-plugin`. This proposal deliberately opts for a simpler MVP (deterministic image names, pre-cloned t-shirt templates) and leaves the instance-type-based redesign for a follow-up HEP once Harvester's KubeVirt version fully supports the newer APIs.
- The initial curated list is contributed from the community-maintained catalog at [`abonillabeeche/harvester-cli/image-metadata.json`](https://github.com/abonillabeeche/harvester-cli/blob/main/image-metadata.json), which is currently used by the `harvester image catalog` CLI subcommands and includes the verification script that will be reused in this AddOn's release pipeline.
- Future work — out of scope for this HEP but tracked as follow-ups: `DataImportCron`-style auto-refresh of catalog images, ARM64 catalog entries, migration of t-shirt sizes to `VirtualMachineClusterInstancetype` + `VirtualMachineClusterPreference`, and community-contributed OS icon packs.
