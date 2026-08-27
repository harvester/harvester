---
description: |
  This workflow assesses the risk profile of a component upgrade when a pull
  request is opened to bump the version of a key component. It categorizes the
  commits in the component's changelog and release notes, and reports the risk 
  profile in the pull request comment to help maintainers make informed decisions
  about whether to proceed with the upgrade.

on:
  slash_command:
    name: upgrade-risk
    events: [pull_request_comment, pull_request_review_comment]
  roles: [admin, maintainer]
  reaction: eyes

permissions:
  contents: read
  copilot-requests: write
  issues: read
  pull-requests: read
  security-events: read

tools:
  github:
    toolsets: [default, code_security, security_advisories]
  web-fetch:

network:
  allowed:
    - defaults
    - github

safe-outputs:
  add-comment:
    max: 5
  noop:
    report-as-issue: false

model: claude-sonnet-4.6
engine:
  id: copilot
timeout-minutes: 10
---

# Upgrade Risk

Perform an upgrade risk assessment review on a pull request when a maintainer invokes `/upgrade-risk` in a PR conversation comment or inline review comment. This is a manually triggered workflow, and it is not automatically triggered on PR open or update. It does not respond to `/upgrade-risk` placed in the PR description body. Because it is triggered via comment events on the base repository, it runs in the base-repo context with full secrets and write access, so it works on PRs from forks as well as same-repo PRs.

If you decide no review action is appropriate, call the `noop` tool with a message explaining why.

## Goals

Helps maintainers make informed decisions about whether to proceed with a component upgrade. When a pull request is opened to upgrade a key component, generate a risk profile of the upgrade by categorizing the commits in the component's changelog and release notes.

## Steps

1. Skip upgrade risk assessment on a pull request if it does not change any of the following files:
    * `deploy/charts/harvester/values.yaml`
    * `deploy/charts/harvester/Chart.yaml`
    * `scripts/version-rancher`
    * `scripts/version-rke2`
1. If a pull request is skipped, call the `noop` tool with a message explaining why.
1. If a pull request changes any of the files listed in step 1, check if it bumps the version of any key components following instructions in the "About components upgrade" section. If not, skip risk assessment on the pull request, and call the `noop` tool with a message explaining why.
1. If a pull request bumps the version of any key component, check the list of commits between the old version and the new version. See the "Determining change logs" section for instructions on how to determine the changelog.
1. Check whether the upstream changes touch the component's packaging (Helm chart or deployment manifests) that we vendor into this repository. See the "Vendored Helm chart drift" section for instructions.
1. Categorize the commits into three categories: new features, bug fixes, and build/test/docs changes. Then report the risk profile based on the rules provided in the "Risk profile assessment rules" section.
1. Report the risk profile in the pull request comment. See the "Report Risk Profile" section for the report template.

## About components upgrade

### deploy/charts/harvester/values.yaml

This file contains the image versions of several key Harvester components and 3rd party components. A pull request upgrades a component's version by bumping the `tag` property of the component image in this file.

We only care about this list of 3rd party components and their images:

Component          | Included Images (name only)
------------------ | ---------------------------
kubevirt-operator  | virt-operator
kubevirt           | virt-controller, virt-handler, virt-api, virt-launcher, libguestfs-tools
cdi                | cdi-operator, cdi-controller, cdi-importer, cdi-cloner, cdi-apiserver, cdi-uploadserver, cdi-uploadproxy, kuberlr-kubectl
csi-snapshotter    | snapshot-controller
kube-vip           | kube-vip-iptables
whereabouts        | whereabouts

Components with image name prefixed with `rancher/harvester` are exempted from the upgrade risk assessment, because they are developed and maintained by us. For example, skip assessment for version changes of components like `harvester-network-controller`, `harvester-networkfs-manager` because their image names are prefixed with `rancher/harvester`.

Hence, we don't care about this list of components and their images:

* containers
* harvester-network-controller
* harvester-networkfs-manager
* harvester-node-disk-manager
* webhook
* upgrade
* harvester-load-balancer
* support-bundle-kit
* generalJob

#### KubeVirt and CDI version upgrade

When bumping KubeVirt and KubeVirt CDI versions, ignore the pre-release identifier (e.g., `-rc`, `-beta`, `-alpha`) of the version; use only the major.minor.patch segment.

For example,

* if a pull request bumps KubeVirt from `1.7.0-150700.3.16.2` to `1.7.0-150700.3.21.1` where both the source and target versions are `1.7.0`, notify the maintainer that this is an internal pre-release upgrade, skip the upgrade risk assessment and report "Unknown" risk profile
* if a pull request bumps KubeVirt from `1.6.3-150700.3.13.1` to `1.7.0-150700.3.16.2`, compare the commits between KubeVirt 1.6.3 and 1.7.0. The pre-release identifiers can be ignored

### deploy/charts/harvester/Chart.yaml

The only dependency version upgrade we care about in this file is the Longhorn version upgrade.

A Longhorn version upgrade is signified by changes to the `version` property of the Longhorn Helm chart defined under the `dependencies` section in this file. For other dependencies, we don't care about their version upgrades.

### scripts/version-rancher

A Rancher version upgrade is signified by changes to the `RANCHER_VERSION` variable in this file. For alpha releases where the release name is suffixed with `-alpha`, you may not find any release notes or changelog in the GitHub repository. Make sure to compare the commits between the old version and the new version anyway to determine the risk profile.

### scripts/version-rke2

A RKE2 version upgrade is signified by changes to the `RKE2_VERSION` variable in this file.

### Examples of component upgrade patches

Bumps KubeVirt from version 1.6.3 to 1.7.0:

```diff
diff --git a/deploy/charts/harvester/values.yaml b/deploy/charts/harvester/values.yaml
index 3ce05c0df..ee6bb6651 100644
--- a/deploy/charts/harvester/values.yaml
+++ b/deploy/charts/harvester/values.yaml
@@ -26,7 +26,7 @@ kubevirt-operator:
     operator:
       image:
         repository: registry.suse.com/suse/sles/15.7/virt-operator
-        tag: &kubevirtVersion 1.6.3-150700.3.13.1
+        tag: &kubevirtVersion 1.7.0-150700.3.16.2
     ## The following images are placeholder for images in use.
     ## They are not used by the kubevirt-operator chart.
     controller:
```

Bumps KubeVirt CDI from version 1.62.0 to 1.64.0:

```diff
diff --git a/deploy/charts/harvester/values.yaml b/deploy/charts/harvester/values.yaml
index f36dc7008..7a8325040 100644
--- a/deploy/charts/harvester/values.yaml
+++ b/deploy/charts/harvester/values.yaml
@@ -132,7 +132,7 @@ kubevirt:
 cdi:
   enabled: true
   containers:
-    version: &cdiVersion 1.62.0-150700.9.3.1
+    version: &cdiVersion 1.64.0-150700.9.6.1
     operator:
       image:
         repository: registry.suse.com/suse/sles/15.7/cdi-operator
```

Bumps Longhorn from version 1.12.0-rc2 to 1.12.0:

```diff
diff --git a/deploy/charts/harvester/Chart.yaml b/deploy/charts/harvester/Chart.yaml
index 4f05eac57..27d4e9c2f 100644
--- a/deploy/charts/harvester/Chart.yaml
+++ b/deploy/charts/harvester/Chart.yaml
@@ -50,8 +50,8 @@ dependencies:
     repository: file://dependency_charts/csi-snapshotter
     condition: csi-snapshotter.enabled
   - name: longhorn
-    version: 1.12.0-rc2
-    repository: file://dependency_charts/longhorn-rc
+    version: 1.12.0
+    repository: https://charts.longhorn.io
     condition: longhorn.enabled
   - name: kube-vip
     version: 0.9.8
```

Bumps Rancher from version v2.14.2 to v2.15.0-alpha21:

```diff
diff --git a/scripts/version-rancher b/scripts/version-rancher
index 283dbb7e9..294cd69d8 100644
--- a/scripts/version-rancher
+++ b/scripts/version-rancher
@@ -1 +1 @@
-RANCHER_VERSION="v2.14.2"
+RANCHER_VERSION="v2.15.0-alpha21"
```

Bumps RKE2 from version v1.35.6+rke2r1 to v1.36.2+rke2r1:

```diff
diff --git a/scripts/version-rke2 b/scripts/version-rke2
index 73c5bae0f..7b9b7ace6 100644
--- a/scripts/version-rke2
+++ b/scripts/version-rke2
@@ -1 +1 @@
-RKE2_VERSION="v1.35.6+rke2r1"
+RKE2_VERSION="v1.36.2+rke2r1"
```

## Vendored Helm chart drift

Harvester does not always consume a 3rd party component's Helm chart from its upstream chart repository. For several components we vendor a **copy** of the upstream chart (or a chart we hand-wrote from the upstream deployment manifests) under `deploy/charts/harvester/dependency_charts/<component>`. These vendored copies are **not** updated automatically when a component image tag is bumped, so an upstream release that changes the chart leaves our copy stale.

This drift is easy to miss and can break the upgrade at deployment time (missing RBAC rules, renamed values keys, new/removed workloads, changed container args). Always check for it, and always surface it in the report.

### Vendored charts and their upstream sources

Component         | Vendored copy in this repository                              | Upstream path(s) to watch
----------------- | ------------------------------------------------------------- | -------------------------
kubevirt-operator | `deploy/charts/harvester/dependency_charts/kubevirt-operator` | `manifests/generated`, `manifests/release` in <https://github.com/kubevirt/kubevirt>
kubevirt          | `deploy/charts/harvester/dependency_charts/kubevirt`          | `manifests/generated`, `manifests/release` in <https://github.com/kubevirt/kubevirt>
cdi               | `deploy/charts/harvester/dependency_charts/cdi`               | `manifests/templates` in <https://github.com/kubevirt/containerized-data-importer>
csi-snapshotter   | `deploy/charts/harvester/dependency_charts/csi-snapshotter`   | `client/config/crd`, `deploy/kubernetes/snapshot-controller` in <https://github.com/kubernetes-csi/external-snapshotter>
whereabouts       | `deploy/charts/harvester/dependency_charts/whereabouts`       | `deployment/whereabouts-chart`, `doc/crds` in <https://github.com/k8snetworkplumbingwg/whereabouts>

Longhorn, kube-vip and the `rancher/harvester`-prefixed components are pulled from remote chart repositories (see the `repository` field of each entry in `deploy/charts/harvester/Chart.yaml`), so they are **not** vendored and are not subject to this drift. Do not report chart drift for them.

### How to detect the drift

For each component the pull request bumps, diff the upstream paths from the table above between the old tag and the new tag, and list the files that changed:

```sh
# whereabouts chart + manifest changes between v0.9.0 and v0.10.0
gh api repos/k8snetworkplumbingwg/whereabouts/compare/v0.9.0...v0.10.0 \
  --jq '.files[].filename | select(startswith("deployment/whereabouts-chart/") or startswith("doc/crds/"))'

# the pull requests that touched them, for linking in the report
gh api repos/k8snetworkplumbingwg/whereabouts/compare/v0.9.0...v0.10.0 \
  --jq '.commits[] | "\(.sha[0:7]) \(.commit.message | split("\n")[0])"'
```

Note that the compare API truncates the `files` array at 300 entries. If the comparison is truncated, say so in the report rather than concluding there is no drift.

When you find changed files, read the upstream diff (`gh api repos/OWNER/REPO/compare/OLD...NEW --jq '.files[] | select(.filename == "...") | .patch'`) and compare it against the corresponding file in our vendored copy to work out what our maintainers actually need to port. Concentrate on changes that alter deployed behaviour:

* new, removed or renamed workloads (Deployment, DaemonSet, Job)
* RBAC changes — added or removed rules in `ClusterRole`/`Role`, new `ServiceAccount`
* new or renamed CRDs, and CRD schema changes
* changed container args, env vars, probes, security contexts or resource requests
* new, renamed or removed keys in the upstream chart's `values.yaml`, since Harvester's `deploy/charts/harvester/values.yaml` sets them
* changes to the chart's `appVersion`, which tells us which image tag the upstream chart expects

Ignore purely cosmetic upstream chart changes such as comment or whitespace edits, chart `description` wording, and README-only updates.

Example: <https://github.com/k8snetworkplumbingwg/whereabouts/pull/700> moves the IP reconciler from a cron job in the DaemonSet into a standalone Deployment, touching `deployment/whereabouts-chart/templates/daemonset.yaml`, `deployment/whereabouts-chart/templates/reconciler.yaml` and `deployment/whereabouts-chart/values.yaml`. A whereabouts bump that includes that pull request requires maintainers to port the new `reconciler.yaml` template and the corresponding values into `deploy/charts/harvester/dependency_charts/whereabouts` — the image tag bump alone is not enough.

## Determining change logs

The following table shows the remote repositories for the components we care about, and where to find the commits, changelog and release notes:

Component    | Remote Repository
------------ |------------------
KubeVirt     | <https://github.com/kubevirt/kubevirt.git>
KubeVirt CDI | <https://github.com/kubevirt/containerized-data-importer.git>
CSI          | <https://github.com/kubernetes-csi/external-snapshotter.git>
kube-vip     | <https://github.com/kube-vip/kube-vip.git>
whereabouts  | <https://github.com/k8snetworkplumbingwg/whereabouts.git>
Rancher      | <https://github.com/rancher/rancher.git>
RKE2         | <https://github.com/rancher/rke2.git>

Use the `gh` CLI tool to fetch the commits between the old version and the new version. For example,

```sh
# for the 1.7.0 to 1.7.4 comparison
gh api repos/kubevirt/kubevirt/compare/v1.7.0...v1.7.4 --jq '.commits[] | "\(.sha[0:7]) \(.commit.message | split("\n")[0])"'

# for the 1.6.3 to 1.6.6 comparison
gh api repos/kubevirt/kubevirt/compare/v1.6.3...v1.6.6 --jq '.commits[] | "\(.sha[0:7]) \(.commit.message | split("\n")[0])"'
```

Do not include current version's commits in the changelog. For example, when upgrading from version 1.7.0 to 1.7.4, do not include the commits of 1.7.0.

Also, use the `web-fetch` tool to fetch the release notes for each version and curate summaries of notable changes, which complement the raw commit list.

If you cannot find the changelog, release notes and commits for the component, report "Unknown" risk profile, explain the challenges you faced, and notify the maintainers to manually review the upgrade.

## Risk profile assessment rules

1. Breaking changes, API changes and deprecated features are high risk items
1. Big change scope (measured in terms of LoC) poses higher risks than small change scope
1. New features pose higher risks than bug fixes
1. Critical or high severity security issues in the new version must be labeled as high risks
1. Vulnerability (CVE) fixes are always good to have
1. Call out dependency upgrades. For example, in `go.mod`. Label them as high risks if there are major version changes or known vulnerabilities in the new versions
1. Upstream Helm chart or deployment manifest changes that are not mirrored in our vendored copy are high risk, because the component would be deployed with a stale chart. Treat added/removed workloads, RBAC changes, CRD changes and removed or renamed `values.yaml` keys as high risk; treat additive, defaulted `values.yaml` keys as medium risk
1. Changes to build/test/docs are generally low risk and can be ignored

## Report Risk Profile

Use the template defined in the "Risk Profile Template" section below to report the risk profile of the component upgrade.

Due to the character limit of each pull request comment, report the risk profile of each component upgrade in a separate pull request comment. For example, if a pull request contains version bumps for both KubeVirt and CDI, create two pull request comments, one for KubeVirt and one for CDI, and report the risk profile separately.

In the risk profile report, use hyperlinks to reference upstream pull requests and security advisories to allow maintainers to quickly access more details about these items.

However, GitHub imposes a limit of 50 hyperlinks in a pull request comment. If the number of hyperlinks exceeded the allowed limit, our workflow would fail. One way to workaround this is to use hyperlinks in the following important sections only:

* High-Risk Items
* Helm Chart Drift
* Bug Fixes Critical/High Priority
* Security Advisories

For less important items in the medium and low priority bug fixes, build/test/docs changes sections, providing just the pull request numbers without hyperlinks is acceptable.

For fun, add some emojis to make the report more visually appealing and easier to scan. For example, you can use 🚨 for high-risk items, ⎈ for Helm chart drift, 🐛 for bug fixes, and 📚 for build/test/docs changes.

### Risk Profile Template

```md
## Upgrade Risk Profile Summary (component name):

This section should summarize the risk profile with the following information

- the component name
- the old version and the new versions
- the risk profile (High/Medium/Low)
- whether the vendored Helm chart needs to be updated (Yes/No/Not vendored)

## High-Risk Items (New Features/Behavioral Changes)

* item 1 - can be commit message, issue number/title, a release note entry etc. if possible, identify the release version of this item.
* item 2
* ...

## Helm Chart Drift

Only include this section for components whose Helm chart we vendor under `deploy/charts/harvester/dependency_charts/`. See the "Vendored Helm chart drift" section.

State up front whether the vendored chart needs to be updated. If the upstream chart and deployment manifests are unchanged between the two versions, say so in a single line and move on.

If there is drift, name the vendored chart path maintainers must update, and for each upstream change list:

* the upstream file that changed and the pull request that changed it, hyperlinked
* what the change does, in one line
* what maintainers need to port into the vendored copy, and whether `deploy/charts/harvester/values.yaml` also needs a matching change

For example:

> ⎈ **The vendored chart needs updating**: `deploy/charts/harvester/dependency_charts/whereabouts`
>
> * [#700](https://github.com/k8snetworkplumbingwg/whereabouts/pull/700) moves the IP reconciler out of the DaemonSet cron into a standalone Deployment (`deployment/whereabouts-chart/templates/reconciler.yaml`, `.../daemonset.yaml`, `.../values.yaml`). Port the new `reconciler.yaml` template and drop the reconciler cron from our `templates/daemonset.yaml`; the new `reconciler.*` values keys also need to be surfaced in `deploy/charts/harvester/values.yaml`.

## Bug Fixes

This section allows maintainers to see the bug fixes in the new version, and assess whether they are important to the upgrade decision.

For each bug fix commit, identify the upstream issue in the KubeVirt repository. Check to see if there is a related Harvester issue. If there is, mark the bug fix as "Critical/High Priority".

Where possible, identify the release version where each item is introduced.

### Critical/High Priority

Critical/high priority bug fixes are those that address critical issues such as security vulnerabilities, system instability, feature breakages, data integrity risks in the older version.

### Medium Priority

Medium priority bug fixes are those that address important issues that may not be critical but can still impact the user experience, such as performance degradation, minor feature breakages, or non-critical bugs. The bugs usually have a workaround or do not affect core functionalities. Workflow interruption, incorrect metrics/observability, feature degradation in specific scenarios etc. can be categorized as medium priority.

### Low Priority

Low priority bug fixes are those that address minor issues, such as UI/UX improvements, non-critical bugs with easy workarounds, architecture/vendor-specific issues, or bugs that affect edge cases with minimal impact on the overall user experience. Cosmetic issues, typos, non-functional documentation etc. can be categorized as low priority.

## Build/Test/Docs (Ignored)

This is for maintainers to be aware of the changes to build/test/docs, but they are generally low risk and can be ignored in the upgrade decision.

## Security Advisories

This section should list any security advisories (CVEs) that are not fixed in the new target version. For example, when upgrading KubeVirt from version 1.7.0 to 1.7.4, only list advisories with CVEs that are not fixed in KubeVirt 1.7.4. Do not report any fixed CVEs to help reduce noise in the report. If you are not sure if a CVE is fixed, include it in the list and notify the maintainers to verify it.

For each CVE, provide a link to the CVE details and a brief description of the vulnerability and its potential impact. Do not include duplicated entries.

## Recommendation

This section should make a recommendation on whether or not to proceed with upgrade, and provide justification for the recommendation.

If the vendored Helm chart needs updating, call it out here as a blocker that must be done in the same pull request as the image tag bump.

Provide a list of important things for maintainers to validate before/after upgrade.
```

## Maintenance

If a pull request adds a new component that is not in the list of components we care about notify the maintainers to update the table in the `.github/workflows/upgrade-risk.md` file.

Likewise, if a component's chart is vendored under `deploy/charts/harvester/dependency_charts/` but is missing from the "Vendored charts and their upstream sources" table, or if the upstream path in that table no longer exists, notify the maintainers to update that table too.
