# Node Manager Support KSM

## Summary

Support running [KSM](https://www.kernel.org/doc/html/latest/admin-guide/mm/ksm.html) on Harvester nodes to reduce its memory usage, and planned to integrate it into the ISO release directly.

### Related Issues

[#2302](https://github.com/harvester/harvester/issues/2302)

## Motivation

### Goals

- Configure Ksmtuned from UI or API, supporting multiple configuration modes.
- Monitor node ksmd CPU utilization [#2743](https://github.com/harvester/harvester/issues/2743).
- Convert Ksmtuned script to Golang program and add unit test.

### Non-goals [optional]

No.

## Proposal

Implement a Ksmtuned controller to manage the Harvester node KSM configurations.

### User Stories

### Configure each node’s KSM parameters

Configuring KSM requires logging into each node and using the command line, which increases operational complexity and security risks.

We would use the Ksmtuned implementation, where Ksmtuned parameters can be configured using the UI or API.

Supports three running options：

- **Stop:** Stop Ksmtuned and KSM. VMs can still use shared memory pages.
- **Run:** Run Ksmtuned.
- **Prune:** Stop Ksmtuned and prune KSM memory pages.

Provides three configuration modes：

- **Standard:** The default mode. The control node ksmd uses about 20% of a single CPU. It uses the following parameters:

    ```yaml
    boost: 0
    decay: 0
    minPages: 100
    maxPages: 100
    sleepMsec: 20
    ```

- **High Performance:** Node ksmd uses 20% to 100% of a single CPU and has higher scanning and merging efficiency. It uses the following parameters:

    ```yaml
    boost: 200
    decay: 50
    minPages: 100
    maxPages: 10000
    sleepMsec: 20
    ```

- **Customized:** You can customize the configuration to reach the performance that you want.

### Monitoring ksmd utilization

Expose ksmd CPU utilization metrics for Prometheus to scrape.

### User Experience In Detail

**UI**

KSM configurations will be displayed or configured in Hosts → info/edit → KSM page.

Add ksmd utilization top10‘s chart in Cluster Metrics on the Dashboard page.

### API changes

No. The first implementation is included in the GA release 1.1.0.****

## Design

### Implementation Overview

Using `DaemonSet` deployed to run in each node, the node will only get the configuration information it is related to.

It writes to the KSM file as configured, gets the node's memory usage at regular intervals, and starts or stops the ksmd program according to the threshold calculation.

**Ksmtuned CR**

Configuration of management node ksmtuned via Ksmtuned CR.

When `spec.mode` is standard/high mode, `spec.ksmtunedParameters` displays the configuration parameters.

```yaml
apiVersion: node.harvesterhci.io/v1beta1
kind: Ksmtuned
metadata:
  name: node1
  ownerReferences:
  - apiVersion: v1
    kind: Node
    name: node1
spec:
  ksmtunedParameters:
    boost: 0
    decay: 0
    maxPages: 100
    minPages: 100
    sleepMsec: 20
  mergeAcrossNodes: 0
  mode: standard/high/customized
  run: stop/run/prune
  thresCoef: 20
status:
  fullScans: 17
  ksmdPhase: Stopped/Running/Pruned
  shared: 171264
  sharing: 1368772
  stableNodeChains: 12
  stableNodeDups: 538
  unshared: 545943
  volatile: 10909
```

Ksmtuned controller checks that the node's Ksmtuned CR does not exist and a CR with the standard mode is automatically created for the node.

When `spec.mode` is the customized mode, you need to configure `spec.ksmtunedParameters`.

```yaml
spec:
  run: run
  mode: customized
  thresCoef: 20
  ksmtuneddParameters:
    boost: 300
    decay: 50
    maxPages: 2000
    minPages: 100
    sleepMsec: 20
```

When `spec.run` is `stop`, it will suspend running ksmtuned, but continue to keep the shared pages.

```yaml
apiVersion: harvesterhci.io/v1beta1
kind: Ksmtuned
metadata:
  name: harvester-master
spec:
  ksmtunedParameters:
    boost: 0
    decay: 0
    maxPages: 100
    minPages: 100
    sleepMsec: 20
  mergeAcrossNodes: 0
  mode: standard
  run: stop
  thresCoef: 20
status:
  sharing: 17299179
  shared: 312514
  unshared: 2334736
  volatile: 65571 
  fullScans: 14102
  stableNodeChains: 30
  stableNodeDups: 56249
  ksmdPhase: Stopped
```

When `spec.run` is `prune`, it will suspend ksmtuned and clean up the shared memory pages.

```yaml
apiVersion: harvesterhci.io/v1beta1
kind: Ksmtuned
metadata:
  name: harvester-master
spec:
  ksmtunedParameters:
    boost: 0
    decay: 0
    maxPages: 100
    minPages: 100
    sleepMsec: 20
  mergeAcrossNodes: 0
  mode: standard
  run: prune
  thresCoef: 20
status:
  sharing: 0
  shared: 0
  unshared: 0
  volatile: 0 
  fullScans: 0
  stableNodeChains: 0
  stableNodeDups: 0
  ksmdPhase: Pruned
```

Ksmtuned CR’s status to display [KSM metrics](https://www.kernel.org/doc/html/latest/admin-guide/mm/ksm.html#ksm-daemon-sysfs-interface) and ksmd phase.

```yaml
apiVersion: harvesterhci.io/v1beta1
kind: Ksmtuned
metadata:
  name: harvester-master
spec:
  ...
status:
  sharing: 17299179
  shared: 312514
  unshared: 2334736
  volatile: 65571 
  fullScans: 14102
  stableNodeChains: 30
  stableNodeDups: 56249
  ksmdPhase: Stopped/Running/Pruned
```

**Ksmtuned script**

Since KSM runs in a single mode, it is possible for large memory devices to occupy the CPU for a long time. To solve this problem, you can set the ksmtuned script threshold to run KSM only when the free memory is less than a certain percentage.

`Ksmtuned.spec` parameters：

- **run:** `stop`/`run`/`prune`, stop/run Ksmtuned or prune share pages。
- **mode:** `standard`/`high`/`customized`.
- **thresCoef:** Free memory threshold, KSM starts when the memory available is less than the set percentage ratio, otherwise KSM stops.
- **mergeAcrossNodes:** specifies if pages from different NUMA nodes can be merged.
- **ksmtunedParameters.boost:** Increment the number of pages scanned at a time, incrementing if the memory availability is less than the threshold.
- **ksmtunedParameters.decay:** Decrement the number of pages scanned at a time, decrementing if memory availability is greater than the threshold.
- **ksmtunedParameters.maxPages:** Maximum number of pages per scan.
- **ksmtunedParameters.minPages:** The minimum number of pages per scan, i.e. the configuration for the first run.
- **ksmtunedParameters.sleepMsec:** Each time you change the configuration interval, calculate the interval between two scans by the formula.(`sleepMsec * 16 * 1024 * 1024 / total memory` ).

**Node Controller**

- When synchronizing a Node, if the Ksmtuned CR does not exist, it is automatically created for that node, **mode is standard, thresCoef is 20 and status is stop**.
- When the Node is deleted, the Ksmtuned CR of the node is deleted.

**Ksmtuned Controller**

- When Ksmtuned CR is created/updated, the running state will adapt the predefined or custom configuration, otherwise, stop running the program.
- Terminates the ksmtuned program when Ksmtuned CR is deleted.

**Metrics**

- ksmd program CPU utilization metrics.

### Test plan

Set `thresCoef` to control KSM run, e.g. set `thresCoef: 20` to run KSM when total available memory is less than total memory * 0.2, then `cat /sys/kernel/mm/ksm/run` will output `1` and the reverse will output `0`.

To test the configuration `boost`、`decay`、`maxPages`、`minPages`, you can use `wathc -n1 grep . /sys/kernel/mm/ksm/*` to observe the change in pages_to_scan.

1. Prepare a node with enough memory and a large number of virtual machines already created.
2. Check the Ksmtuned CR that automatically creates this node.
3. Adjust the percentage ratio of `thresCoef` in Ksmtuned by node or test expectation.
4. Adjust the values of `boost/decay/maxPages/minPages` in Ksmtuned by node or test expectation.
5. Check the `status` of Ksmtuned CR or log in to the system to observe it.

### Upgrade strategy

No.

## Note [optional]

No.