# rancher-k3k

The rancher-k3k addon leverages [k3k](https://github.com/rancher/k3k) to create a shared k3k cluster named `rancher-k3k-cluster` in the `rancher-k3k` namespace.

The addon depends on the [k3k addon](../k3k) which needs to be installed before installing this addon.

The addon will also inject additional manifests to configure and install a fully functional rancher.

The rancher will be using a self signed certificate.

Users can define the rancher version and rancher url via the `valuesContent` section in the addon.

```
rancher:
  hostname: "rancher.harvester_vip.sslip.io"
  version: "v2.14.0"
  replicas: 1
  bootstrapPassword: "your_secure_password"
  repo: "rancherChartRepo"
k3kCluster:
  servers: 1
  version: v1.35.4-k3s1
  storageClassName: "harvester-longhorn"
```

The k3k cluster will sync the ingress for the newly deployed rancher to the underlying harvester cluster.

Users need to ensure that the `hostname` defined for accessing rancher is accessible via a DNS record pointing to the harvester vip.

Updates to `version` in the contentValues can be used to trigger rancher upgrades in the rancher installed in k3k.

Similar workflow can also be used to trigger k3s upgrades.

*NOTE:* The rancher deployed in k3k can be used for managing the underlying harvester, including provisioning more downstream clusters. Please be aware that running rancher in k3k is not as secure as a separate VM based install. A user with cluster level or project admin access to harvester-system namespace, will be able to access the rancher deployed in k3k.