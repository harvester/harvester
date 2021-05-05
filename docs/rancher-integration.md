#  Rancher Integration

>  Available as of v0.2.0

[Rancher](https://github.com/rancher/rancher) is an open source multi-cluster management platform. Harvester has integrated Rancher into its HCI mode installation by default.

## Enable Rancher Dashboard

Users can enable the Rancher dashboard by going to the Harvester `Settings` page.

1. Click the actions of the `rancher-enabled` setting.
1. Select the `Enable` option and click the save button.
1. On the top-right corner the Rancher dashboard button will appear.
1. Click the Rancher button, and it will open a new tab to navigate to the Rancher dashboard.

For more detail about how to use the Rancher, you may refer to this [doc](https://rancher.com/docs/rancher/v2.5/en/).



# Creating K8s Clusters using the Harvester Node Driver

Harvester node driver is used to provision VMs in the Harvester cluster, which Rancher uses to launch and manage Kubernetes clusters.

In the ISO mode, the Harvester driver has been added by default. Users can reference this [doc](./node-driver.md) for more details.
