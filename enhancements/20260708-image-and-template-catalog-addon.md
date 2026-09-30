# Native KubeVirt-Driven VM Catalog AddOn

## Summary

Add an optional Harvester AddOn — `vm-catalog` — that provides a streamlined, cloud-like VM creation workflow directly from the main sidebar. Rather than introducing proprietary Harvester catalog CRDs or custom controller daemons, the VM Catalog builds natively on upstream KubeVirt APIs already bundled with Harvester (`VirtualMachineClusterInstancetype` and `VirtualMachineClusterPreference` from KubeVirt's `common-instancetypes` bundle).

Specification resolution is delegated server-side to KubeVirt's native `expand-vm-spec` subresource API, producing standard Harvester VMs that are 100% compatible with existing backup, restore, migration, and admission webhooks.

The AddOn is fully optional and opt-in via **Advanced > Addons**. When enabled, a top-level **Catalog** entry appears in the main navigation sidebar. Harvester behavior is completely unchanged when the AddOn is disabled.

### Related Issues

- https://github.com/harvester/harvester/issues/11101

## Motivation

Today, provisioning a new VM in Harvester requires several manual steps that assume domain knowledge the user often does not have on day one:

1. Locate a cloud-init-ready cloud image URL for the desired distro. Discovering the "right" URL is non-trivial — many distros publish multiple variants (Cloud, GenericCloud, Minimal, JeOS, ISO), and only some ship cloud-init pre-installed.
2. Create the `VirtualMachineImage` from that URL and wait for Longhorn to import it.
3. Manually pick CPU, memory, disk, and network configuration for the VM.
4. Write cloud-init user-data to bootstrap SSH access.

Users who spin up VMs frequently repeat this dance for every cluster and every namespace, often maintaining private notes with "known good" URLs. Newcomers routinely end up with images that lack cloud-init, non-bootable images (wrong architecture, wrong format), or VMs sized incorrectly for their workload.

Harvester already deploys KubeVirt's `common-instancetypes` bundle, providing:
- Standardized compute sizing across series (`u1` General Purpose, `o1` Memory-Optimized, `cx1` Compute-Optimized, `d1` Dedicated, `m1`, `n1`, `rt1`).
- Curated OS preferences defining optimal disk bus, firmware (BIOS vs. UEFI), network model, and OS icons for over 50 Linux and Windows operating systems.

By building the catalog directly on these native upstream APIs instead of introducing proprietary catalog CRDs and custom controllers, Harvester can deliver a modern, cloud-like provisioning experience with zero ongoing controller maintenance overhead.

### Goals

- Deliver a fast, 3-step VM provisioning flow (OS Tile → Size Matrix → Basic Details) under a top-level **Catalog** sidebar item.
- Leverage native upstream KubeVirt APIs (`VirtualMachineClusterInstancetype` and `VirtualMachineClusterPreference`) without introducing new CRDs or controller maintenance overhead.
- Resolve instancetype and preference references server-side via KubeVirt's `expand-vm-spec` API, ensuring produced VMs work with existing Harvester admission webhooks, live migration, and backup/restore.
- Package as a lightweight AddOn (`vm-catalog`) providing RBAC permissions for non-admin users.
- Support both Linux and Windows golden images, in both connected and air-gapped/offline clusters.
- Provide an interactive "Preview spec" dialog comparing requested references against expanded KubeVirt domains.
- Provide full automated UI and navigation tests in Cypress.

### Non-goals

- Not introducing proprietary Harvester catalog CRDs (`ImageCatalogEntry`, `TemplateCatalogEntry`) or custom catalog reconciliation controllers.
- Not replacing the existing advanced VM creation/edit form (`edit/kubevirt.io.virtualmachine`). Users who require advanced hardware passthrough, complex disk layouts, or custom cloud-init configs continue to use the standard form.
- Not an image downloading daemon in the MVP. The catalog surfaces available bootable `VirtualMachineImage` resources on the cluster, mapping them dynamically to matching preferences.

## Proposal

### User Stories

#### Story 1: First-time Harvester user provisions a VM

**Before:** A new user installs Harvester and wants to try a VM. They open the Dashboard, click *Virtual Machines → Create*, and are presented with a complex form requiring CPU, memory, disk types, network bindings, and guest OS settings. First-run experience: ~15 minutes with multiple tabs open to search for recommended parameters.

**After:** The user enables the `vm-catalog` AddOn from *Advanced → Addons*. Clicks **Catalog** in the left sidebar. Sees an OS tile grid: openSUSE, Ubuntu, Fedora, Debian, Windows, ... Clicks an OS tile → picks an instance size (`u1.medium`, 1 vCPU / 4 GiB) → enters a VM name (or keeps the auto-suggested name) → clicks *Create*. VM boots. First-run experience: under 60 seconds.

#### Story 2: Platform team standardizes sizing and OS defaults

**Before:** Different teams configure VMs with arbitrary, unstandardized CPU and memory values, complicating capacity planning and leading to frequent memory pressure or over-provisioning.

**After:** Platform teams direct users to the Catalog. Sizing conforms to upstream KubeVirt standard series (`u1`, `cx1`, `o1`), with recommended memory-to-vCPU ratios. Operating system preferences ensure correct bus drivers (`virtio`), clock configurations, and firmware settings are applied consistently.

#### Story 3: Air-Gapped and Enterprise Deployments

**Before:** Catalog designs that hardcode external internet URLs fail in air-gapped or restricted networks where nodes cannot reach external mirrors.

**After:** The VM Catalog dynamically inspects existing `VirtualMachineImage` resources in the cluster and maps them to cluster preferences. In air-gapped environments, images pre-staged via Hauler or local registries immediately populate the Catalog tiles without external internet access.

### User Experience In Detail

**Enabling the AddOn:**
1. Dashboard → Advanced → Addons.
2. Locate `vm-catalog` (disabled by default).
3. Click *Enable*. The AddOn only gates the UI; no RBAC objects are needed (see [API & RBAC Changes](#api--rbac-changes)).
4. The **Catalog** item appears directly in the main left sidebar under root.

**Catalog Provisioning Flow:**
1. Navigate to **Catalog** in the left sidebar.
2. **Step 1 (Operating system):**
   - Displays a grid of OS tiles. Each tile shows the operating system logo, display name, number of matching images, and associated preference.
   - If multiple images match an OS (e.g., Ubuntu 22.04 and Ubuntu 24.04), a dropdown allows selecting the specific image.
3. **Step 2 (Size):**
   - Organizes compute options into series tabs: `u1` (General Purpose), `o1` (Memory Optimized), `cx1` (Compute Optimized), `d1` (Dedicated), `m1`, `n1`, `rt1`.
   - Each size card displays vCPU, RAM, tier badge (micro, small, medium, large, etc.), and proportional linear bars visualizing resource allocation.
   - Defaults to common sizes with a "Show all sizes" toggle.
4. **Step 3 (Details):**
   - **Name:** Auto-suggested based on the chosen OS (e.g., `opensuse-vm-a1b2`), with real-time RFC 1123 DNS validation.
   - **Namespace:** Target namespace selector.
   - **Network:** Network attachment definition selector (defaults to Management Network).
   - **Root disk:** Sized in GiB with validation enforcing that size cannot be smaller than the image's virtual size.
   - **Credentials:** Multi-select for existing SSH keys and optional console password injected via cloud-init.
   - **Start after creation:** Checkbox controlling `spec.runStrategy` (`RerunOnFailure` vs `Halted`).
5. **Preview spec (Optional):**
   - Clicking **Preview spec** triggers a server-side call to KubeVirt's `expand-vm-spec` subresource.
   - Displays a side-by-side comparison:
     - *You ask for:* The lightweight VM definition with `spec.instancetype` and `spec.preference` references.
     - *KubeVirt expands to:* The fully populated `domain` spec showing resolved CPU topology, memory limits, devices, and firmware.
6. **Create:**
   - Clicking **Create** expands and shims the VM spec, creates the `VirtualMachine` in Harvester, and redirects the user to the VM detail page.

## Design

### Architectural Pattern: Server-Side Spec Expansion

KubeVirt supports creating VMs that reference instancetypes directly (`spec.instancetype` / `spec.preference`). However, Harvester's existing admission webhooks enforce that:
1. `spec.template.spec.domain` must have explicit `resources.limits.memory` or `memory.guest` for memory overcommit calculations.
2. A mutating webhook executes a JSON-patch `replace` on `/spec/template/spec/domain/cpu/maxSockets`, which fails if `domain.cpu` is omitted because it is delegated to an instancetype.

Verified on Harvester v1.9.0 (KubeVirt 1.8.4): a dry-run create of a reference-mode VM (`u1.medium` + `opensuse.leap`) is rejected with `either memory.guest or resources.limits.memory must be set`.

To resolve this without destabilizing existing webhooks, the Catalog employs the **Expand-on-Create** pattern:

```
+-------------------------------------------------------------+
| 1. buildCatalogVm()                                         |
|    Constructs VM referencing instancetype & preference      |
+-------------------------------------------------------------+
                              │
                              ▼
+-------------------------------------------------------------+
| 2. PUT /apis/subresources.kubevirt.io/v1/namespaces/<ns>/   |
|        expand-vm-spec                                       |
|    KubeVirt resolves instancetype sizing, CPU topology,     |
|    bus, model, and firmware server-side                     |
+-------------------------------------------------------------+
                              │
                              ▼
+-------------------------------------------------------------+
| 3. harvesterShim()                                          |
|    Pins cpu.maxSockets = cpu.sockets;                       |
|    Sets resources.limits for Harvester overcommit           |
+-------------------------------------------------------------+
                              │
                              ▼
+-------------------------------------------------------------+
| 4. harvester/create + save()                                |
|    Applies standard VM to Harvester (passes all webhooks)   |
+-------------------------------------------------------------+
```

**Why the shim (step 3) is needed:** the expanded spec from step 2 already passes Harvester's admission webhooks on its own, since it carries `memory.guest` and an explicit `domain.cpu`. But it has `resources: {}`, and Harvester's mutator only applies CPU/memory overcommit to `requests` when `resources.limits` is set. Without the shim, catalog VMs would silently skip the overcommit settings that UI-created VMs get. With the shim, the result matches UI-created VMs. For example, `u1.medium` on the default overcommit config (cpu 1000%, memory 150%) gets limits `cpu: 1` / `memory: 4Gi` and mutated requests `cpu: 100m` / `memory: 2730Mi`.

This guarantees that:
- Created VMs are standard, fully-expanded Harvester VMs.
- Live migration, backups, snapshots, and clone operations work out of the box.
- When Harvester admission webhooks are updated in the future to natively support instancetype references, the flow can switch to reference mode simply by bypassing the expansion step.

### Image-to-Preference Mapping

Images are mapped dynamically to OS tiles using a prioritized heuristic:

1. **Explicit Preference Label:** Image label `instancetype.kubevirt.io/default-preference: <preference-name>`.
2. **Harvester OS Type Label:** Harvester's built-in `harvesterhci.io/os-type` label (`sles`, `openSUSE`, `ubuntu`, `redhat`, `windows`, etc.) mapped to matching preference prefixes.
3. **Display Name Matching:** Heuristic matching against image display name tokens (e.g., `leap`, `tumbleweed`, `ubuntu-24`, `win2k22`).
4. **Fallback:** If no preference matches, the image is placed under the **Other** tile.

The default size is resolved from `instancetype.kubevirt.io/default-instancetype` on the image, falling back to `u1.medium`.

### Provenance Metadata

To maintain visibility into which instancetype and preference were used to create the VM, non-functional metadata annotations are recorded on the VM:

```yaml
metadata:
  annotations:
    catalog.harvesterhci.io/instancetype: u1.medium
    catalog.harvesterhci.io/preference: ubuntu
    catalog.harvesterhci.io/image: default/ubuntu-24.04
```

These annotations avoid reserved KubeVirt prefixes (`kubevirt.io/*`, `instancetype.kubevirt.io/*`) so controllers do not attempt conflicting reconciliations.

### API & RBAC Changes

**Zero new CRDs.** 

**Zero new RBAC.** KubeVirt already ships every permission the catalog needs:

| Catalog call | RBAC attributes checked by the API server | Already granted by |
|---|---|---|
| `PUT /apis/subresources.kubevirt.io/v1/namespaces/<ns>/expand-vm-spec` | group `subresources.kubevirt.io`, resource `expand-vm-spec`, verb `update` (namespaced, no subresource) | `kubevirt.io:edit` / `kubevirt.io:admin`, which aggregate into `edit` / `admin` |
| `GET .../virtualmachines/<name>/expand-spec` (preview of an existing VM) | resource `virtualmachines`, subresource `expand-spec`, verb `get` | `kubevirt.io:edit` / `kubevirt.io:view` |
| List `virtualmachineclusterinstancetypes` / `virtualmachineclusterpreferences` | cluster-scoped `get`/`list`/`watch` | `instancetype.kubevirt.io:view`, bound to `system:authenticated` by virt-operator |
| Create `VirtualMachine`, list `VirtualMachineImage` | existing Harvester VM permissions | `edit` (Rancher *Project Member* inherits `edit`, *Project Owner* inherits `admin`) |

A custom ClusterRole granting `virtualmachines/expand-vm-spec` would match nothing: the namespaced endpoint is authorized as resource `expand-vm-spec` with no subresource. Aggregating to `view` would also give read-only users a write-verb endpoint. Separately, rules aggregated into `edit` cannot grant the cluster-scoped instancetype resources through a project RoleBinding, and they are not needed. The AddOn therefore ships no RBAC objects. Its only job is to toggle the UI.

**Open question for reviewers:** with no RBAC to deploy, the `vm-catalog` AddOn chart is effectively empty. Is an AddOn still the right on/off switch (it is consistent with `vm-import-controller` side-nav gating), or should this be a Harvester setting or a UI feature flag instead?

Verified on Harvester v1.9.0 (KubeVirt 1.8.4) with a ServiceAccount whose only permission is a namespace RoleBinding to `edit`:
- `expand-vm-spec` in its namespace returns `200`.
- `expand-vm-spec` in another namespace returns `403` (`cannot update resource "expand-vm-spec" in API group "subresources.kubevirt.io"`).
- Listing cluster instancetypes and preferences is allowed.
- A dry-run create of the expanded and shimmed VM returns `201`.

### Dashboard UI Integration (`harvester-ui-extension`)

- Registered as a top-level route: `${PRODUCT_NAME}-c-cluster-catalog` at `/:product/c/:cluster/catalog`.
- Registered via `registerAddonSideNav()` so the **Catalog** entry appears in the main navigation sidebar only when the `vm-catalog` AddOn is enabled. Direct URL navigation displays a friendly warning banner linking to the Addons page when disabled.
- Fully supports both light and dark theme using Rancher Shell CSS custom properties.

## Test Plan

### Automated Cypress E2E Tests (`harvester-ui-tests`)

The automated test suite `testcases/virtualmachines/catalog.spec.ts` exercises:
1. **Sidebar Navigation:** Verifies Catalog entry appears in the navigation and routes to the wizard.
2. **Initial State & Waiting Steps:** Verifies steps 2 and 3 remain in waiting/disabled state until prerequisites are met.
3. **OS Distro Selection:** Verifies selecting an OS tile auto-suggests a valid RFC 1123 VM name.
4. **Instance Type Series & Sizes:** Verifies switching series tabs (`u1`, `o1`, `cx1`) and selecting size tiers.
5. **Form Field Validations:** Verifies DNS naming constraints and root disk minimum size validation (must be >= image virtual size).
6. **Spec Preview:** Verifies "Preview spec" button calls `expand-vm-spec` and displays requested references alongside the expanded domain.
7. **Toggle Controls:** Verifies the "Start after creation" toggle updates summary and runStrategy.

### Manual / QA Verification

1. **AddOn Enable/Disable:**
   - Enable `vm-catalog` in **Advanced > Addons**; verify **Catalog** appears in the sidebar.
   - Disable `vm-catalog`; verify **Catalog** disappears from the sidebar and direct URL access is blocked.
2. **Non-Admin User RBAC:**
   - Log in as a standard user with Project Member / Edit permissions.
   - Open Catalog, select OS, click "Preview spec", and create a VM.
   - Verify non-admin user can execute `expand-vm-spec` without permission errors.
   - Verify a user with only *Read-only* (`view`) access gets `403` on `expand-vm-spec`, and that the Catalog UI disables *Create* rather than failing late.
   - Verify a Project Member of project A gets `403` on `expand-vm-spec` in project B's namespaces.
3. **VM Lifecycle:**
   - Verify created VMs can be started, stopped, cloned, snapshotted, and live-migrated across nodes.

## Upgrade Strategy

- Upgrades to existing clusters automatically register the `vm-catalog` AddOn via `upgrade_manifests.sh` in the `upgrade_addons()` function.
- The AddOn is disabled by default on upgrade; existing workloads and behaviors are 100% unaffected.
