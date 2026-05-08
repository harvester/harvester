# Harvester Benchmark Policy

## Summary

This HEP defines the benchmarking policy for Harvester — what categories we test, how we run them, and what follow-up actions we take on the results. The goal is to establish a repeatable, release-gated process so performance regressions are caught before shipping and improvements can be tracked across releases.

Currently, benchmark runs are ad-hoc: different engineers use different tools, results are not captured consistently, and there is no agreed baseline to compare against. This makes it difficult to reason about performance trends or give users reliable sizing guidance.

### Related Issues

- <https://github.com/harvester/harvester/issues/9777>
- <https://github.com/harvester/harvester/issues/10421>

## Motivation

### Goals

- **Define Benchmark Categories**: Establish canonical categories covering Harvester's performance surface — Resource Footprint, I/O Performance, and KubeVirt Scaling.
- **Standardize Tooling and Methodology**: Specify tools and procedures for each category so results are reproducible by any team member.
- **Maintain a Shared Baseline**: Publish benchmark results to the Harvester wiki each release and compare against the previous baseline.

### Non-goals

- Full automation of benchmark execution.
- Profiling and root-cause analysis (a follow-up action triggered by benchmark results, not part of the benchmark itself).
- Define Regression Criteria: Set thresholds for flagging regressions and a process for acting on them before release.

## Proposal

### User Stories

**Story 1: As a field engineer/PM presenting Harvester to a customer**
I want per-release benchmark results so that I can give concrete sizing guidance instead of guessing. Specifically:

- **Resource footprint**: How much CPU/memory does Harvester itself consume at idle? How much headroom is left for VMs?
- **VM capacity**: What is the maximum number of VMs a given hardware configuration can support before hitting a bottleneck?
- **Storage/network throughput**: What IOPS, bandwidth, and latency can VMs expect under realistic workloads?

**Story 2: As a Harvester software engineer improving performance**
I want profiling data so that I can identify bottlenecks, catch potential OOMs before they hit production, and prove my optimizations work. Specifically:

- **Per-component resource profiling**: CPU/memory consumption of each pod as VM count grows, to understand which component becomes the bottleneck first (e.g., instance-manager at ~20 GiB per pod with 149 volumes).
- **Memory pressure analysis**: Identify components at risk of OOM — comparing scheduler-reserved memory vs actual RSS, and flagging components where actual consumption grows disproportionately with VM count.
- **Upgrade per-phase timing**: Break down upgrade duration into pre-drain, node drain, post-drain, and total, so we can pinpoint which phase is slow and whether optimizations reduce it (see [#10421](https://github.com/harvester/harvester/issues/10421)).
- **VM migration profiling**: Duration and resource cost of live migration under varying VM sizes and storage backends, since migration involves both network and storage I/O.
- **CSI driver throughput under load**: Whether the Harvester CSI driver can keep up during rapid guest node shutdown/recreation before Longhorn times out.

### User Experience In Detail

1. At each release, run the benchmark suite against a clean cluster matching the defined topology.
2. Record results in the wiki under the release version.
3. Compare against the previous release baseline and publish in release notes.

### API changes

N/A

## Design

### Implementation Overview

The benchmark policy is organized into three categories. Each defines scope, tooling, topology, methodology, and key metrics.

#### Category 1: Resource Footprint

- **Scope**: CPU and memory consumption of each Harvester component at idle and under maximum VM density.
- **Tooling**:
  - `kubectl top pods` — per-pod CPU/memory usage.
  - `ps -C containerd,kubelet,rke2 -o rss=` — host-level process RSS.
  - `grep -E 'MemTotal|MemFree|MemAvailable|SReclaimable' /proc/meminfo` — host memory allocation.
  - `kubectl describe node | grep -A5 'Allocated resources'` — scheduler CPU/memory resource allocation.
  - `etcdctl endpoint status` — etcd db size.
- **Topology**:
  - Single-node: 1 control-plane node, no worker nodes.
  - Multi-node: 3-node cluster (1 control-plane, 2 workers).
- **Methodology**:
  - [Harvester Resource Footprint gist](https://gist.github.com/Vicente-Cheng/ae8d5ce743f94c0c9a333574813bc777).
  - Phase 1: Measure at idle (0 VMs running) to establish the platform baseline.
  - Phase 2: Create VMs until the scheduler rejects — record the maximum VM count and which resource (CPU, memory, or other) is the bottleneck.
  - Capture per-namespace breakdown (kube-system, harvester-system, longhorn-system, cattle-system) at each phase.
  - Compare single-node vs multi-node to understand how platform overhead scales with node count.
- **Key metrics**:
  - Platform idle cost: total CPU (millicores) and memory (MiB) with 0 VMs.
  - Maximum VM count before scheduler rejects, and the bottleneck resource.
  - Scheduler-reserved vs actual physical consumption ratio.
  - etcd db size (MiB).

#### Category 2: Performance Benchmark

##### 2a: Storage and Network I/O

- **Scope**: Storage throughput/latency and network throughput.
- **Tooling**:
  - Storage: `fio` (sequential and random read/write, 4K and 128K block sizes)
  - Network: `iperf3` (TCP throughput, UDP jitter)
- **Topology**:
  - Single-node: 1 control-plane node, no worker nodes.
  - Multi-node: 3-node cluster (1 control-plane, 2 workers).
- **Methodology**: [Harvester Performance wiki](https://github.com/harvester/harvester/wiki/Harvester-Performance-Result).
  - Raw Device measure - fio / iperf3
  - VMs on Longhorn volumes for storage tests.
    - Linux VM - fio
    - Windows VM - fio
- **Key metrics**: IOPS, throughput (MB/s), latency (ms), bandwidth (Gbps).

##### 2b: etcd

- **Scope**: etcd get, put, and watch latency and throughput at idle (0 VMs), to establish baseline performance for a given hardware configuration.
- **Tooling**:
  - etcd [benchmark](https://github.com/etcd-io/etcd/tree/main/tools/benchmark) tool — build from `etcd/tools/benchmark` in the etcd repository.
- **Topology**:
  - Single-node: 1 control-plane node, no worker nodes.
  - Multi-node: 3-node cluster (1 control-plane, 2 workers).
- **Methodology**:
  - Run at idle (0 VMs) to establish baseline.
  - For each operation, run a single-client and a concurrent workload:
    - **Put**:
      - Single: 1 connection, 1 client, 10,000 requests, 256-byte value.
      - Concurrent: 100 connections, 1,000 clients, 100,000 requests, 256-byte value.
    - **Get** (seed keys before benchmarking):
      - Single: 1 connection, 1 client, 10,000 requests.
      - Concurrent: 100 connections, 1,000 clients, 100,000 requests.
    - **Watch**:
      - Single: 1 connection, 1 client, 10,000 watch events.
      - Concurrent: 100 connections, 1,000 clients, 100,000 watch events.
- **Key metrics**:
  - Requests per second (throughput).
  - Latency: average, P50, P99.

#### Category 3: KubeVirt Scaling Comparison

- **Scope**: Measure the additional overhead Harvester introduces on top of vanilla KubeVirt, using the KubeVirt version bundled with the Harvester release under test.
- **Tooling**:
  - Prometheus (built-in to Harvester) — query KubeVirt metrics via PromQL.
  - KubeVirt [perfscale-audit](https://github.com/kubevirt/kubevirt/tree/main/tools/perfscale-audit) tool — connects to Prometheus, runs predefined queries, dumps results to JSON.
- **Topology**:
  - Single-node: 1 control-plane node, no worker nodes.
  - Multi-node: 3-node cluster (1 control-plane, 2 workers).
- **Methodology**:
  - Adopt measurement methodology from [KubeVirt perf-scale-benchmarks](https://github.com/kubevirt/kubevirt/blob/main/docs/perf-scale-benchmarks.md), not their cluster topology.
  - Use local-path storage for VM volumes to isolate KubeVirt overhead from storage backend variance.
  - Create 100 VMs with 100ms interval (minimal spec: Cirros, 100m CPU, 90Mi memory), wait for all to reach Running state.
  - Collect metrics via Prometheus after waiting for scrape interval (30s default).
  - Run the same test on vanilla KubeVirt (same version as bundled in Harvester) and Harvester on the same hardware.
  - Ensure workloads are evenly distributed among workers to avoid skewed results.
- **Phase 1 — Baseline** (establish the performance gap):
  - vmiCreationToRunningSeconds P50/P95 (boot time tail latency).
  - Total time for 100 VMs to reach Running (batch throughput).
- **Phase 2 — Profiling** (identify where the gap comes from):
  - **API operation counts**: PATCH-pods-count, PATCH-virtualmachineinstances-count, UPDATE-virtualmachineinstances-count per component — identifies the most expensive calls from the Harvester stack.

### Test plan

N/A

### Upgrade strategy

N/A

## Note [optional]

- Future work:
  - Benchmark automation (scheduled runs, result diffing, automatic regression issue filing).
  - etcd benchmark under VM load — run during VM creation to capture peak write pressure, and compare with steady-state after VMs are running.
  - Disk topology comparison — measure etcd performance when etcd shares the root disk (e.g., LVM) vs. a dedicated disk, to quantify I/O contention impact on etcd latency.
