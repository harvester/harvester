# Contributing to Harvester

Thank you for considering contributing to Harvester! We welcome contributions from everyone. Here are some guidelines to help you get started.

## Reporting an Issue

If you encounter any issues or have suggestions for improvements, please report them in the [harvester/harvester repository](https://github.com/harvester/harvester/issues). Provide as much detail as possible, including steps to reproduce the issue, your environment, and any relevant logs or screenshots.

To report an issue, click the "New Issue" button and select the appropriate issue type (e.g., Bug Report, Feature Request, Document, etc.). Fill out the provided template with as much detail as possible.

### Issue management

We use GitHub Project to manage issues, please see https://github.com/harvester/harvester/wiki/Issue-Management for more information. Note you must be a member of the Harvester organization to see the project. Feel free to delegate the issue management to maintainers if you are not a member.

## Submitting Code Changes

Code changes must be submitted via GitHub pull requests. Here are some general guidelines:

- Find or create an issue first. Every PR should link to at least one issue. Please add the issue URL below the `**Related Issue**` header.
- Fork the repository that you want to contribute. Make and commit changes in your fork repository.
- Sign your work with a sign-off statement in the commit messages.
- Create a pull request that targets an appropriate branch by following the PR description template.
- Provide a test plan in the PR description.
- Make sure all the checks pass in the PR.


### Commit message format

We didn't enforce the commit message format, any reasonable format is acceptable. One recommendation is [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).

### Branch strategy

The branch strategy of the harvester/harvester repo is:

- Create a pull request that targets the `master` branch.
- Backport your change to stable branches with backport pull requests.

Please see https://github.com/harvester/harvester/wiki/Branch-Strategy#harvesterharvester for more information. Note you must create backport issue and link backport PR to backport issues.

### How to Test Your Changes Locally

You need a Linux box with docker for the development. After you make some change to the code, you can run the integration test with:

```
make ci
```

To build a Harvester ISO, you can run the command:

```
make build-iso
```

And check the `dist/artifacts` for the resulting files. You can test the ISO on physical servers or use [Vagrant](https://github.com/harvester/ipxe-examples/tree/main/vagrant-pxe-harvester) to test the ISO on virtual machines.

### Code Style

The code must be linted with `golangci-lint`. You can manually run the linter/configure your IDE with the [config](https://github.com/harvester/harvester/blob/master/.golangci.yaml) or run the command to do a validation:

```
make validate
```

## Contribute to Harvester Documentation

There are two document sites:
- Harvester Document (https://docs.harvesterhci.io/): You can submit PRs at https://github.com/harvester/docs.
- Harvester Knowledge Base (https://harvesterhci.io/kb/): You can submit PRs at https://github.com/harvester/harvesterhci.io.

The documents are written in Markdown and you can check the preview after submitting PRs.

## Where to Ask for Help

If you have any questions or need assistance, feel free to reach us in the `#harvester` channel in [Slack](https://slack.rancher.io/).