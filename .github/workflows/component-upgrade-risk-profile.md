---
description: |
  This workflow assesses the risk profile of a component upgrade when a pull
  request is opened to bump the version of a key component. It categorizes the
  commits in the component's changelog and release notes, and reports the risk 
  profile in the pull request comment to help maintainers make informed decisions
  about whether to proceed with the upgrade.

on:
  pull_request

permissions:
  contents: read
  copilot-requests: write
  issues: read
  pull-requests: read

tools:
  github:
    toolsets: [default]
  web-fetch:

network:
  allowed:
    - defaults
    - github

safe-outputs:
  add-comment:
    max: 5
  noop:

engine:
  id: copilot
  model: claude-sonnet-4.6

timeout-minutes: 10
---
# Component Upgrade Risk Profile

## Goals

Helps maintainers make informed decisions about whether to proceed with a component upgrade. When a pull request is opened to upgrade a key component, generate a risk profile of the upgrade by categorizing the commits in the component's changelog and release notes.

## Steps

1. If a pull request does not change the `deploy/charts/harvester/values.yaml` or `deploy/charts/harvester/Chart.yaml` files, skip risk assessment and stop.
1. If a pull request changes the `deploy/charts/harvester/values.yaml` or `deploy/charts/harvester/Chart.yaml` files, check if it bumps the version of any key component following instructions in the "About components upgrade" section. If not, skip risk assessment and stop.
1. If a pull request bumps the version of any key component, check the list of commits between the old version and the new version. See the "Determining change logs" section for instructions on how to determine the changelog.
1. Categorize the commits into three categories: new features, bug fixes, and build/test/docs changes. Then report the risk profile based on the rules provided in the "Risk profile assessment rules" section.
1. Report the risk profile in the pull request comment. See the "Report Risk Profile" section for the report template.

## About components upgrade

A pull request contains components upgrade when it bumps the `tag` property of the components in the `deploy/charts/harvester/values.yaml` file.

These are the list of components and their images we care about:

Component          | Included Images (name only)
------------------ | ---------------------------
kubevirt-operator  | virt-operator
kubevirt           | virt-controller, virt-handler, virt-api, virt-launcher, libguestfs-tools
cdi                | cdi-operator, cdi-controller, cdi-importer, cdi-cloner, cdi-apiserver, cdi-uploadserver, cdi-uploadproxy, kuberlr-kubectl
csi-snapshotter    | snapshot-controller
kube-vip           | kube-vip-iptables
whereabouts        | whereabouts

The image versions are determined by the `tag` property of the images in the `deploy/charts/harvester/values.yaml` file.

Components with image name prefixed with `rancher/harvester` are exempted from the upgrade risk assessment, because they are developed and maintained by us. For example, skip assessment for version changes of components like `harvester-network-controller`, `harvester-networkfs-manager` because their image names are prefixed with `rancher/harvester` .

These are the list of components and their images that we don't care about:

* containers
* harvester-network-controller
* harvester-networkfs-manager
* harvester-node-disk-manager
* longhorn (managed differently, see the "Longhorn version upgrade" section for details)
* webhook
* upgrade
* harvester-load-balancer
* support-bundle-kit
* generalJob

Longhorn is managed differently. See the "Longhorn version upgrade" section for details.

### Longhorn version upgrade

The Longhorn components are managed differently. For Longhorn, a version upgrade is signified by changes to the `version` property of the Longhorn Helm chart defined under the `dependencies` section in the `deploy/charts/harvester/Chart.yaml` file. There is no need to track down its image names and versions.

### KubeVirt and CDI version upgrade

When bumping KubeVirt and KubeVirt CDI versions, ignore the pre-release identifier (e.g., `-rc`, `-beta`, `-alpha`) of the version; use only the major.minor.patch segment.

For example,

* if a pull request bumps KubeVirt from `1.7.0-150700.3.16.2` to `1.7.0-150700.3.21.1` where both the source and target versions are `1.7.0`, notify the maintainer that this is an internal pre-release upgrade, skip the upgrade risk assessment and report "Unknown" risk profile
* if a pull request bumps KubeVirt from `1.6.3-150700.3.13.1` to `1.7.0-150700.3.16.2`, compare the commits between KubeVirt 1.6.3 and 1.7.0. The pre-release identifiers can be ignored

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

## Determining change logs

The following table shows the remote repositories for the components we care about, and where to find the commits, changelog and release notes:

Component    | Remote Repository
------------ |------------------
KubeVirt     | <https://github.com/kubevirt/kubevirt.git>
KubeVirt CDI | <https://github.com/kubevirt/containerized-data-importer.git>
CSI          | <https://github.com/kubernetes-csi/external-snapshotter.git>
kube-vip     | <https://github.com/kube-vip/kube-vip.git>
whereabouts  | <https://github.com/k8snetworkplumbingwg/whereabouts.git>

Use the `gh` CLI tool to fetch the commits between the old version and the new version. For example,

```sh
# for the 1.7.0 to 1.7.4 comparison
gh api repos/kubevirt/kubevirt/compare/v1.7.0...v1.7.4 --jq '.commits[] | "\(.sha[0:7]) \(.commit.message | split("\n")[0])"'

# for the 1.6.3 to 1.6.6 comparison
gh api repos/kubevirt/kubevirt/compare/v1.6.3...v1.6.6 --jq '.commits[] | "\(.sha[0:7]) \(.commit.message | split("\n")[0])"'
```

Do not include current version's commits in the changelog. For example, when upgrading from version 1.7.0 to 1.7.4, do not include the commits of 1.7.0.

Also, use the `web-fetch` tool to fetch the release notes for each version and curate summaries of notable changes, which  complement the raw commit list.

If you cannot find the changelog or release notes for the component, report "Unknown" risk profile, explain the challenges you faced, and notify the maintainers to manually review the upgrade.

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

Limit the usage of hyperlinks in the risk profile to no more than 50 links due to GitHub limitation on the number of links in a comment. A good usage of hyperlinks is to link to upstream pull requests and issues.

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

This section should list any security advisories (CVEs) affecting the new versions. These are CVEs that are not fixed in the new versions. For example, when upgrading from version 1.7.0 to 1.7.4, list all the CVEs that are not fixed in 1.7.1, 1.7.2, 1.7.3 and 1.7.4.

For each CVE, provide a link to the CVE details and a brief description of the vulnerability and its potential impact. Do not include duplicated entries.

## Recommendation

This section should make a recommendation on whether or not to proceed with upgrade, and provide justification for the recommendation.

Provide a list of important things for maintainers to validate before/after upgrade.
```

## Maintenance

If a pull request adds a new component that is not in the list of components we care about notify the maintainers to update the table in the `.github/workflows/component-upgrade-risk-profile.md` file.
