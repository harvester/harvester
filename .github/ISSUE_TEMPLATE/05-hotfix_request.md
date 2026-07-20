---
name: Hotfix task
about: Create a hotfix task
title: "[HOTFIX] Create HOTFIX for v"
type: "Task"
labels: kind/release
assignees: ''

---

## What's the task? Please describe

The regression in Harvester/<component> v1.x.y. <!-- add details for the regression -->

To mitigate/resolve this issue, we will release a hotfix image before the v1.x.<y+1> release.

Reference: <!-- add the link to the issue that describes the regression -->

## Describe the sub-tasks

- [ ] Create temporary branch for hotfixed component repo and push the fix to this branch
- [ ] Validate the fix using hotfixed image
- [ ] Remove the temporary branch
- [ ] Update the release note of vX.Y.Z

## Additional context

<!--Add any other context or screenshots about the task request here.-->
