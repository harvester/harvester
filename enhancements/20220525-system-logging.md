# System Logging

## Summary

We need to be able to support exporting harvester system logs outside the cluster, to allow for collecting and analyzing those logs to aid in debugging and harvester management.

### Related Issues

- https://github.com/harvester/harvester/issues/577
- https://github.com/harvester/harvester/issues/2644
- https://github.com/harvester/harvester/issues/2645
- https://github.com/harvester/harvester/issues/2646
- https://github.com/harvester/harvester/issues/2647

## Motivation

### Goals

List the specific goals of the enhancement. How will we know that this has succeeded?

- [X] The user can aggregate harvester logs in a centralized location
  - [X] k8s cluster logs
  - [X] host system logs (ie `/var/log`)
- [X] The user can export the aggregated harvester logs outside the cluster (ex rancher)

### Non-goals

- this enhancement does not cover integration into the harvester / rancher UI (but ideally this will eventually be implemented as well)

## Proposal

Install [`rancher-logging`](https://github.com/rancher/charts/tree/dev-v2.7/charts/rancher-logging/100.1.3%2Bup3.17.7)
as a `ManagedChart` in the [Harvester Installer](https://github.com/harvester/harvester-installer) to collect logs from
the Harvester cluster.

To collect the host system logs on each node, we will patch and enable `rancher-logging`'s `rke2` logging source. This
will deploy a `DaemonSet` to mount each node's `/var/log/journal`. Collecting all logs under `/var/log/journal` is too
much, so we will add systemd filters to the deployed `fluent-bit` pod to only collect logs from the kernel and important
services:

```
        Systemd_Filter    _SYSTEMD_UNIT=rke2-server.service
        Systemd_Filter    _SYSTEMD_UNIT=rke2-agent.service
        Systemd_Filter    _SYSTEMD_UNIT=rancherd.service
        Systemd_Filter    _SYSTEMD_UNIT=rancher-system-agent.service
        Systemd_Filter    _SYSTEMD_UNIT=wicked.service
        Systemd_Filter    _SYSTEMD_UNIT=iscsid.service
        Systemd_Filter    _TRANSPORT=kernel
```

Users will be able to configure where to send the logs by applying `ClusterOutput` and `ClusterFlow` to the cluster, but
by default none will be added.

Once installed the cluster should have the pods below:

```
NAME                                                      READY   STATUS      RESTARTS   AGE
rancher-logging-685cf9664-w4wl2                           1/1     Running     0          17m
rancher-logging-rke2-journald-aggregator-6zfsp            1/1     Running     0          17m
rancher-logging-root-fluentbit-hj72q                      1/1     Running     0          15m
rancher-logging-root-fluentd-0                            2/2     Running     0          15m
rancher-logging-root-fluentd-configcheck-ac2d4553         0/1     Completed   0          17m
```

### User Stories

#### Easily View Harvester Logs

Currently, users need to manually check harvester for failing pods or services and manually check logs using `kubectl`.

This enhancement will allow users to send their logs using any of the [output plugins](https://banzaicloud.com/docs/one-eye/logging-operator/configuration/plugins/outputs/),
and be able to look at and filter the logs of the entire cluster without being limited to 1 pod container at a time.

#### Configure New Logging Outputs

Users will be able to write and apply their own custom `ClusterOutput`s and `ClusterFlow`s to send logs to the desired 
location. For example, to [send logs to graylog](https://banzaicloud.com/docs/one-eye/logging-operator/configuration/plugins/outputs/), 
you can use the following yamls:

```yaml
# graylog-cluster-flow.yaml
apiVersion: logging.banzaicloud.io/v1beta1
kind: ClusterFlow
metadata:
  name: "all-logs-gelf-hs"
  namespace: "cattle-logging-system"
spec:
  globalOutputRefs:
    - "example-gelf-hs"
```

```yaml
# graylog-cluster-output.yaml
apiVersion: logging.banzaicloud.io/v1beta1
kind: ClusterOutput
metadata:
  name: "example-gelf-hs"
  namespace: "cattle-logging-system"
spec:
  gelf:
    host: "192.168.122.159"
    port: 12202
    protocol: "udp"
```

You can verify that they are installed successfully, you can check them with `kubectl`:

```bash
>>> kubectl get clusteroutputs -n cattle-logging-systems example-gelf-hs
NAME              ACTIVE  PROBLEMS
example-gelf-hs   true
>>> kubectl get clusterflows -n cattle-logging-systems example-gelf-hs
NAME                ACTIVE  PROBLEMS
all-logs-gelf-hs    true
```

#### Viewing Logs Via Grafana Loki

### Setup

Loki will not be installed to the cluster by default, but you can manually install it:

  1. Install the helm chart: `helm install --repo https://grafana.github.io/helm-charts --values enhancements/20220525-system-logging/loki-values.yaml --version 2.7.0 --namespace cattle-logging-system loki-stack loki-stack`
  2. Apply the `ClusterFlow` and `ClusterOutput`: `kubectl apply -f enhancements/20220525-system-logging/loki.yaml`

After some time for the logging operator to load and apply the `ClusterFlow` and `ClusterOutput` you will see the logs
flowing into loki.

##### Accessing

To view the Loki UI, you can go to port forward to the `loki-stack-grafana` service. For example map `localhost:3000` to `loki-stack-grafana`s port 80:

```bash
kubectl port-forward --namespace cattle-logging-system service/loki-stack-grafana 3000:80
```

##### Authentication

The default username is `admin`. To get the password for the admin user run the following command:

```bash
kubectl get secret --namespace cattle-logging-system loki-stack-grafana --output jsonpath="{.data.admin-password}" | base64 --decode
```

##### Log Querying

Once the UI is open on the left tab you can select the "Explore" tab to take you to a page where you manually send
queries to loki:

![](./20220525-system-logging/loki-explore-tab.png)

In the search bar you can enter queries to select the logs you are interested in. The query language is described in the
[grafana loki docs](https://grafana.com/docs/loki/latest/logql/log_queries/). For example, to select all logs in the
`cattle-logging-system`:

![](./20220525-system-logging/loki-explore-query-kubernetes-namespace-name-cattle-logging-system.png)

Or to select harvester host machine logs from the `rke2-server.service` service:

![](./20220525-system-logging/loki-explore-query-systemd-rke2-server.png)

#### Logging is Enabled by Default
Due to limitations with enabling and disabling `ManagedCharts` the logging feature will be enabled by default, and
cannot be disabled later by the user.

#### Log Storage PVC
Logging is not backed by a PVC, users need to configure a log receiver (see instructions above) in order to view and
store logs.

### User Experience In Detail

### API changes

None.

## Design

### Implementation Overview

The bulk of the logging functionality is handled by installing the [`rancher-logging`](https://github.com/rancher/charts/tree/dev-v2.7/charts/rancher-logging/100.1.3%2Bup3.17.7/)
to deploy the main logging components:

| Name                | Purpose                                                                    |
|---------------------|----------------------------------------------------------------------------|
| logging-operator    | manages the `ClusterFlow`s and `ClusterOutput`s defining log routes        |
| fluentd             | the central log aggregator which will forward logs to other log collectors |
| fluent-bit          | collects the logs from teh cluster pods                                    |
| journald-aggregator | deploys a pod to collect the journal logs from each node                   |

The journald-aggregator is a `fluent-bit` pod which collects the node logs by mounting the host's `/var/log/journal`
directory to the pod. Using the [`systemd`](https://docs.fluentbit.io/manual/pipeline/inputs/systemd) input plugin, the
logs can be filtered by the fields in the log entry (ex `_SYSTEMD_UNIT`, `_TRANSPORT`, etc). Collecting all the jogs
from `/var/log/journal` is too much, so we only select logs from some important services: rke2-server, rke2-agent,
rancherd, rancher-system-agent, wicked, iscsid, and kernal logs.

The logging feature is enabled by default; however, due to an [issue](https://github.com/harvester/harvester/issues/2719)
with enabling and disabling `ManagedChart`s, it cannt be disabled.

### Test plan

1. Install harvester cluster
2. Check that the related pods are created
   1. `rancher-logging`
   2. `rancher-logging-rke2-journald-aggregator`
   3. `rancher-logging-root-fluentbit`
   4. `rancher-logging-root-fluentd-0`
   5. `rancher-logging-root-fluentd-configcheck`
3. Setup a log server, e.g. `Graylog`, `Splunck`, `ElasticSearch`, `Webhook`. Make sure it is network reachable with Harvester cluster VIP.
4. Config `ClusterFlow` and `ClusterOutput` to route to this server
5. Verify logs are being routed to the configured `ClusterOutput`
6. Verify k8s pod logs are received
7. Verify hoist kernel and systemd logs are received

### Upgrade strategy
No user intervention is required during the upgrade. After upgrade, logging should be installed and enabled.
