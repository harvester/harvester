# Harvester Developer Guide

This guide provides essential information for developers who want to contribute to Harvester.

## Table of Contents

- [Things You Need to Know Before Development](#things-you-need-to-know-before-development)
  - [Understanding Harvester](#understanding-harvester)
  - [Rules](#rules)
- [Repository Relations](#repository-relations)
  - [Main Components](#main-components)
  - [Add-ons](#add-ons)
    - [Built-in](#built-in)
    - [Experimental](#experimental)
  - [Rancher Related](#rancher-related)
  - [Integrated Upstream Repositories](#integrated-upstream-repositories)
- [Development](#development)
  - [Prerequisites](#prerequisites)
  - [Basic Knowledge](#basic-knowledge)
  - [Deployment](#deployment)
  - [Writing Test Cases](#writing-test-cases)
- [Debugging and Troubleshooting](#debugging-and-troubleshooting)
- [Testing and Building](#testing-and-building)
  - [Test Your Changes by Patching the Image](#test-your-changes-by-patching-the-image)
  - [Test Your Changes with a Fresh ISO](#test-your-changes-with-a-fresh-iso)
  - [Automation Testing](#automation-testing)
- [Before Opening a Pull Request](#before-opening-a-pull-request)
  - [How to Find an Issue to Work On](#how-to-find-an-issue-to-work-on)
  - [Branch Strategy](#branch-strategy)
  - [Code Style](#code-style)
  - [Commit Message Format](#commit-message-format)
- [After Opening a Pull Request](#after-opening-a-pull-request)
- [After All PRs Are Merged](#after-all-prs-are-merged)
- [Example Issues](#example-issues)

## Things You Need to Know Before Development

### Understanding Harvester

We recommend installing a Harvester cluster to understand how to use its features. We provide several installation methods:

- [ISO Installation](https://docs.harvesterhci.io/latest/install/index)
- [USB Installation](https://docs.harvesterhci.io/latest/install/usb-install/)
- [PXE Boot Installation](https://docs.harvesterhci.io/latest/install/pxe-boot-install/)
- [Net Install ISO](https://docs.harvesterhci.io/latest/install/net-install/)

### Rules

Code changes must be submitted via GitHub pull requests. Here are some general guidelines:

- Find or create an issue first. Every PR should link to at least one issue.
- Fork the repository to which you want to contribute. Make and commit changes in your fork.
- Create a pull request that targets the appropriate branch, using the PR description template.
- Sign off your commits by including a Signed-off-by line in each commit message.

## Repository Relations

This section describes the scope of each repository:

### Main Components

- harvester/harvester
  - Basic features, including virtual machines, images, upgrades ([package/upgrade](./package/upgrade)), volumes, etc.
- harvester/harvester-installer
  - Installation console and ISO building. It also packages the underlying OS (harvester/os2) and rancher/rancherd.
- harvester/os2
  - The Harvester baseOS provider
- harvester/docs
  - [Harvester Official Documentation](https://docs.harvesterhci.io/)
- harvester/network-controller-harvester
  - Manages host network configuration.
- harvester/terraform-provider-harvester
  - Terraform provider for Harvester.
- harvester/node-disk-manager
  - Provides an automated way to add storage to Longhorn as Harvester's backend storage, including multipath support.
- harvester/harvester-ui-extension
  - Harvester Dashboard UI. For more details, see the [official documentation](https://docs.harvesterhci.io/latest/rancher/harvester-ui-extension/).
- harvester/load-balancer-harvester
  - A built-in Layer 4 load balancer that distributes incoming traffic across workloads deployed on Harvester [virtual machines (VMs)](https://docs.harvesterhci.io/latest/networking/loadbalancer#vm-load-balancer) or [guest Kubernetes clusters](https://docs.harvesterhci.io/latest/networking/loadbalancer#guest-kubernetes-cluster-load-balancer).
- harvester/node-manager
  - Manages the node kernel configuration of the Harvester cluster

Check out the [Component Maintainers](https://github.com/harvester/harvester/wiki/Component-Maintainers) for a complete list.

### Add-ons

#### Built-in

- harvester/pcidevices ([Document](https://docs.harvesterhci.io/latest/advanced/addons/pcidevices))
  - PCI, USB, GPU, and vGPU device passthrough.
- harvester/nvidia-driver-toolkit ([Document](https://docs.harvesterhci.io/latest/advanced/addons/nvidiadrivertoolkit))
  - Enable vGPU devices and assign them to Harvester virtual machines. It's used with harvester/pcidevices together.
- harvester/rancher-logging ([Document](https://docs.harvesterhci.io/latest/logging/harvester-logging))
  - Collect versatile logs, events, and audits from the Harvester cluster and route them to many kinds of servers based on flows.
- harvester/rancher-monitoring ([Document](https://docs.harvesterhci.io/latest/monitoring/harvester-monitoring))
  - Collect Harvester cluster and virtual machine metrics, view them on the embedded dashboard, and send alert(s) to remote servers.
- harvester/vm-import-controller ([Document](https://docs.harvesterhci.io/latest/advanced/addons/vmimport))
  - Helps migrate VM workloads from external clusters to an existing Harvester cluster. It currently supports VMware and OpenStack.
- harvester/kubeovn-operator ([Document](https://docs.harvesterhci.io/v1.6/advanced/addons/kubeovn-operator/))
  - Manage the lifecycle of [Kube-OVN](https://github.com/kubeovn/kube-ovn) as a secondary CNI on underlying Harvester clusters.

#### Experimental

- harvester/experimental-addons/harvester-csi-driver-lvm ([Document](https://docs.harvesterhci.io/latest/advanced/addons/lvm-local-storage))
  - CSI driver that supports local path provisioning through LVM.
- harvester/experimental-addons/vm-dhcp-controller ([Document](https://docs.harvesterhci.io/latest/advanced/addons/managed-dhcp))
  - A managed DHCP service for virtual machines running on Harvester.
- harvester/experimental-addons/rancher-vcluster ([Document](https://docs.harvesterhci.io/latest/advanced/addons/rancher-vcluster))
  - Runs Rancher Manager as a workload on the underlying Harvester cluster, implemented using vcluster. It's one way to start a Rancher service and use [Rancher Integration](https://docs.harvesterhci.io/latest/rancher/rancher-integration) with Harvester.


### Rancher Related

- harvester/docker-machine-driver-harvester
  - Use Harvester as a cloud provider to provision guest clusters in Rancher. The related UI is located [here](https://github.com/rancher/dashboard/tree/master/pkg/harvester-manager).
- harvester/cloud-provider-harvester
  - Makes Harvester function as a Kubernetes cloud provider.
- harvester/harvester-csi-driver
  - Provides the generic storage interface for downstream clusters.

### Integrated Upstream Repositories

- longhorn/longhorn
  - Harvester uses Longhorn for virtual machine and node volumes.
- kubevirt/kubevirt
  - Harvester uses KubeVirt to provide virtualization.
- kubevirt/containerized-data-importer
  - Harvester uses Containerized Data Importer to provide third-party storage integration.

## Development

### Prerequisites

Before you start, ensure the following are installed on your development machine:

- OS: Linux or macOS (Linux is required to build ISOs; macOS is fine for day-to-day development).
- Go: See go.mod and use that major.minor version to avoid module/tooling mismatches.
- Docker-compatible container engine: required by [Dapper](https://github.com/rancher/dapper) for builds (Docker Engine or Docker Desktop; Rancher Desktop or Colima also work).
- Make and Git: used to invoke build scripts and manage source.

Notes:
- [Dapper](https://github.com/rancher/dapper) is downloaded automatically by `make`; you just need a working container engine.
- To push locally built images, you’ll set environment variables such as: REPO, PUSH, and USE_LOCAL_IMAGES (see [Testing and Building](#testing-and-building)).

### Basic Knowledge

The most important components in Harvester are the custom resources, controllers, webhooks, and the API server.

- Custom Resource (CR): Harvester defines its Kubernetes API using [custom resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/). The Go API types reside under [pkg/apis](./pkg/apis/). We generate the CustomResourceDefinitions (CRDs) via [go generate](./main.go) and package them under [deploy/charts/harvester-crd](./deploy/charts/harvester-crd). Other Harvester projects may also define their own CRDs.
- Controller: We use [rancher/wrangler](https://github.com/rancher/wrangler) to implement [custom controllers](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/#custom-controllers), which are located in [pkg/controller/master](./pkg/controller/master). Controllers execute the corresponding logic when users create, update, or delete resources.
- Webhook: Webhooks validate and mutate resources when they are created, updated, or deleted. All rules are located in [pkg/webhook/resources](./pkg/webhook/resources/).
- API Server: We use [rancher/apiserver](https://github.com/rancher/apiserver?tab=readme-ov-file) to implement the API server, which is located in [pkg/api](./pkg/api) and [pkg/server](./pkg/server). The API server exposes actions used by the Harvester UI.

### Deployment

The main deployment charts reside in [deploy/charts/harvester](./deploy/charts/harvester/) and [deploy/charts/harvester-crd](./deploy/charts/harvester-crd).

In the underlying implementation, we use rancherd to install all resources needed in Harvester. We deploy a pod called `harvester-cluster-repo-xxxx` to serve the Harvester [deploy/charts](./deploy/charts/) via an Nginx server. Other services that need the Helm charts will call this server to fetch the charts. To debug the deployment process, run `journalctl -u rancherd` on the host to examine the logs.


### Writing Test Cases

When you implement a feature, unit tests are required.

Most Harvester features follow the [Kubernetes controller pattern](https://kubernetes.io/docs/concepts/architecture/controller/), so it's important to test them. Please take a look at the following example to understand basic test case structure:
- [support bundle controller](./pkg/controller/master/supportbundle/controller_test.go)

In addition to unit tests, write automated tests where applicable. If the feature requires an automated test, refer to the [Automation Testing](#automation-testing) section for how to write and run them.

## Debugging and Troubleshooting

In general, we need cluster information for debugging. We usually obtain a [support bundle](https://docs.harvesterhci.io/latest/troubleshooting/harvester/#generate-a-support-bundle) from users. It contains a lot of useful cluster information.

Then, we use [rancher/support-bundle-kit](https://github.com/rancher/support-bundle-kit) to emulate the cluster described by the support bundle. It's a toolkit for generating and analyzing support bundles for Harvester and Longhorn.

After building the binary from the project, you can run `./bin/support-bundle-kit-{amd64|arm64} simulator --bundle-path ./supportbundle_xxxx --reset`. It takes some time to build the simulated cluster. Then you can use the kubeconfig from `~/.sim/admin.kubeconfig` to operate the cluster. In general, most `kubectl` and `helm` commands will work on the simulater cluster. Commands such as `kubectl exec`, `kubectl port-forward` etc., which require the existence of real workloads, will not work.

Finally, refer to the following resources to search for possible solutions.
- [Harvester Documentation](https://docs.harvesterhci.io/)
- [GitHub Issues](https://github.com/harvester/harvester/issues)
- [Harvester HCI Knowledge Base](https://harvesterhci.io/kb/)

If you need help for development, you can ask questions in the [`harvester-dev` channel in Rancher Slack](https://slack.rancher.io/)

## Testing and Building

All repositories use Dapper to build. Ensure you have a Docker-compatible container engine before building.

### Test Your Changes by Patching the Image

In general, each repository produces two images:

- The controller image
- The webhook image

For quick testing, you can patch the image in the cluster with a locally built image. Normally, you can use `make` or `./scripts/build` in each repository, and push the result to a registry by retagging the resulting image. If that doesn't work, see the scripts under `./scripts`.

Therefore, if you develop a virtual machine feature, you should patch the image in the Harvester controller. This depends on which feature you're developing. Before patching, identify which repositories need to be updated in the Harvester cluster.


### Test Your Changes with a Fresh ISO

Making changes to the Harvester installer requires a different workflow to generate the ISO files.

You need a Linux machine with Docker for development. After you make changes to the code, run the build with:

```bash
make
```

To build a Harvester ISO, run:

```bash
make build-iso
```

Additionally, see [Build ISO images](https://github.com/harvester/harvester/wiki/Build-OCI-and-ISO-images) for more build options. If you'd like to push the images to a Docker registry, use the following commands:

```bash
export REPO={docker user name}
export PUSH=true
export USE_LOCAL_IMAGES=true
make
make build-iso
```

After building, check the `dist/artifacts` directory for the resulting files. You can test the ISO on physical servers or use [Vagrant](https://github.com/harvester/ipxe-examples/tree/main/vagrant-pxe-harvester) to test on virtual machines.

Other common environment variables: 
- `TAG`: Target image tag
- [`RKE2_IMAGE_REPO`](https://github.com/harvester/harvester-installer/blob/5610f0e77e706def53a9dceda57316789d005bc0/scripts/build-bundle#L8): Used as the base RKE2 version while building ISO.

### Automation Testing

In addition to unit tests in each project, we have a dedicated repository, [harvester/tests](https://github.com/harvester/tests), to test Harvester features. See the [README](https://github.com/harvester/tests/blob/main/README.md) for project setup details.

Use these directories to test different targets:
- API: [tests/harvester_e2e_tests/apis](https://github.com/harvester/tests/tree/main/harvester_e2e_tests/apis)
- Integration: [tests/harvester_e2e_tests/integrations](https://github.com/harvester/tests/tree/main/harvester_e2e_tests/integrations)

If you're not sure how to start from scratch, take a look at these two PRs:
- API: [harvester/tests#2068](https://github.com/harvester/tests/pull/2068)
- Integration: [harvester/tests#2054](https://github.com/harvester/tests/pull/2054)

Here are example commands to run the tests:

```bash
# xxx.xxx.xxx.xxx -> This is your cluster IP.
# Example: run an API test file
pytest harvester_e2e_tests/apis/test_support_bundle.py --username {account} --password {password} --endpoint {https://xxx.xxx.xxx.xxx}

# Example: run an integration test file
pytest harvester_e2e_tests/integrations/test_1_images.py --username {account} --password {password} --endpoint {https://xxx.xxx.xxx.xxx}
```

## Before Opening a Pull Request

Make sure you have an issue on GitHub. All PRs require an issue. Describe the problem you're trying to solve and how you plan to address it. If the issue has the `require/hep` label, a [HEP (Harvester Enhancement Proposal)](./enhancements) is required for discussion before implementation.

The general workflow is:
- Create or find an issue.
- Discuss why we need to fix it, what the goal is, and how to solve it.
- Open a pull request for an HEP if needed.
- Open a pull request to solve the issue.
- Test the solution.

### How to Find an Issue to Work On

All issues are tracked in the harvester/harvester repository. Issues with a milestone or a "good first issue" label are ready for development. You can use [this filter](https://github.com/harvester/harvester/issues?q=is%3Aissue%20state%3Aopen%20has%3Amilestone) and [this filter](https://github.com/harvester/harvester/issues?q=state%3Aopen%20label%3A%22good%20first%20issue%22) to find open issues with a milestone.

If an issue is already assigned but you want to work on it, leave a comment asking to be assigned or to coordinate with the current assignee. Discussion is welcome.

### Branch Strategy

The branch strategy for the harvester/harvester repository is:

- Create a pull request that targets the `master` or `main` branch. Some older repositories still use the `master` branch.

For more detailed information, see the [Branch Strategy Wiki](https://github.com/harvester/harvester/wiki/Branch-Strategy).

### Code Style

The code must be linted with `golangci-lint`. You can manually run the linter, configure your IDE with the [config](./.golangci.yaml), or run the following command to validate:

```bash
make validate
```

### Commit Message Format

We don't enforce a strict commit message format; any reasonable format is acceptable. One recommendation is [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).

## After Opening a Pull Request

- Provide a test plan in the PR description. Describe how to test and the expected results. A short demo recording is welcome.
- Ensure all checks pass in the PR.

## After All PRs Are Merged

We use the GitHub Project to manage our issues (see [Issue Management](https://github.com/harvester/harvester/wiki/Issue-Management)). Once all PRs are merged, we'll move the issue status to "Ready for Test". The bot will then create a comment titled "Pre Ready-For-Testing Checklist". Please fill out all necessary information in that comment.

Depending on the severity and scope of your changes, discuss with the maintainers to decide if your pull requests need to be backport to older supported versions of Harvester.

## Example Issues

If you're not sure how to get started, take a look at the following examples. In general, we have different types of pull requests categorized by topic. Each topic might involve multiple PRs, including changes to charts, YAML files, and other component repositories.

- **Bumping Dependencies**: [#8642](https://github.com/harvester/harvester/issues/8642)
- **Harvester Feature (Frontend + Backend)**: [#7136](https://github.com/harvester/harvester/issues/7136)
- **Harvester Upgrades and Documentation**: [#8163](https://github.com/harvester/harvester/issues/8163)
- **Harvester Deploy YAML Changes**: [#8116](https://github.com/harvester/harvester/issues/8116), [#8746](https://github.com/harvester/harvester/issues/8746)
- **Rancherd and Harvester Installer**: [#7312](https://github.com/harvester/harvester/issues/7312)
- **Node-Disk-Manager Feature**: [#8296](https://github.com/harvester/harvester/issues/8296)
- **PCI Device Add-on**: [#6779](https://github.com/harvester/harvester/issues/6779)