# CSI Snapshotter

The [CSI snapshotter](https://github.com/kubernetes-csi/external-snapshotter) is part of Kubernetes implementation of Container Storage Interface (CSI).

The volume snapshot feature supports CSI v1.0 and higher. It was introduced as an Alpha feature in Kubernetes v1.12 and has been promoted to an Beta feature in Kubernetes 1.17. In Kubernetes 1.20, the volume snapshot feature moves to GA.

Introduction
------------

This chart installs the csi-snapshotter on the [harvester](https://github.com/harvester/harvester) cluster using the [Helm](https://helm.sh) package manager.

For more details please find out in:

- https://github.com/kubernetes-csi/external-snapshotter/tree/master/deploy/kubernetes/snapshot-controller
