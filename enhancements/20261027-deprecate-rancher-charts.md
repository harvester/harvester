# Deprecate CSI Driver and Cloud Provider Rancher Charts

## Summary

This HEP proposes to deprecate the Harvester CSI driver and cloud provider Helm
charts from the `rancher/charts` repository, and only publish them to the
`rancher/rke2-charts` repository. This reduces the monthly maintenance overhead,
context-switching and interruptions associated with maintaining and publishing
the charts, which requires constant alignment with both Rancher and RKE2 release
cycles.

### Related Issues

<https://github.com/harvester/harvester/issues/11999>

## Motivation

### Goals

#### Goal 1 - Deprecate charts from `rancher/charts` repository

The only reason we keep the charts in the `rancher/charts` repository is to
provide guest cluster operators with an out-of-band upgrade path to change the
Harvester CSI driver and cloud provider versions.

This unsupported workflow causes a drift in the image version referenced by the
underlying Rancher `Apps` and RKE2 `HelmChart` resources. When the
`helm-controller` reconciles the `HelmChart` resource, it will revert the image
version back to the one specified in the chart, which may not be what the user
would expect.

✏️ Currently, the Rancher UI displays a warning message to inform user that directly
upgrading these components using the Rancher charts is unsupported.

#### Goal 2 - Retain charts in `rancher/rke2-charts` repository

The publishing of the charts to the `rancher/rke2-charts` repository remains
intact because they are needed to bootstrap RKE2 clusters on Harvester-managed
virtual machines. Nothing here will change.

#### Goal 3 - Provide guidance for out-of-band upgrade of component images

Following discussion with RKE2 team, the recommended approach for out-of-band
upgrades of images of bundled system component is to either:

* use `HelmChartConfig` to override the image version for non-prime setup, or
* use `rke2-patch` to override the image version for prime setup

Both approaches require post-upgrade configuration to remove the overrides.

In general, it is recommended to prefer a cluster upgrade over an out-of-band
upgrade of the component images.

#### Goal 4 - No changes to the charts, code base, pipelines or release process

This proposal does not require any changes to the charts, code base, pipelines or
release process.

### Non-goals [optional]

N/A.

## Proposal

### User Stories

#### Story 1 - Chart maintainer

As a chart maintainer, I don't want to keep maintaining and publishing Helm
charts to multiple repositories where the benefits and use cases are not
immediately clear. I want to minimize context-switching and interruptions, and
avoid escalation from other teams due to unforeseen last-minute CVE discoveries.

#### Story 2 - Guest cluster operator

As a RKE2 guest cluster operator, I understand that the primary upgrade path for
the Harvester CSI driver and cloud provider involves upgrading my RKE2 cluster.
However, I also understand that there may be scenarios where I need to upgrade
the component images out-of-band, and I want to have the option to do so if necessary.

### User Experience In Detail

In general, user no longer interacts with the charts in the "App" page of the
guest cluster. The UI should mark these charts as deprecated. The real charts
remains bundled as part of the RKE2 installation and upgrade.

For out-of-band upgrades, non-prime user follows our documentation to create
`HelmChartConfig` resources to override the image, and then perform manual
post-upgrade cleanup to remove the overrides. This proposal doesn't provide any
automated solution as this should not be considered a primary mechanism for
upgrading the component images.

For prime setup, we will discuss with RKE2 team to extend `rke2-patcher` to support
the CSI driver and cloud provider images. This provides prime users with the
canonical way to perform out-of-band upgrades on bundled system components.

### API Changes

N/A

## Design

### Implementation Overview

On `rancher/charts` repository, stop publishing new releases. We can't remove
existing releases from the repository because they are already referenced by Rancher.

TODO: find out if Rancher UI can mark chart as deprecated. We can't mark the
charts themselves as deprecated because they are still needed in RKE2.

On `rancher/rke2-charts` repository, continue publishing new releases as normal.

In the documentation, provide instructions to perform out-of-band upgrades of the
component images using `HelmChartConfig` in non-prime setup, follow by manual post-
upgrade cleanup.

Example, to update the Harvester CSI driver image to `v0.2.11`:

```sh
cat<<EOF | kubectl apply -f -
apiVersion: helm.cattle.io/v1
kind: HelmChartConfig
metadata:
  labels:
  name: harvester-csi-driver
  namespace: kube-system
spec:
  failurePolicy: reinstall
  valuesContent: |
    image:
      harvester:
        csiDriver:
          repository: rancher/harvester-csi-driver
          tag: v0.2.11
EOF
```

TODO: discuss with RKE2 team to extend `rke2-patcher` to support Harvester CSI
driver and cloud provider.

### Test plan

Testing is scoped to the verification of the out-of-band upgrade process for the
Harvester CSI driver and cloud provider images, ensuring that the overrides work
as expected and that manual post-upgrade cleanup is successful.

### Upgrade strategy

No official upgrade strategy is required for this enhancement.

For users who had previously performed out-of-band upgrade using the Rancher
charts, we coulld provide documentation on how to identify drifts between the
Rancher `App` and the RKE2 `HelmChart` resources, and how to reconcile them.

## Note [optional]

This section addresses the question _What happens when user overrides the CSI
driver or cloud provider with the Rancher "Apps" charts_.

⚠️ This workflow is unsupported and not recommended.

Key custom resource definitions (CRDs) involved in this workflow:

`HelmChart` represents a Helm chart. It's managed by the `helm-controller` and
contains the embedded base64-encoded chart tarball.

`HelmChartConfig` represents values override of a Helm chart. It's managed by the
`helm-controller` and contains YAML or JSON snippet of overide values.

`App` represents a runtime release of a Helm chart. It's managed by Rancher for
every Helm release found on the cluster, regardless of who created it.

In summary, when overriding the bundled charts with the Rancher charts, the `App`
resources are overridden (with a new version), but the `HelmChart` resources are
not. Rancher creates an `Operation` job to execute `helm upgrade` in a
`helm-operation` pod.

The overrides in the `App` resource and its secrets (usually named as
`sh.helm.release.v1.<chart-name>.<release-number>`) are persistent despite
system restart. However, when the `helm-controller` reconciles the `HelmChart`
resources again, it will revert the image version back to the one specified in
the chart.

Similarly, during the next RKE2 upgrade, the `HelmChart` resources are updated
which also triggers the `helm-controller` reconciliation. The `App` resource and
its secrets are then overridden again.
