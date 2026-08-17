# Addon Repository Consolidation and Lifecycle Management

## Summary

Harvester addons currently live in two repositories: `harvester/addons` (the eight
addons packaged into the ISO, rendered from a single monolithic template) and
`harvester/experimental-addons` (manually-applied static manifests with no defined
lifecycle). This split gives addons no clear maturity model, no machine-readable
packaging contract, and no defined path for an addon to graduate into — or retire
from — the product. It also leaves ISO builds non-hermetic: the build clones the
addons repository's branch head at build time, so a harvester commit alone does not
determine the addon content of the resulting ISO.

This enhancement moves all addons into the `harvester/harvester` repository itself,
following the precedent set by the harvester-installer merge. Each addon lives in
its own directory under `addons/` and carries a `metadata.yaml` declaring two
orthogonal properties: its **stage** (`experimental` | `preview` | `ga`, describing
maturity and support commitment) and **builtIn** (`true` | `false`, describing
whether it is packaged into the ISO and managed by upgrades), plus a `deprecated`
flag for retirement. CI enforces the invariants between them.

With addons in-tree, addon changes and product changes land atomically in one PR,
ISO addon content is fully determined by the harvester commit, and non-built-in
manifests are versioned and released together with Harvester — installable from a
raw URL pinned to a release tag.

`harvester/experimental-addons` is archived after the import so existing raw-URL
references keep working. `harvester/addons` stops developing on `main`, while its
release branches (`v1.4`–`v1.9`) keep serving already-released Harvester versions
unchanged until those versions reach EOL.

### Related Issues

https://github.com/harvester/harvester/issues/10649

## Motivation

### Goals

- Single home for all Harvester addons, inside `harvester/harvester`;
  `harvester/experimental-addons` archived (not deleted, preserving existing raw
  manifest URLs) and `harvester/addons` frozen for released branches only.
- Per-addon directories with a `metadata.yaml` that declares stage and packaging,
  replacing the monolithic `pkg/templates/rancherd-22-addons.yaml`.
- A defined lifecycle: stage definitions, promotion flow, and deprecation flow.
- Structural CI guarantees: only `builtIn: true` addons can reach the ISO; stage
  labels are injected from metadata rather than hand-maintained.
- Hermetic ISO builds: remove the build-time network clone of the addons
  repository; the rendered addon content is determined by the harvester commit.
- Release alignment: non-built-in manifests are tagged with every Harvester
  release, so users install the manifest matching their cluster version.
- Rendered ISO content is unchanged by the migration itself.

### Non-goals

- UI treatment of the new `ga`, `preview` and `deprecated` labels (follow-up UI
  work; the existing `experimental` label handling is already in place).
- Merging `version_info` into Harvester's own version machinery. The file moves
  as-is (bash format, sourced by build scripts); unifying it is future work.
- Templating the version/image references of non-builtIn addons. Their manifests
  remain fully rendered static YAML (see Note for a future option).
- Changing how addon charts are served in-cluster (`harvester-cluster-repo`).
- Relocating already-released addon branches: `harvester/addons` `v1.4`–`v1.9`
  continue to serve existing Harvester releases exactly as today.

## Proposal

### Repository layout

Inside `harvester/harvester`:

```
harvester/harvester
├── addons/                          # one directory per addon, all stages
│   ├── vm-import-controller/
│   │   ├── metadata.yaml            # stage: ga, builtIn: true
│   │   ├── addon-template.yaml      # fragment of the rancherd bootstrap template
│   │   └── README.md
│   ├── rancher-vcluster/
│   │   ├── metadata.yaml            # stage: experimental, builtIn: false
│   │   ├── addon.yaml               # static rendered manifest, kubectl-apply-able
│   │   └── README.md
│   ├── ...
│   ├── version_info                 # moved as-is (bash contract for build scripts)
│   └── hack/                        # chart patch/check scripts (was addons scripts/hack)
├── pkg/addons/render/               # generator library (walk, assemble, validate)
└── cmd/addon-generator/             # CLI: -generateTemplates / -generateAddons / -validate
```

The monolithic `pkg/templates/rancherd-22-addons.yaml` of the standalone addons
repository is split into per-addon `addon-template.yaml` fragments; the standalone
repository's generator moves in-tree (exact package paths subject to review).

A key benefit of the per-addon directory is that every addon carries its own
`README.md` describing its current status at a finer granularity than the single
`stage` field: an addon's stage summarizes its overall maturity, but individual
capabilities within it mature at different rates, and the README is where those
details live — e.g. which capabilities are already GA and which are still in
preview, along with known limitations. This gives users and reviewers one obvious
place to check what an addon actually delivers today.

### metadata.yaml

```yaml
name: vm-import-controller
namespace: harvester-system
stage: ga            # experimental | preview | ga
builtIn: true        # decides ISO packaging + upgrade management
deprecated: false    # optional, defaults to false
```

The two main fields are orthogonal:

- **stage** describes maturity and support commitment:
  - `experimental`: the feature is roughly implemented end-to-end, without
    sufficient test-case coverage.
  - `preview`: detailed features are partially implemented, with partial test
    coverage in place.
  - `ga`: all features are implemented with sufficient automated test coverage.
- **builtIn** describes the delivery channel: whether the addon is assembled into
  the ISO (rancherd bootstrap template + upgrade bundle + in-cluster chart repo) or
  installed manually from a static manifest. This is purely a usage-driven
  packaging decision — whether users need the addon available out of the box — and
  is independent of `stage`.
- **deprecated** marks an addon that is no longer maintained. It is separate from
  `stage` (an addon can be retired from any stage) and `stage` retains its
  pre-retirement value for historical clarity.

CI-enforced invariants:

| Invariant | Rationale |
|---|---|
| `deprecated: true` ⇒ `builtIn: false` | Retiring a built-in addon starts by removing it from the ISO. |
| `builtIn: true` ⇒ `addon-template.yaml` exists and all `<< >>` variables resolve from `version_info` | Built-in addons are templated; versions are managed centrally. |
| `builtIn: false` ⇒ `addon.yaml` exists and is valid, fully-rendered YAML | Non-built-in addons must be directly `kubectl apply`-able from the raw URL. |
| `README.md` exists in every addon directory | The README documents per-capability status (which parts are GA, which are still preview) and known limitations, beyond the summary `stage` field. |
| Stage labels match metadata (see below) | Labels are generated, not hand-maintained. |

### Labels

The generator/CI derives labels on the Addon resource from metadata; hand-written
label drift is a CI failure:

- `stage: experimental` → `addon.harvesterhci.io/experimental: "true"` (existing
  label, already recognized by the UI)
- `stage: preview` → `addon.harvesterhci.io/preview: "true"` (new)
- `stage: ga` → `addon.harvesterhci.io/ga: "true"` (new) — every stage carries an
  explicit label so the UI can positively display GA status instead of inferring
  it from the absence of other labels
- `deprecated: true` → `addon.harvesterhci.io/deprecated: "true"` (new)

### Lifecycle flows

**Promotion**

- `experimental` → `preview`: one-line `stage` change once the coverage bar is met.
- `preview` → `ga`: one-line `stage` change. The manifest stays static; the
  installation method (raw URL) is unchanged.

**Becoming built-in** (`builtIn: false` → `true`) is orthogonal to stage promotion
and can happen at any stage, driven by usage considerations. It is the real
engineering step: convert the static manifest into a template fragment, add version
variables to `version_info`, add the `{{ .Addons }}` enabled block, and ensure the
chart is mirrored into the ISO chart bundle. With addons in-tree this conversion is
a single atomic PR.

**Deprecation**

1. A built-in addon: flip `builtIn: true` → `false` **and** set
   `deprecated: true`. Removing the addon from the ISO and the upgrade bundle is
   exactly what retirement means: upgrades stop managing the addon, the Addon CR
   on existing clusters is left untouched, so users can keep running it, but it
   is no longer maintained.
2. A non-built-in addon: set `deprecated: true` (it already has
   `builtIn: false`); the static manifest stays in the repository (raw URLs keep
   working).
3. Documentation formally announces the addon as unsupported.

**Chart availability after deprecating a built-in addon.** The Addon CR of a
built-in addon references the in-cluster chart repository
(`http://harvester-cluster-repo.cattle-system.svc/charts`). After the chart is
dropped from the ISO, any operation that re-fetches the chart (re-sync, reconfigure,
re-enable) would fail. The chart remains published in the external Harvester charts
repository; the deprecation documentation must therefore include:

- a one-time migration step for **all** users who keep the addon: point the Addon
  CR `spec.repo` at the external chart repository;
- an additional step for **air-gapped** users: mirror the chart into a repository
  reachable from the cluster before switching `spec.repo`.

### User Stories

#### Story 1: installing a non-built-in addon

Before: users find experimental addons in a separate, sparsely documented
repository with no indication of maturity beyond a repo-level disclaimer, and no
correlation to the Harvester version they run.

After: all addons live in the harvester repository; `metadata.yaml` states the
stage, and the manifest is applied from a raw URL pinned to the running Harvester
version:

```
kubectl apply -f https://raw.githubusercontent.com/harvester/harvester/<version>/addons/<name>/addon.yaml
```

where `<version>` is the release tag (e.g. `v1.10.0`) or release branch matching
the cluster; documentation for the development version uses `master`.

#### Story 2: changing an addon together with the product

Before: a built-in addon default that depends on a Harvester code change requires
two PRs in two repositories, merged in the right order, and the ISO picks up
whatever the addons branch head happens to be at build time.

After: one atomic PR in `harvester/harvester`; the ISO content is exactly what the
harvester commit says it is.

#### Story 3: promoting an addon

Before: moving an addon from experimental to shipped-in-ISO is an ad-hoc
cross-repository code move with no checklist.

After: stage promotion is a reviewed one-line metadata change; ISO inclusion is a
separate, well-defined conversion in the same repository with CI verifying the
template/version_info contract.

#### Story 4: retiring an addon

Before: no defined process; removal risks breaking running clusters silently.

After: the deprecation flow above, with documented chart-repo migration so existing
users can keep running the addon at their own risk.

### User Experience In Detail

- No change for ISO installation or upgrades: the set of built-in addons and their
  rendered manifests are identical before and after the migration.
- Users of former experimental-addons raw URLs: existing URLs keep working against
  the archived repository (frozen at its final state); documentation for current
  releases is updated to the new `harvester/harvester` URLs.
- The UI continues to badge experimental addons via the existing label; `ga`,
  `preview` and `deprecated` badges are enabled by the new labels as follow-up UI
  work.

### API changes

None to Harvester's APIs. Three new label keys on `addons.harvesterhci.io`
resources: `addon.harvesterhci.io/ga`, `addon.harvesterhci.io/preview`,
`addon.harvesterhci.io/deprecated`.

## Design

### Implementation Overview

**Generator.** The standalone repository's generator moves in-tree as a library
(`pkg/addons/render`) plus a small CLI, keeping the same behaviors:

- Walk `addons/*/metadata.yaml`.
- `-generateTemplates`: assemble the `addon-template.yaml` fragments of all
  `builtIn: true` addons (alphabetical order by addon name) into the rancherd
  bootstrap template and render `<< >>` variables from `addons/version_info`.
- `-generateAddons`: per-addon disabled manifests for the upgrade bundle
  (built-in addons only).
- `-validate`: enforce the invariants table and label derivation. Validation also
  runs automatically before any generation, so invalid metadata can never produce
  ISO or upgrade artifacts.

**Build integration.** The Dockerfile `prepare-addons` stage stops cloning
`github.com/harvester/addons` over the network and consumes the in-tree `addons/`
directory; `scripts/lib/addon` sources `addons/version_info` and the chart patch
scripts from `addons/hack/` instead of a sibling checkout. The
`HARVESTER_ADDONS_VERSION` branch selector is removed on `master` (release branches
of already-shipped versions keep their current behavior).

**CI cost control.** Addon-only changes (paths under `addons/`) skip the heavy ISO
build and test pipeline via path filtering, so chart-bump PRs stay as cheap as they
are in the standalone repository today; generator validation and render checks
always run.

**Migration (single PR to `harvester/harvester`, plus repo disposition).**

1. Split the monolithic template into eight `addons/<name>/` directories
   (fragment + `metadata.yaml` + `README.md`); move `version_info` and the chart
   patch/check scripts; add the generator library and CLI; rewire the Dockerfile
   and build scripts. Rendered output is equivalence-checked (see Test plan).
2. Import the experimental addons from `harvester/experimental-addons` as a single
   commit (no history migration; full history remains browsable in the archived
   repository): `harvester-csi-driver-lvm`, `harvester-upgrade-manager`,
   `harvester-vm-dhcp-controller`, `rancher-k3k`, `rancher-vcluster`,
   `suse-observability-agent` — each with `metadata.yaml`
   (`stage: experimental`, `builtIn: false`).
   - `k3k` is not imported: it lacks the experimental label and README and is
     superseded by `rancher-k3k` (to be confirmed with the addon owner before the
     migration lands).
   - Version references of the Harvester-owned experimental addons are re-aligned
     with the active dev cycle during import.
3. Add the explicit `ga` labels in a separate commit on top of the migration
   (keeps the equivalence gate meaningful).
4. Repo disposition after the migration merges:
   - `harvester/experimental-addons`: archived immediately.
   - `harvester/addons`: `main` stops development and gets a pointer README;
     release branches `v1.4`–`v1.9` continue to serve released Harvester versions
     until EOL, after which the repository is archived.
5. Documentation wave: update current-release docs to the new
   `harvester/harvester` raw URLs. Versioned docs for released versions are frozen
   and keep pointing at the archived/frozen repositories — which is the reason
   those repositories are archived rather than deleted.

### Test plan

- **Migration equivalence test**: render `-generateTemplates` and
  `-generateAddons` from the standalone addons repository (current pipeline) and
  from the in-tree layout, and assert the set of Addon resources is deeply equal
  per resource (resource order within the bootstrap manifest is not significant
  to rancherd). This gate must pass before the migration PR merges. The new
  `addon.harvesterhci.io/ga` label is introduced in a separate commit on top of
  the migration, so the equivalence gate verifies the move itself and the label
  change stays an independently reviewable diff.
- **Invariant checks** (harvester CI on every PR): the metadata invariants table,
  label derivation, `builtIn: false` manifests parse as valid fully-rendered
  YAML, `builtIn: true` fragments render with no missing variables.
- **ISO build check**: an ISO built after the migration is byte-comparable at the
  addon-manifest level to one built before it; existing addon-related integration
  tests cover runtime behavior.

### Upgrade strategy

No upgrade impact from the consolidation itself: rendered built-in addon manifests
are unchanged, so upgrades behave identically. Released Harvester versions keep
building from their existing `harvester/addons` release branches.

For future deprecations of built-in addons, the upgrade stops shipping and managing
the addon; the Addon CR on existing clusters is preserved. The release notes and
documentation for that release must include the chart-repository migration steps
described in the deprecation flow (external `spec.repo` switch; chart mirroring for
air-gapped clusters).

## Note

Follow-up work tracked separately:

- UI badges for the `ga`, `preview` and `deprecated` labels.
- Generating `version_info` from per-addon metadata, and longer-term unifying it
  with Harvester's own version handling now that they share a repository.
- Optional templating for non-built-in addons that want centrally-managed version
  references: allow an addon directory to carry a template fragment alongside its
  committed `addon.yaml`, with CI verifying the committed manifest equals the
  rendered fragment. The raw-URL install path keeps working because the rendered
  manifest stays checked in. Note that a static `addon.yaml` already installs via
  Helm — the Addon CR's `spec.repo`/`chart`/`version` is a Helm source — so this
  is only about version management, not the install mechanism.
