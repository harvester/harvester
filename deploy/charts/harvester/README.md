# Harvester Helm Chart [Development]

Harvester is an open source Hyper-Converged Infrastructure(HCI) solution based on Kubernetes.

Note: the master branch is under active development, please use the `stable` branch if you need a stable version of the Harvester chart.

## Chart Details

This chart will do the following:

- Deploy a KubeVirt Operator if needed, defaults to deploy.
- Deploy a KubeVirt CRD resource to enable KubeVirt if needed, defaults to deploy.
- Deploy the Harvester resources.
- Deploy [Longhorn](https://longhorn.io) as the built-in storage management.

### Prerequisites

- Kubernetes 1.16+.
- Helm 3.2+.
- [Multus CNI](https://github.com/k8snetworkplumbingwg/multus-cni). An [example chart](https://github.com/rancher/rke2-charts/tree/main-source/packages/rke2-multus/charts).
- [Harvester CRDs](https://github.com/harvester/harvester/tree/master/deploy/charts/harvester-crd)

### Installing the Chart

To install the chart with the release name `harvester`.

```bash
## create target namespace
$ kubectl create ns harvester-system

## install chart to target namespace
$ helm install harvester harvester --namespace harvester-system --set service.harvester.type=NodePort
```

### Uninstalling the Chart

To uninstall/delete the `harvester` release.

```bash
## uninstall chart from target namespace
$ helm uninstall harvester --namespace harvester-system
```

#### Notes

- Use the existing KubeVirt/Longhorn Service.

    If you have already prepared the KubeVirt or Longhorn, you can disable these installations in this chart.
    
    ```bash
    $ helm install harvester harvester --namespace harvester-system \
        --set kubevirt.enabled=false --set kubevirt-operator.enabled=false \
        --set longhorn.enabled=false
    ```

- Use other storage drivers.
    
    Currently, storage drivers other than Longhorn are not supported.

### Configuration

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install` or `helm upgrade`.

For details on using parameters, please refer to [values.yaml](values.yaml).

#### Configure KubeVirt Operator

To configure the KubeVirt Operator, you need to know its [parameters](dependency_charts/kubevirt-operator/values.yaml) and put all items under `kubevirt-operator` domain.

For example, if you want to change the Secret name of admission webhooks, you can do as below.

```bash
$ helm install harvester harvester --namespace harvester-system \
    --set-string kubevirt-operator.containers.operator.certificates.admissionWebhook.secretName=mysecret
```

If you don't want to install KubeVirt Operator, you can do the following.

```bash
$ helm install harvester harvester --namespace harvester-system \
    --set kubevirt-operator.enabled=false
```

#### Configure KubeVirt (CRD resource)

To configure the KubeVirt CRD resource, you need to know its [parameters](dependency_charts/kubevirt/values.yaml) and put all items under `kubevirt` domain.

> **It is worth noting that almost all KubeVirt parameters are string type, including bool field and numeric field.**

For example, if you want to enable the emulation mode on the non-KVM supported hosts, you can do as below.

```bash
$ helm install harvester harvester --namespace harvester-system \
    --set-string kubevirt.spec.configuration.developerConfiguration.useEmulation=true
```

If you don't want to install KubeVirt CRD resource, you can do the following.

```bash
$ helm install harvester harvester --namespace harvester-system \
    --set kubevirt.enabled=false
```

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
