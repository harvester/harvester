# k3k

The k3k addon installs the [k3k](https://github.com/rancher/k3k) controller in the `k3k-system` namespace. k3k runs K3s clusters as workloads on the underlying Harvester cluster.

The addon is a prerequisite of the [rancher-k3k addon](../rancher-k3k) and needs to be installed and enabled before it.

The chart is pulled from the upstream k3k chart repository (`https://rancher.github.io/k3k`), so its version is not tied to the Harvester release.
