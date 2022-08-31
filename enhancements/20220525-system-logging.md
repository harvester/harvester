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

We will deploy a new `ManagedChart` in the [Harvester Installer](https://github.com/harvester/harvester-installer) to
install. The `ManagedChart` will deploy a `ClusterFlow` to select and aggregate interesting logs, which can be managed
by a new `ClusterOutput` crd.

To collect the host system logs, a new harvester sub-chart [harvester-journal](https://github.com/harvester/charts/tree/master/charts/harvester-journal).

### User Stories

#### Easily View Harvester Logs

Currently, users need to manually check harvester for failing pods or services and manually check logs using `kubectl`.

This enhancement will allow users to send their logs using any of the [output plugins]().

### User Experience In Detail

The user should be able to view harvester logs via UI, and configure where the logs are sent.

### API changes

None.

## Design

### Implementation Overview

- Install a `harvester-logging` managed chart defining a `ClusterFlow` and `ClusterOutput`
  - By default we send logs to loki
- Add a new sub chart to the harvester

### Test plan

1. Install harvester with the implemented charts and CRDs
2. Verify logs are being routed to the configured ClusterOutput
3. Change log output settings
4. Verify logs are routed to the newly configured ClusterOutput

### Upgrade strategy

No user intervention is required during the upgrade.
