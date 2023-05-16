# Harvester Enhancement Proposal (HEP)

This directory serves as a collection of Harvester Enhancement Proposals (HEPs). HEPs are used to describe highlighted features, enhancements, or bug fixes that require substantial changes to the code or architecture. To create a HEP, you can use the provided [template](./YYYYMMDD-template.md) and submit a pull request.

The following steps describe how HEPs are involved in the development process:

1. An engineer or contributor is assigned a GitHub issue. The GitHub issue should have the `require/HEP` label. If the issue doesn't have the `require/HEP` label initially but after investigation, the assigned engineer or contributor thinks the issue should have a HEP, they can add the label or contact the project's maintainer to add the label.
1. Once the assignee decides to work on the issue (typically targeting the next minor release), they will begin working on the HEP first. This process may involve proof of concept and extensive discussions.
1. After the assignee has a preliminary design, they will submit a pull request containing the HEP to the `harvester/harvester` GitHub repository's `enhancements` directory. For example, you can find a HEP pull request at https://github.com/harvester/harvester/pull/3673.
1. The HEP will undergo further discussion, allowing for feedback from the community or team.
1. The HEP might not be merged immediately. The assignee may choose to implement the feature further before updating the HEP. Generally, it is required to merge the HEP when the feature itself is merged. The HEP will ensure that the feature work reflects the initial iteration of work.
