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

The benchmark policy is organized into four categories. Each defines scope, tooling, topology, methodology, and key metrics.

#### Category 1: Resource Footprint

- **Scope**: CPU and memory consumption of each Harvester component (pods) at idle and under VMs load.
- **Tooling**:
  - `kubectl top pods`, know the pod cpu, memory usages
  - `grep -E 'MemTotal|MemFree|MemAvailable|SReclaimable' /proc/meminfo`, host memory allocation.
  - `kubectl describe node | grep -A5 'Allocated resources'`, know the scheduler cpu/memory resource allocation.
- **Topology**:
  - Single-node: 1 control-plane node, no worker nodes.
  - Multi-node: 3-node cluster (1 control-plane, 2 workers).
- **Methodology**:
  - [Harvester Resource Footprint gist](https://gist.github.com/Vicente-Cheng/ae8d5ce743f94c0c9a333574813bc777).
  - Capture component-level CPU/memory at rest, then with increasing VM counts (e.g., 10 VMs, 20 VMs) to understand how resource consumption scales.
- **Key metrics**:
  - CPU (millicores), memory RSS (MiB), per pod, per VM count.
  - VMs can be created in a given environment before a component becomes the bottleneck.

#### Category 2: Performance Benchmark (I/O)

- **Scope**: Storage throughput/latency and network throughput.
- **Tooling**:
  - Storage: `fio` (sequential and random read/write, 4K and 128K block sizes)
  - Network: `iperf3` (TCP throughput, UDP jitter)
- **Topology**: Multi-node cluster.
- **Methodology**: [Harvester Performance wiki](https://github.com/harvester/harvester/wiki/Harvester-Performance-Result).
  - Raw Device measure - fio / iperf3
  - VMs on Longhorn volumes for storage tests.
    - Linux VM - fio
    - Windows VM - fio
- **Key metrics**: IOPS, throughput (MB/s), latency (ms), bandwidth (Gbps).

#### Category 3: KubeVirt Scaling Comparison

- **Scope**: Measure the additional overhead Harvester introduce on top of vanilla KubeVirt.
- **Tooling**:
  - `kubectl top pods` — per-component CPU/memory during and after VM creation.
- **Topology**:
  - Single-node: 1 control-plane node, no worker nodes.
  - Multi-node: 3-node cluster, from [kubevirt-presubmits-1.8.yaml](https://github.com/kubevirt/project-infra/blob/main/github/ci/prow-deploy/files/jobs/kubevirt/kubevirt/kubevirt-presubmits-1.8.yaml).
- **Methodology**:
  - Follow [KubeVirt perf-scale-benchmarks](https://github.com/kubevirt/kubevirt/blob/main/docs/perf-scale-benchmarks.md).
  - Create 100 VMs with 100ms interval, wait for all to reach Running state.
  - Compare results between vanilla KubeVirt and Harvester on the same hardware, similar to Category 1: Resource Footprint
- **Key metrics**:
  - vmiCreationToRunningSeconds P50/P95
  - CPU/memory of virt-controller, harvester-controller during batch creation

### Test plan

N/A

### Upgrade strategy

N/A

## Note [optional]

- Future work: benchmark automation (scheduled runs, result diffing, automatic regression issue filing).  
