# Harvester Enhancement Proposal (HEP)

This directory collects Harvester Enhancement Proposals (HEP). HEPs are used to describe highlighted features, enhancements, or bug fixes that require significant changes to the code or architecture. You can use the [template](./YYYYMMDD-template.md) to create a HEP pull request.

The following steps describe how HEPs are involved in the development process:

1. An Engineer or contributor gets assigned a Github issue. The GitHub issue should have the `require/HEP` label.
1. When the assignee decides to work on it (which likely means it’s now targeting the next minor release), they will start working on the HEP if HEP is required. It might take a while because it also involves POC and lots of discussions.
1. Once the assignee has an initial design, he or she will submit a PR containing the HEP to the `harvester/harvester` GitHub repo, `enhancements` directory. For example https://github.com/harvester/harvester/pull/3673.
1. More discussion will start on the HEP, now it’s time we can get feedback from the community or team.
1. HEP might be not getting merged for a while. The assignee might decide to implement the feature more and come back on updating the HEP. It’s generally required HEP to be merged when the feature is merged, so the HEP is at least going to reflect how the first iteration of works.
