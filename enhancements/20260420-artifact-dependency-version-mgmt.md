# Automated Dependency Updates with Renovate

## Summary

Current Renovate setup doesn't handle dependency updates well. Developers still do a lot of manual work for:

- K8s version bumps (need to coordinate multiple `k8s.io/*` modules), <https://github.com/harvester/harvester/issues/10110>.
- Longhorn updates (spans Helm charts and Go modules), <https://github.com/harvester/harvester/pull/10193>.
- Go version updates (need to touch Dockerfile and `go.mod` together).

This proposes using Renovate with proper grouping rules and automated merge policies. Result: one PR per logical change with auto-merge for patches/security fixes.

### Related Issues

<https://github.com/harvester/harvester/issues/10031>.

## Motivation

### Goals

- Group related dependencies into single PRs (all `k8s.io/*` modules together, not 15 separate PRs).
- Auto-merge patch and security updates after waiting period (2 weeks).
- Require human review for minor updates.
- Apply minor/patch/security updates to all active branches (master/main trunk + active release branches).
- Restrict release branches to patch/security updates only.
- Centralize update policies via shared Renovate config in `harvester/dependency-automation`.

### Non-goals

- Auto-merge minor updates (require human review).
- Major version updates (disabled, require manual updates).

## Introduction

Use Renovate to automate dependency updates with proper grouping and auto-merge policies.

**Update policies**:
- **Patch/security updates**: Auto-merge after 2-week waiting period if CI passes
- **Minor updates**: Create PR, require human review
- **Major updates**: Disabled (require manual updates)

**Shared config**: Write rules once in `harvester/dependency-automation`, reuse via `extends` in each repo.

## Proposal

1. **Grouping rules** - Group related dependencies (k8s.io/*, longhorn/*, golang toolchain) into single PRs
2. **Auto-merge policies** - Patch/security updates auto-merge after 2 weeks; minor updates require human review
3. **Shared configuration** - Centralize in `harvester/dependency-automation`, extend in each repo

### User Stories

#### K8s Dependency Updates

**Before**: K8s patch release → 15+ separate Renovate PRs for each `k8s.io/*` module due to incomplete Renovate settings → devs manually group them (e.g., [issue #10110](https://github.com/harvester/harvester/issues/10110)) → manually verify compatibility → create grouped PR by hand.

**After**: One PR with all `k8s.io/*` modules grouped, e.g. <https://github.com/pohanhuang/harvester/pull/157>.

#### Go Version Updates

**Before**: Update Go 1.25.7 → 1.25.8 requires touching:

- `.github/workflows/*.yml`
- `go.mod`
- Easy to miss a file or have inconsistent versions.

**After**: Renovate's `go-version` datasource handles it in one PR. All files updated together using regex managers.

#### Longhorn Go Module Updates

**Before**: Longhorn release → manually update multiple Go modules (e.g., [fork PR #209](https://github.com/pohanhuang/harvester/pull/209) touching `longhorn-manager`, `longhorn-instance-manager`, etc.) → manual version verification across modules.

**After**: One PR with all Longhorn Go modules grouped and updated together.

### User Experience In Detail

**Setup** (one-time per repo):

- Update `renovate.json` to extend the shared config from `harvester/dependency-automation`.
- Per-repo customizations can override shared settings as needed.

**Daily usage**:

- Patch/security updates: Auto-merged after waiting period (e.g., 2 weeks) and CI passes.
- Minor updates: Review PRs as they come in, titles like "chore: update k8s deps to v1.33.0".
- Approve/merge after CI passes.

No changes for contributors.

### API changes

None.

## Design

### Implementation Overview

**Phase 1: Create shared config in `harvester/dependency-automation`**

Repository `harvester/dependency-automation` (renamed from `harvester/renovate`) hosts shared config.

**Dependency grouping**:
- Golang toolchain (go.mod + Dockerfile) → 1 PR
- K8s + Rancher (all k8s.io/* + rancher/rancher) → 1 PR
- Longhorn (all longhorn/*) → 1 PR
- SUSE BCI base images → 1 PR

**Update policies**:
- Major updates: Disabled for Go modules
- Minor updates: PR created, require human review
- Patch/security updates: Auto-merge after 2 weeks if CI passes

**Disabled dependencies** (require manual updates):
- kubevirt (breaking changes need testing)
- rancher/lasso (version pinned)
- Helm charts (separate management)

**Phase 2: Apply to `harvester/harvester` (master → v1.x branches)**

Create PR to extend shared config in `harvester/harvester`:
- Master branch adopts shared config first
- Apply to active release branches (v1.x) for patch/security updates only
- Monitor PR quality and tune grouping rules

**Phase 3: Gradually migrate to other repos**

Gradually rollout to remaining repos:
- Start with 1-2 repos per week
- Each repo: `renovate.json` extends `github>harvester/dependency-automation`
- Monitor PR quality and adjust shared config based on feedback
- Repo-specific overrides allowed

### Test plan

**Success criteria**:
- Grouping: k8s.io/* in 1 PR, longhorn/* in 1 PR, golang toolchain in 1 PR
- Auto-merge: Patch updates wait 14 days → auto-merge if CI passes
- Manual review: Minor updates require approval
- `make ci` passes after bumps

### Upgrade strategy

Dev infrastructure only—no end-user impact.

**Migration per repo**:

1. Update `renovate.json` to extend `github>harvester/dependency-automation`
2. Remove any local grouping rules that are now in shared config
3. Add repo-specific overrides if needed
4. Existing Renovate setup continues until new config is merged (no disruption)

## Note

Renovate handles current requirements. Updatecli will be used if needed in the future.
