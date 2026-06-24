# Automated Dependency Updates with Renovate

## Summary

Current Renovate setup doesn't handle dependency updates well. Developers still do a lot of manual work for:

- K8s version bumps (need to coordinate multiple `k8s.io/*` modules), <https://github.com/harvester/harvester/issues/10110>.
- Longhorn updates (spans Helm charts and Go modules), <https://github.com/harvester/harvester/pull/10193>.
- Go version updates (need to touch Dockerfile and `go.mod` together).

This proposes using Renovate with proper grouping rules and automated merge policies for the initial set of supported dependencies. Result: one PR per logical change with auto-merge for patches/security fixes. Longhorn remains manually managed for now.

### Related Issues

<https://github.com/harvester/harvester/issues/10031>.

## Motivation

### Goals

- Centralize Renovate policy in `harvester/dependency-automation` so Harvester repositories share the same defaults.
- Enable security-focused dependency updates using Renovate's vulnerability alert support.
- Group related dependency updates into a single PR when a security fix is needed.
- Keep risky or operationally sensitive dependency families under manual control.
- Allow repositories to opt into additional branch coverage or local overrides when needed.

### Non-goals

- Auto-merge minor updates (require human review).
- Major version updates (disabled, require manual updates).
- Automate Longhorn dependency updates in the initial rollout.

## Introduction

Harvester now consumes a shared Renovate configuration from `harvester/dependency-automation` through:

```json
{
  "extends": [
    "github>harvester/dependency-automation:renovate"
  ]
}
```

The shared configuration is security-oriented rather than upgrade-oriented.

**Update policies**:

- **Security updates**: enabled by default and require manual review.
- **Non-security patch/minor/major updates**: disabled by default.
- **Major Go updates**: explicitly disabled.

**Shared config defaults**:

- `osvVulnerabilityAlerts` enabled.
- Group related dependencies such as Golang toolchain, SUSE BCI base images, and K8s plus Rancher dependencies.
- Exclude risky or manually managed updates such as Longhorn, KubeVirt, Helm, and selected pinned dependencies.

**Branch policy**:

- `main` and `master`: normal shared policy applies.
- Release branches matching `v*`: not scanned by default.
- A repository can opt release branches in with `baseBranchPatterns`; when it does, only patch and security updates are allowed there.

## Proposal

1. **Shared configuration**: keep Renovate rules centralized in `harvester/dependency-automation` and extend them from each repository.
2. **Security-first update policy**: create PRs only for known vulnerable dependencies by default.
3. **Dependency grouping**: reduce review overhead by grouping logically related updates into one PR.
4. **Manual handling for sensitive dependencies**: keep excluded ecosystems under explicit human control.

### User Stories

#### K8s Dependency Updates

**Before**: A vulnerable K8s dependency requires developers to identify all affected `k8s.io/*` modules, update them consistently, and verify compatibility manually.

**After**: Renovate opens one grouped PR for the affected K8s and related dependencies when a vulnerability-backed update is available.

#### Go Toolchain Updates

**Before**: Updating Go often means touching `go.mod`, workflow files, and container build files separately, with a risk of inconsistent versions.

**After**: When the shared configuration allows a relevant security-driven update, Renovate can group the matching Go-related file changes into one PR.

#### Longhorn Dependency Updates

**Before**: Longhorn releases require coordinated updates across several modules and charts with manual compatibility verification.

**After**: No change in the shared default policy. Longhorn remains manually managed until a safer update strategy is defined.

### User Experience In Detail

**Setup** (one-time per repo):

- Update `renovate.json` to extend the shared config from `harvester/dependency-automation`.
- Per-repo customizations can override shared settings as needed.

**Daily usage**:

- Most repositories receive no routine version bump PRs.
- Vulnerability-backed updates appear as grouped Renovate PRs for manual review.
- Maintainers continue to handle excluded dependencies manually.

No contributor workflow changes are required outside normal PR review.

### API changes

None.

## Design

### Implementation Overview

**Shared config repository**

`harvester/dependency-automation` is the single source of truth for shared Renovate policy.

**Supported managers**:

- `gomod`
- `dockerfile`
- `github-actions`
- `pip_requirements`

**Grouping rules**:

- Golang toolchain related updates
- SUSE BCI base images
- K8s plus Rancher dependencies

**Default update policies**:

- Security updates only
- Non-security patch updates disabled
- Non-security minor updates disabled
- Non-security major updates disabled

**Disabled or manually managed dependency areas**:

- Longhorn
- KubeVirt
- Helm-related updates
- Go major version bumps
- Other pinned or operationally sensitive dependencies defined in the shared config

**Harvester repository adoption**

`harvester/harvester` consumes the shared config through its root `renovate.json`. Release branches can opt in separately if patch and security coverage is required there.

### Test plan

**Success criteria**:

- Shared configuration can be consumed from `harvester/dependency-automation`.
- Vulnerability-backed dependency updates are grouped into coherent PRs.
- Non-security update noise is eliminated by default.
- Excluded dependency families do not generate unintended Renovate PRs.
- Harvester CI still validates the resulting dependency change PRs.

### Upgrade strategy

This affects developer workflow only and has no direct end-user impact.

**Migration per repository**:

1. Add or update `renovate.json` to extend `github>harvester/dependency-automation:renovate`.
2. Remove local rules that duplicate shared behavior.
3. Add repository-specific overrides only where justified.
4. Optionally opt release branches in with `baseBranchPatterns` if patch and security coverage is needed there.

## Note

Renovate covers the current security-focused dependency automation requirements. Other tooling such as Updatecli can be evaluated later if Harvester needs broader artifact management workflows.
