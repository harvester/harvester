# Harvester Helm Chart

## Chart Details

This chart will do the following:

- Deploy a KubeVirt Operator if needed, defaults to deploy.
- Deploy the Harvester resources.
- Deploy [Longhorn](https://longhorn.io) as the built-in storage management.
- Deploy Containerized Data Importer (CDI) to support third-party storage management.
- Deploy csi-snapshotter
- Deploy Snapshot Validation Webhook
- Deploy whereabouts

## Introduction

Harvester uses a `ManagedChart` resource to deploy its core components. This is configured via the [harvester-installer](https://github.com/harvester/harvester-installer/blob/master/pkg/config/templates/rancherd-10-harvester.yaml). Be aware that `deploy/harvester` and `deploy/harvester-crd` are two separate `ManagedChart` resources.

Once a `ManagedChart` is created, Fleet generates a `Bundle` resource to trigger Helm and install the chart. You can learn more about this process in the [Fleet documentation](https://fleet.rancher.io/concepts).

During upgrades, Harvester runs a script to patch the release version in the `ManagedChart`. This happens in the [`upgrade_manifests.sh`](https://github.com/harvester/harvester/blob/50da36ac3b751c1a1dbfc8d25e5499a4c6216450/package/upgrade/upgrade_manifests.sh#L885-L935) script.


## For Developers

### How to bump dependency charts

1. Make sure the correct dependency version is specified in `go.mod`.
2. Run `go mod vendor`.
3. Update the `Chart.yaml` and `values.yaml` files with the correct version numbers.
4. In the `deploy/charts/harvester` directory, run `helm dependency update .`
   
If the dependency doesn’t involve a change in go.mod, you can start from step 3.

Here are example PRs for reference:

- https://github.com/harvester/harvester/pull/8233
- https://github.com/harvester/harvester/pull/8210

### Use case

To use Longhorn release candidates for testing purposes, download the Longhorn charts and place them in `deploy/charts/harvester/dependency_charts/longhorn`.

Since we're using a non-official version, we need to modify the Chart.yaml to point to the local folder instead of the remote repository:

```diff
-  repository: https://charts.longhorn.io
-  version: 1.7.1
+  repository: file://dependency_charts/longhorn
+  version: 1.7.2-rc2
```

## License
Copyright (c) 2025 [SUSE, LLC.](https://www.suse.com/)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
