# KubeVirt Operator Helm Chart

KubeVirt Operator provides an easy way to deploy and manage the [KubeVirt](https://github.com/kubevirt/kubevirt) bundle components in Kubernetes.

[KubeVirt repo](https://github.com/kubevirt/kubevirt) has already provided an all-in-one YAML for deploying [the KubeVirt Operator](https://github.com/kubevirt/kubevirt/blob/master/manifests/release/kubevirt-operator.yaml.in), but it is not flexible for configuring. This chart is going to supplement the all-in-one YAML and provide a way to deploy the KubeVirt Operator in air-gap environment.

## Chart Details

This chart will do the following:

- Deploy the KubeVirt Operator.

### Prerequisites

- Kubernetes 1.16+.
- Helm 3.2+.

### Installing the Chart

To install the chart with the release name `my-release`.

```bash
$ # create target namespace
$ kubectl create ns harvester-system

$ # install chart to target namespace
$ helm install my-release kubevirt-operator --namespace harvester-system
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
