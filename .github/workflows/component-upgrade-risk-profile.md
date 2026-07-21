---
description: |
  This workflow assesses the risk profile of a component upgrade when a pull
  request is opened to bump the version of a key component. It categorizes the
  commits in the component's changelog and release notes, and reports the risk 
  profile in the pull request comment to help maintainers make informed decisions
  about whether to proceed with the upgrade.

on:
  slash_command:
    name: assess-upgrade
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

engine:
  id: copilot
  model: claude-sonnet-4.6

timeout-minutes: 10
---
# Component Upgrade Risk Profile

Perform an upgrade risk assessment review on a pull request when a maintainer invokes `/assess-upgrade` in a PR conversation comment or inline review comment. This is a manually triggered workflow, and it is not automatically triggered on PR open or update. It does not respond to `/assess-upgrade` placed in the PR description body. Because it is triggered via comment events on the base repository, it runs in the base-repo context with full secrets and write access, so it works on PRs from forks as well as same-repo PRs.

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
1. Changes to build/test/docs are generally low risk and can be ignored

## Report Risk Profile

Use the template defined in the "Risk Profile Template" section below to report the risk profile of the component upgrade.

Due to the character limit of each pull request comment, report the risk profile of each component upgrade in a separate pull request comment. For example, if a pull request contains version bumps for both KubeVirt and CDI, create two pull request comments, one for KubeVirt and one for CDI, and report the risk profile separately.

In the risk profile report, use hyperlinks to reference upstream pull requests and security advisories to allow maintainers to quickly access more details about these items.

However, GitHub imposes a limit of 50 hyperlinks in a pull request comment. If the number of hyperlinks exceeded the allowed limit, our workflow would fail. One way to workaround this is to use hyperlinks in the following important sections only:

* High-Risk Items
* Bug Fixes Critical/High Priority
* Security Advisories

For less important items in the medium and low priority bug fixes, build/test/docs changes sections, providing just the pull request numbers without hyperlinks is acceptable.

For fun, add some emojis to make the report more visually appealing and easier to scan. For example, you can use 🚨 for high-risk items, 🐛 for bug fixes, and 📚 for build/test/docs changes.

### Risk Profile Template

```md
## Upgrade Risk Profile Summary (component name):

This section should summarize the risk profile with the following information

- the component name
- the old version and the new versions
- the risk profile (High/Medium/Low)

## High-Risk Items (New Features/Behavioral Changes)

* item 1 - can be commit message, issue number/title, a release note entry etc. if possible, identify the release version of this item.
* item 2
* ...

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

Provide a list of important things for maintainers to validate before/after upgrade.
```

## Maintenance

If a pull request adds a new component that is not in the list of components we care about notify the maintainers to update the table in the `.github/workflows/component-upgrade-risk-profile.md` file.
