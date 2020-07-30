# KubeVirt Containerized Data Importer Helm Chart

[`CDI`](https://github.com/kubevirt/containerized-data-importer/blob/dbab72c93e05957fe8eb5d8d861843d79f7998bf/pkg/apis/core/v1beta1/types.go#L230-L245) declaratively defines a desired [KubeVirt Containerized Data Importer](https://github.com/kubevirt/containerized-data-importer) setup to run in a Kubernetes cluster. It provides options to configure KubeVirt Containerized Data Importer bundle components to deploy.

The current configuration management of KubeVirt Containerized Data Importer Operator for `CDI` resource has some limitations as below:

- `CDI` resource is a cluster scope resource.
- Only the first deployed one in multiple `CDI` resources is valid.

## Chart Details

This chart will do the following:

- Deploy a `CDI` CRD resource named `cdi`.

### Prerequisites

- Kubernetes 1.16+.
- Helm 3.2+.
- KubeVirt Containerized Data Importer Operator v1.21.0+.

### Installing the Chart

To install the chart with the release name `my-release`.

```bash
$ # install chart to target namespace installed cdi-operator, i.e: harvester-system
$ helm install my-release cdi --namespace harvester-system
```

### Uninstalling the Chart

To uninstall/delete the `my-release` release.

```bash
$ # uninstall chart from target namespace
$ helm uninstall my-release --namespace harvester-system
```

### Configuration

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install` or `helm upgrade`.

For details on using parameters, please refer to [values.yaml](values.yaml).

## License
Copyright (c) 2020 [Rancher Labs, Inc.](http://rancher.com)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
