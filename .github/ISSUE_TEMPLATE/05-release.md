---
name: Release task
about: Create a release task
title: "[RELEASE] Release v"
labels: kind/task
assignees: ''

---

## What's the task? Please describe.
Action items for releasing v

## Roles
- Release captain:  <!--responsible for RD efforts of release development and coordinating with QA captain-->
- QA captain:  <!--responsible for coordinating QA efforts of release testing tasks-->

## Describe the sub-tasks.

### Pre-Release

**The Release Captain needs to finish the following items.**

- [ ] Before creating RC1, create a new milestone and stable branches for the release [[doc](https://github.com/harvester/harvester/wiki/Creating-a-new-milestone)].
- [ ] Release candidate builds [[doc](https://github.com/harvester/harvester/wiki/Create-a-Harvester-release)].

**The QA captain needs to coordinate the following items before the GA release.**

- [ ] Regression test plan (manual) on Qase.
- [ ] Run e2e tests for pre-GA milestones.
- [ ] Run upgrade tests for the pre-GA build
 
<!--
- [ ] Run security testing of container images for pre-GA milestones
- [ ] Create security issues at upstream for unresolved CVEs in CSI sidecar images 
-->

### Release

**The Release Captain needs to finish the following items.**

- [ ] Release build [[doc](https://github.com/harvester/harvester/wiki/Create-a-Harvester-release)].
- [ ] Release note - Work with @harvester/doc
  - [ ] Create a release note in https://github.com/harvester/release-notes
    - PR: 
  - [ ] Deprecation note
  - [ ] Upgrade notes including highlighted notes, deprecation, compatible changes, and others impacting the current users
- Doc
    - [ ] Publish the new version of doc and add the next patch version of the dev doc. @harvester/doc 
    - [ ] Create an upgrade page for this version.
      - PR:
      - Doc:
- [ ] Update the README. (PR: )
  - [ ] Update the release to the latest in the [versions](https://github.com/harvester/harvester?tab=readme-ov-file#releases).
  - [ ] Update screenshots.
- Support matrix
  - [ ] Coordinate with PM and QA to create a [support matrix page](https://www.suse.com/suse-harvester/support-matrix) (e.g., [Harvester v1.4.x](https://www.suse.com/suse-harvester/support-matrix/all-supported-versions/harvester-v1-4-x/)).
    - Updated support matrix is at: 


### Post-Release

- [ ] Update [support-versions](https://github.com/harvester/harvester/blob/master/misc/support-versions.txt). You can only keep the latest patch release here.
- [ ] Update upgrade responder 1 week after the release [doc].
  - For the first stable release, we need to consider several factors and reach a consensus among maintainers before claiming it stable. 
  - For any patch release after a stable release, we need to wait 1-2 weeks for user feedback.
- [ ] Create `isv:rancher:Harvester:OS:v<next minor>` and `isv:Rancher:Harvester:ExtraPackages:v<next minor>` projects on BS and link them to the corresponding dev project, and unlink the previous minor from dev.


**After marking the release as a `stable` release, the Release Captain needs to coordinate the following items**

- [ ] Update the [support matrix](https://www.suse.com/suse-harvester/support-matrix) - @asettle

cc @harvester/dev @harvester/qa
