#  Harvester Node Driver

The Harvester node driver is used to provision VMs in the Harvester cluster. In this section, you'll learn how to configure Rancher to use the Harvester node driver to launch and manage Kubernetes clusters.

A node driver is the same as a [Docker Machine driver](https://docs.docker.com/machine/drivers/), and the project repo is available at [harvester/docker-machine-driver-harvester](https://github.com/harvester/docker-machine-driver-harvester).

>  Available as of v0.2.0

## Create Cluster

Now users can access the Rancher UI from Harvester, spin up Kubernetes clusters on top of the Harvester cluster, and manage them there.
> Prerequisite: VLAN network is required for Harvester node driver

1. From the **Global** view, click **Add Cluster**.
1. Click **Harvester**.
1. Select a [Template](#create-node-template).
1. Fill out the rest of the form for creating a cluster.
1. Click **Create**.

See [launching kubernetes and provisioning nodes in an infrastructure provider](https://rancher.com/docs/rancher/v2.5/en/cluster-provisioning/#launching-kubernetes-and-provisioning-nodes-in-an-infrastructure-provider) for more info.

## Create Node Template
You can use the Harvester node driver to create node templates and eventually node pools for your Kubernetes cluster.

1. Configure  **Account Access**. For Harvester embedding Rancher, you can choose **Internal Harvester**,  which will use the  `harvester.harvester-system` as the default `Host`, `8443` as the default `Port`.
1. Configure **Instance Options**
    * Configure the CPU, memory, disk, and disk bus.
    * Select an OS image that is compatible with the `cloud-init` config.
    * Select a network that the node driver is able to connect to, currently only `VLAN` is supported.
    * Enter the SSH User, the username will be used to ssh to nodes. For example, a default user of the Ubuntu cloud image will be `ubuntu`.
1. Enter a **RANCHER TEMPLATE** name.

See [nodes hosted by an infrastructure provider](https://rancher.com/docs/rancher/v2.5/en/cluster-provisioning/rke-clusters/node-pools/) for more info.

## How to add Harvester Node Driver in dev mode

Developer mode doesn't come with Harvester node driver pre-installed. Following the steps below to install it:

1. Navigate to the **Rancher** UI.
1. From the **Global** view, choose **Tools > Drivers** in the navigation bar. From the **Drivers** page, select the **Node Drivers** tab. In versions before v2.2.0, you can select **Node Drivers** directly in the navigation bar.
1. Click **Add Node Driver**.
1. Enter **Download URL**([docker-machine-driver-harvester](https://github.com/harvester/docker-machine-driver-harvester/releases)) and **Custom UI URL**([ui-driver-harvester](https://github.com/harvester/ui-driver-harvester/releases)). 
1. Add domains to the **Whitelist Domains**.
1. Click **Create**.
