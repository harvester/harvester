# Harvester Helm Chart [Development]

Rancher Harvester is an open source Hyper-Converged Infrastructure(HCI) solution based on Kubernetes.

Note: the master branch is under active development, please use the `stable` branch if you need a stable version of the Harvester chart.

## Chart Details

This chart will do the following:

- Deploy a KubeVirt Operator if needed, defaults to deploy.
- Deploy a KubeVirt CRD resource to enable KubeVirt if needed, defaults to deploy.
- Deploy a KubeVirt Containerized Data Importer(CDI) Operator if needed, defaults to deploy.
- Deploy a CDI CRD resource to enable KubeVirt Containerized Data Importer(CDI) if needed, default to deploy.
- Deploy the Harvester resources.
- Deploy [Longhorn](https://longhorn.io) as the built-in storage management.
- [Multus-CNI](https://github.com/intel/multus-cni) as default multi networks management solution(enable by set `multus.enabled=true` in the helm install).

### Prerequisites

- Kubernetes 1.16+.
- Helm 3.2+.

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

- Use the existing KubeVirt/CDI/Longhorn Service.

    If you have already prepared the KubeVirt, CDI or Longhorn, you can disable these installations in this chart.
    
    ```bash
    $ helm install harvester harvester --namespace harvester-system \
        --set kubevirt.enabled=false --set kubevirt-operator.enabled=false \
        --set cdi.enabled=false --set cdi-operator.enabled=false \
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

#### Configure KubeVirt Containerized Data Importer Operator

To configure the KubeVirt Containerized Data Importer Operator, you need to know its [parameters](dependency_charts/cdi-operator/values.yaml) and put all items under `cdi-operator` domain.

For example, if you want to change the image name of CDI cloner, you can do as below.

```bash
$ helm install harvester harvester --namespace harvester-system \
    --set-string cdi-operator.containers.cloner.image.repository=myrepo/my-cloner-image
```

If you don't want to install KubeVirt Containerized Data Importer Operator, you can do the following.

```bash
$ helm install harvester harvester --namespace harvester-system \
    --set cdi-operator.enabled=false
```

#### Configure KubeVirt Containerized Data Importer (CRD resource)

To configure the KubeVirt Containerized Data Importer CRD resource, you need to know its [parameters](dependency_charts/cdi/values.yaml) and put all items under `cdi` domain.

For example, if you want to override the pull policy of CDI operator, you can do as below.

```bash
$ helm install harvester harvester --namespace harvester-system \
    --set-string cdi.spec.imagePullPolicy=Always
```

If you don't want to install KubeVirt Containerized Data Importer CRD resource, you can do the following.

```bash
$ helm install harvester harvester --namespace harvester-system \
    --set cdi.enabled=false
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
