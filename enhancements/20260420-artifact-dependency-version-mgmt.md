# Automated Dependency Updates with Renovate + Updatecli

## Summary

Current Renovate setup doesn't handle complex dependency updates well. Developers still do a lot of manual work for:

- K8s version bumps (need to coordinate multiple `k8s.io/*` modules), <https://github.com/harvester/harvester/issues/10110>.
- Longhorn updates (spans Helm charts and Go modules), <https://github.com/harvester/harvester/pull/10193>.
- Go version updates (need to touch `Dockerfile.dapper`, CI workflows, and `go.mod` together).

This proposes using Renovate with better grouping rules + Updatecli for complex multi-file updates. Result: one PR per logical change instead of scattered manual updates.

### Related Issues

<https://github.com/harvester/harvester/issues/10031>.

## Motivation

### Goals

- Group related dependencies into single PRs (all `k8s.io/*` modules together, not 15 separate PRs).
- Handle multi-file updates that Renovate can't do (Go version in `Dockerfile.dapper` + CI + `go.mod`), e.g. <https://github.com/pohanhuang/harvester-installer/pull/28>.
- Centralize update policies - write once, reuse across repos, <https://github.com/pohanhuang/harvester-updatecli-poc>.

### Non-goals

- Disable auto-merge (still need human review).

## Introduction

**Updatecli** automates multi-file updates across different formats (Go modules, Dockerfiles, YAML) in a single atomic change.

**Policies** are reusable update configurations packaged as OCI artifacts (e.g., `ghcr.io/harvester/updatecli-policies:latest`). Write update logic once, reuse across repos.

**Workflow**:

1. Policy maintainers publish update logic to `ghcr.io/harvester/updatecli-policies/{policy}:v1.0.0`
2. Each repo adds `updatecli/updatecli-compose.yaml` that references the policy and provides repo-specific values.
3. GitHub Actions runs `updatecli compose apply` on schedule (e.g., weekly)
4. Updatecli pulls the policy, detects version changes, and creates a PR updating all affected files together (e.g., `Dockerfile.dapper` + `go.mod` + CI workflows for Go bumps), e.g. <https://github.com/pohanhuang/harvester/pull/226>.

## Proposal

Use two tools to achieve artifact automation bumping:

1. **Renovate** - for Go modules with better grouping.
2. **Updatecli** - for multi-file or complex updates Renovate can't handle.

### User Stories

#### K8s Dependency Updates

**Before**: K8s patch release → 15+ separate Renovate PRs for each `k8s.io/*` module → devs figure out how to manually group them (e.g., [issue #10110](https://github.com/harvester/harvester/issues/10110)) → manually verify compatibility → create grouped PR by hand.

**After**: One PR with all `k8s.io/*` modules grouped, e.g. <https://github.com/pohanhuang/harvester/pull/157>.

#### Go Version Updates

**Before**: Update Go 1.25.7 → 1.25.8 requires touching:

- `Dockerfile.dapper`
- .github/workflows/*.yml
- `go.mod`
- Easy to miss a file or have inconsistent versions.

**After**: Updatecli does it in one PR. All files updated together, example PR e.g. <https://github.com/pohanhuang/harvester/pull/226>.

#### Longhorn Go Module Updates

**Before**: Longhorn release → manually update multiple Go modules (e.g., [fork PR #209](https://github.com/pohanhuang/harvester/pull/209) touching `longhorn-manager`, `longhorn-instance-manager`, etc.) → manual version verification across modules.

**After**: One PR with all Longhorn Go modules grouped and updated together.

### User Experience In Detail

**Setup** (one-time per repo):

- Add `updatecli/` config files.
- Update `renovate.json` to extend the centralized config.
- Add GitHub Actions workflow for Updatecli.

**Daily usage**:

- Review PRs as they come in
- Titles like "chore: update k8s deps to v1.33.8"
- Approve/merge after CI passes

No changes for contributors.

### API changes

None.

## Design

### Implementation Overview

**Phase 1: Fix Renovate grouping**

Rename `harvester/renovate` → `harvester/dependency-automation` to reflect broader scope (Renovate + Updatecli).

Adopt grouping rules from [PR #5](https://github.com/harvester/renovate/pull/5), which groups related dependencies to reduce PR noise:

- **Group 1**: k8s + kubevirt + rancher (these share compatibility constraints and update together)
- **Group 2**: longhorn (independent update cycle, needs separate k8s compatibility check)

This reduces 15+ separate PRs per k8s release → 2 PRs total.

```json
{
  "packageRules": [
    {
      "description": "Group k8s, kubevirt and rancher packages",
      "matchManagers": ["gomod"],
      "matchPackagePatterns": [
        "k8s\\.io/", 
        "^kubevirt\\.io/",
        "^github\\.com/rancher/rancher$"
      ],
      "groupName": "k8s + kubevirt + rancher dependencies",
      "prBodyNotes": [
        "⚠️ **Check longhorn compatibility before merging.**"
      ]
    },
    {
      "description": "Group longhorn packages",
      "matchManagers": ["gomod"],
      "matchPackagePatterns": ["^github\\.com/longhorn/"],
      "groupName": "longhorn dependencies",
      "prBodyNotes": [
        "⚠️ **Check k8s compatibility before merging.**"
      ]
    }
  ]
}
```

**Phase 2: Create Updatecli policies**

Develop policies in `harvester/dependency-automation/policies/` for multi-file updates Renovate can't handle:

- **Go version**: Updates `Dockerfile.dapper` + `.github/workflows/*.yml` + `go.mod` together (see [example PR](https://github.com/pohanhuang/harvester/pull/226))

**Phase 3: Package and distribute policies**

Publish policies as OCI artifacts to `ghcr.io/harvester/dependency-automation/{policy}:{version}` using GitHub Actions in the `dependency-automation` repo. Repos pull the latest policies at runtime, so updates to policy logic propagate automatically.

**Phase 4: Enable in target repos**

Each repo adds minimal configuration:

```yaml
# .github/workflows/updatecli.yml
name: Updatecli

on:
  workflow_dispatch:
  schedule:
    - cron: "0 8 * * *"

jobs:
  updatecli:
    name: Bump golang version
    runs-on: ubuntu-latest
    permissions:
      contents: write
      pull-requests: write
    steps:
      - name: Checkout
        uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6.0.2

      - name: Install Updatecli
        uses: updatecli/updatecli-action@2cc8e6d8e356d76b0280cdd03766c36596a0614e # v3.0.0

      - name: Diff (dry-run on PR)
        if: github.ref != 'refs/heads/master'
        env:
          GITHUB_TOKEN: ${{ secrets.HARVESTER_TOKEN }}
        run: |
          updatecli compose diff --file updatecli/updatecli-compose.yaml

      - name: Apply
        if: github.ref == 'refs/heads/master'
        env:
          GITHUB_TOKEN: ${{ secrets.HARVESTER_TOKEN }} # Will replace this with Github Apps to autoroates the token.
        run: |
          updatecli compose apply --file updatecli/updatecli-compose.yaml
```

### Test plan

**Unit testing**:

- Run `updatecli diff` on each policy before publishing to verify it detects updates correctly.
- Test against sample repos to ensure file patterns match, see current result, e.g. <https://github.com/pohanhuang/harvester/pull/226>.

**Integration testing**:

Pilot repos: `harvester/harvester-installer`.

Success criteria:

- PRs created with correct file updates (no missed files, no extra files)
- No merge conflicts with concurrent development
- CI passes on generated PRs
- Manual review confirms version consistency across updated files

**Rollout schedule**:

- **Week 1-2**: Enable on pilot repos, monitor PR quality
- **Week 3-4**: Fix issues, tune grouping rules and policies based on feedback
- **Week 5+**: Gradual rollout to remaining repos (1-2 repos per week)

### Upgrade strategy

Dev infrastructure only—no end-user impact.

**Migration per repo**:

1. Add `updatecli/` directory with compose configuration
2. Update `renovate.json` to extend `harvester/dependency-automation:default.json`
3. Add `.github/workflows/updatecli.yml`
4. Existing Renovate setup continues until new config is merged (no disruption)

## Note

**Future enhancements**:

- Automated backports to release branches (e.g., `v1.8`, `v1.7.1`)
