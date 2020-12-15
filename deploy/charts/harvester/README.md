# Harvester Helm Chart

Rancher Harvester is an open source Hyper-Converged Infrastructure(HCI) solution based on Kubernetes.

## Chart Details

This chart will do the following:

- Deploy a KubeVirt Operator if needed, defaults to deploy.
- Deploy a KubeVirt CRD resource to enable KubeVirt if needed, defaults to deploy.
- Deploy a KubeVirt Containerized Data Importer(CDI) Operator if needed, defaults to deploy.
- Deploy a CDI CRD resource to enable KubeVirt Containerized Data Importer(CDI) if needed, default to deploy.
- Deploy a Minio as virtual machine image storage if needed, defaults to deploy.
- Deploy the Harvester resources.
- Longhorn as build-in storage management(enable by set `longhorn.enabled=true` in the helm install).
- [Multus-CNI](https://github.com/intel/multus-cni) as default multi networks management solution(enable by set `multus.enabled=true` in the helm install).

### Prerequisites

- Kubernetes 1.16+.
- Helm 3.2+.
- **Default** [StorageClass](https://v1-16.docs.kubernetes.io/docs/concepts/storage/storage-classes/) if the Longhorn in Harvester is disabled.

### Installing the Chart

To install the chart with the release name `harvester`.

```bash
## create target namespace
$ kubectl create ns harvester-system

## install chart to target namespace
$ helm install harvester harvester --namespace harvester-system \
    --set longhorn.enabled=true,minio.persistence.storageClass=longhorn,service.harvester.type=NodePort
```

### Uninstalling the Chart

To uninstall/delete the `harvester` release.

```bash
## uninstall chart from target namespace
$ helm uninstall harvester --namespace harvester-system
```

#### Notes

- Use the existing KubeVirt/CDI/Image-Storage Service.

    If you have already prepared the KubeVirt, CDI or image storage compatible with S3 like Minio, you can disable these installations in this chart.
    
    ```bash
    $ helm install harvester harvester --namespace harvester-system \
        --set kubevirt.enabled=false --set kubevirt-operator.enabled=false \
        --set cdi.enabled=false --set cdi-operator.enabled=false \
        --set minio.enabled=false
    ```

- Change the default StorageClass.
    
    By default, the default StorageClass is required for both the Minio and [`DataVolume`](https://github.com/kubevirt/containerized-data-importer#datavolumes). You can follow the [official document](https://kubernetes.io/docs/tasks/administer-cluster/change-default-storage-class/) to change the default StorageClass of Kubernetes cluster.

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

#### Configure Minio

> **Noted:** Minio chart mirrors from the [Minio official](https://github.com/minio/charts).

To configure the Minio, you need to know its [parameters](https://github.com/minio/charts#configuration) and put all items under `minio` domain.

For example, if you want to use "distributed" mode Minio, you can do as below.

```bash
$ helm install harvester harvester --namespace harvester-system \
    --set minio.mode=distributed
```

If you don't want to install Minio, you can do the following.

```bash
$ helm install harvester harvester --namespace harvester-system \
    --set minio.enabled=false
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
