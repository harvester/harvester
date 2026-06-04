# Harvester Benchmark Policy

## Summary

This HEP defines the benchmarking policy for Harvester — what categories we test, how we run them, and what follow-up actions we take on the results. The goal is to establish a repeatable, release-gated process so performance regressions are caught before shipping and improvements can be tracked across releases.

Currently, benchmark runs are ad-hoc: different engineers use different tools, results are not captured consistently, and there is no agreed baseline to compare against. This makes it difficult to reason about performance trends or give users reliable sizing guidance.

### Related Issues

- <https://github.com/harvester/harvester/issues/9777>
- <https://github.com/harvester/harvester/issues/10421>

## Motivation

### Goals

- **Define Benchmark Categories**: Establish canonical categories covering Harvester's performance surface — Resource Footprint (including VM creation latency) and I/O Performance.
- **Standardize Tooling and Methodology**: Specify tools and procedures for each category so results are reproducible by any team member and users can run benchmarks in their own environments.
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

The benchmark policy is organized into two categories. Each defines scope, tooling, topology, methodology, and key metrics.

#### Prerequisites

##### Infrastructure Configuration Consistency

**Standardize configurations** to ensure results are comparable across releases:
- **Hardware**: Document CPU model, RAM, disk type (SSD/NVMe/HDD), network interfaces. Use same hardware across releases or annotate changes.
- **Network**: Kube-OVN is enabled.
- **Storage**: Specify Longhorn replica count, LVM config, disk partitioning, filesystem type.
- **Harvester config**: Document non-default settings (kernel params, resource reservations, addons).

##### Monitoring Stack Setup

**All benchmark categories require the Harvester monitoring stack to be enabled** to collect detailed metrics during test execution.

- **Installation**: Enable the `rancher-monitoring` chart in Harvester settings before running any benchmark.
- **Purpose**: 
  - Collect component-level metrics (CPU, memory, API latency) via Prometheus during benchmark runs, rather than just a peak value
  - Capture P50/P99/tail latency metrics for VM creation.

#### Category 1: Resource Footprint

- **Scope**: CPU and memory consumption of each Harvester component at idle and under maximum VM density, including VM creation latency metrics.
- **Tooling**:
  - `kubectl top pods` — per-pod CPU/memory usage.
  - `ps -C containerd,kubelet,rke2 -o rss=` — host-level process RSS.
  - `grep -E 'MemTotal|MemFree|MemAvailable|SReclaimable' /proc/meminfo` — host memory allocation.
  - `kubectl describe node | grep -A5 'Allocated resources'` — scheduler CPU/memory resource allocation.
  - `etcdctl endpoint status` — etcd db size.
  - Prometheus (enabled via monitoring stack) — query `kubevirt_vmi_phase_transition_time_from_creation_seconds` and component metrics.
  - KubeVirt [perfscale-audit](https://github.com/kubevirt/kubevirt/tree/main/tools/perfscale-audit) tool — connects to Prometheus for VM creation latency queries.
- **Topology**:
  - Single-node: 1 control-plane node, no worker nodes.
  - Multi-node: 3-node cluster (1 control-plane, 2 workers).
- **Methodology**:
  - [Harvester Resource Footprint gist](https://gist.github.com/Vicente-Cheng/ae8d5ce743f94c0c9a333574813bc777).
  - Adopt VM creation latency measurement from [KubeVirt perf-scale-benchmarks](https://github.com/kubevirt/kubevirt/blob/main/docs/perf-scale-benchmarks.md) (PromQL queries).
  - **Phase 1: Idle baseline**
    - Ensure monitoring stack is running and all pods are Ready.
    - Measure resource consumption with 0 VMs to establish platform baseline.
    - Capture per-namespace breakdown: `kube-system`, `harvester-system`, `longhorn-system`, `cattle-system`, `cattle-monitoring-system`.
  - **Phase 2: VM creation and scaling**
    - Create VMs incrementally with 100ms interval (minimal spec: Cirros or Alpine, 100m CPU, 128Mi memory) until the scheduler rejects new VMs.
    - **During VM creation**, capture VM creation latency via Prometheus:
      - `kubevirt_vmi_phase_transition_time_from_creation_seconds` (histogram) — P50, P95, P99.
      - Total time from first VM creation to last VM reaching Running (batch throughput).
    - Record the maximum VM count and identify the bottleneck resource (CPU, memory).
    - Capture per-component resource consumption at max VM density (same metrics as Phase 1).
    - Ensure workloads are evenly distributed among workers to avoid skewed results.
  - Compare single-node vs multi-node to understand how platform overhead scales with node count.
  - **User-reproducible script**: Provide a standalone script with VM manifest templates and PromQL queries so users can run in their own environment to validate expected resource footprint and VM creation performance.
- **Key metrics**:
  - **Resource consumption**:
    - Platform idle cost: total CPU (millicores) and memory (MiB) with 0 VMs, broken down by namespace.
    - Monitoring stack overhead: CPU/memory of `cattle-monitoring-system`, Prometheus TSDB size (MiB).
    - Maximum VM count before scheduler rejects, and the bottleneck resource.
    - Scheduler-reserved vs actual physical consumption ratio.
    - etcd db size (MiB).
  - **VM creation latency** (captured during Phase 2):
    - P50, P95, P99 (seconds from VMI creation to Running state).
    - Tail latency: max creation time in the batch.
    - Batch throughput: total time for all VMs to reach Running.
#### Category 2: Performance Benchmark

##### 2a: Storage and Network I/O

- **Scope**: Storage throughput/latency and network throughput, with Prometheus monitoring of system-level resource impact during I/O tests.
- **Tooling**:
  - **Storage**: 
    - `fio` (sequential and random read/write, 4K and 128K block sizes)
    - **Version pinning**: Use the same fio version across all Harvester releases for result consistency (e.g., fio-3.36 from official container image or built into test VM image).
  - **Network**: 
    - `iperf3` (TCP throughput, UDP jitter)
    - **Packaged image**: Provide a pre-built container image with iperf3 for reproducibility (e.g., `harbor.example.com/harvester/iperf3:3.15`).
  - **Monitoring**:
    - Prometheus (enabled via monitoring stack) — track node-level disk I/O, network bandwidth, and CPU iowait during tests.
- **Topology**:
  - **Multi-node (3-node cluster)**: For cluster-level storage/network tests.
    - All three nodes: 1 control-plane, 2 workers.
    - **Storage network**: 10GbE dedicated NIC for Longhorn replication traffic, isolated from management and VM networks.
    - **Kube-OVN enabled**: Test with Kube-OVN networking to measure its impact on VM network performance (compare against default Canal/Multus).
    - **Longhorn replica count > 1**: Set Longhorn volume replicas to 2 or 3 to measure replication overhead on storage throughput.
  - **Inter-node bandwidth**: Measured via iperf3 before benchmark runs to confirm line rate (record actual observed bandwidth).
- **Methodology**: [Harvester Performance wiki](https://github.com/harvester/harvester/wiki/Harvester-Performance-Result).
  - **Storage overhead (layered testing)**:
    - **Layer 0 (Raw Device)**: fio directly on host block device (multi-node host controller node).
    - **Layer 1 (Longhorn volume, host)**: fio on Longhorn PVC mounted to a pod on the host (multi-node, with replica count ≥ 2).
    - **Layer 2 (guest VM)**: fio inside Linux/Windows VM backed by Longhorn volume (multi-node, with Kube-OVN enabled/disabled comparison).
  - **Network overhead (layered testing)**:
    - **Layer 0 (Raw Device)**: Confirm inter-node line rate via iperf3 between bare metal nodes or host pods.
    - **Layer 1 (VM network)**: iperf3 between two VMs on different nodes to measure guest network throughput.
  - **Prometheus monitoring during tests**:
    - Capture time-series data during the entire fio/iperf3 run.
    - Track node-level metrics to identify bottlenecks (see Prometheus metrics section).
  - **User-reproducible script**: Provide a standalone script with:
    - Pre-built fio/iperf3 container images.
    - VM manifest templates with fio/iperf3 pre-installed.
    - PromQL queries to validate results.
- **Key metrics**:
  - **Storage**:
    - IOPS (4K random read/write).
    - Throughput (MB/s, 128K sequential read/write).
    - Latency (ms, P50/P95/P99).
  - **Network**:
    - Bandwidth (Gbps, TCP throughput).
    - Latency (ms, UDP jitter).

##### 2b: etcd

- **Scope**: etcd get, put, and watch latency and throughput at idle (0 VMs), with Prometheus monitoring to identify disk-level bottlenecks.
- **Tooling**:
  - **etcd benchmark**: etcd [benchmark](https://github.com/etcd-io/etcd/tree/main/tools/benchmark) tool — build from `etcd/tools/benchmark` in the etcd repository.
  - **Prometheus metrics**: Monitor etcd disk performance via `etcd_disk_wal_fsync_duration_seconds` and `etcd_disk_backend_commit_duration_seconds`, [reference](https://etcd.io/docs/v3.6/metrics/).
- **Topology**:
  - Single-node: 1 control-plane node, no worker nodes.
  - Multi-node: 3-node cluster (1 control-plane, 2 workers).
- **Methodology**:
  - Run at idle (0 VMs) to establish baseline.
  - **etcd API benchmark**:
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
  - **etcd API performance (from etcd benchmark)**:
    - Requests per second (throughput).
    - Latency: average, P50, P99.
  - **Prometheus monitoring during tests**:
    - Capture etcd disk performance metrics during the benchmark:
      - `etcd_disk_wal_fsync_duration_seconds` — WAL (Write-Ahead Log) fsync latency (how long it takes to persist each write to disk).
      - `etcd_disk_backend_commit_duration_seconds` — boltdb snapshot commit latency (how long it takes to commit batched changes to disk).

### Test plan

N/A

### Upgrade strategy

N/A

## Note [optional]

### User-Reproducible Benchmark Scripts

All benchmark categories will include standalone scripts that users can run in their own Harvester environments to:
- Validate that their hardware meets expected performance baselines.
- Debug performance issues by comparing local results against published benchmarks.
- Reproduce benchmark results for support cases.

### Future Work

- Benchmark automation (scheduled runs, result diffing, automatic regression issue filing).
- etcd benchmark under VM load — run during VM creation to capture peak write pressure, and compare with steady-state after VMs are running.
- Disk topology comparison — measure etcd performance when etcd shares the root disk (e.g., LVM) vs. a dedicated disk, to quantify I/O contention impact on etcd latency.
