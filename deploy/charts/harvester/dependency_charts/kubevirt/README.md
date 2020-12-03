# KubeVirt Helm Chart

[`KubeVirt`](https://github.com/kubevirt/kubevirt/blob/6f3f5b489a72c12bef026d6c2c3c90fcc63edbe8/staging/src/kubevirt.io/client-go/api/v1/types.go#L1058-L1067) CRD declaratively defines a desired [KubeVirt](https://github.com/kubevirt/kubevirt) setup to run in a Kubernetes cluster. It provides options to configure KubeVirt bundle components to deploy.

For each `KubeVirt` CRD resource, the [KubeVirt Operator](../kubevirt-operator) deploys the corresponding bundle components in the same Namespace, like [virt-api](https://github.com/kubevirt/kubevirt/tree/master/cmd/virt-api) Deployment, [virt-controller](https://github.com/kubevirt/kubevirt/tree/master/cmd/virt-controller) Deployment and [virt-handler](https://github.com/kubevirt/kubevirt/tree/master/cmd/virt-handler) Deamonset, at the same time, some required CRDs will be created, like [`VirtualMachine`](https://github.com/kubevirt/kubevirt/blob/6f3f5b489a72c12bef026d6c2c3c90fcc63edbe8/staging/src/kubevirt.io/client-go/api/v1/types.go#L817-L833), [`VirtualMachineInstance`](https://github.com/kubevirt/kubevirt/blob/6f3f5b489a72c12bef026d6c2c3c90fcc63edbe8/staging/src/kubevirt.io/client-go/api/v1/types.go#L46-L57) and so on.

The current configuration management of KubeVirt Operator for `KubeVirt` resource has some limitations as below:

- `KubeVirt` resource should deploy into the **same Namespace** as KubeVirt Operator.
- Only the first deployed one in multiple `KubeVirt` resources is valid.

## Chart Details

This chart will do the following:

- Deploy a `KubeVirt` CRD resource named `kubevirt`.

### Prerequisites

- Kubernetes 1.16+.
- Helm 3.2+.
- KubeVirt Operator v0.30.5+.

### Installing the Chart

To install the chart with the release name `my-release`.

```bash
$ # install chart to target namespace installed kubevirt-operator, i.e: harvester-system
$ helm install my-release kubevirt --namespace harvester-system
```

### Uninstalling the Chart

To uninstall/delete the `my-release` release.

```bash
$ # uninstall chart from target namespace
$ helm uninstall my-release --namespace harvester-system
```

### Configuration

It is worth noting that almost all parameters are string type, including bool field and numeric field, please use `--set-string key=value[,key=value]` to indicate. However, for array parameter, please keep using `--set`. 

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
