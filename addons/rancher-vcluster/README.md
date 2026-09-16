# rancher-vcluster

The rancher-vcluster addon leverages [vcluster](https://github.com/loft-sh/vcluster) to create a vcluster named `rancher-vcluster` in the `harvester-system` namespace.

The addon will also inject additional manifests to configure and install a fully functional rancher.

The rancher will be using a self signed certificate.

Users can define the rancher version and rancher url via the `valuesContent` section in the addon.

```
    hostname: "rancher.172.19.108.3.sslip.io"
    rancherVersion: "v2.7.4"
```

The vcluster will sync the ingress for the newly deployed rancher to the underlying harvester cluster.

Users need to ensure that the `hostname` defined for accessing rancher is accessible via a DNS record pointing to the harvester vip.

Updates to `rancherVersion` in the contentValues can be used to trigger rancher upgrades in the vcluster.

Similar workflow can also be used to trigger k3s upgrades.

*NOTE:* The rancher deployed in vcluster can be used for managing the underlying harvester, including provisioning more downstream clusters. Please be aware that running rancher in vcluster is not as secure as a separate VM based install. A user with cluster level or project admin access to harvester-system namespace, will be able to access the rancher deployed in the vcluster.