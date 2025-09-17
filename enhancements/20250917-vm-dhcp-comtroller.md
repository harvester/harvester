# Refactoring for Scalability and Stability

**Authors:** davidepasquero
**Status:** Draft  
**Created At:** 2025-09-17
**Last Updated:** 2025-09-17

---

## Summary

This set of changes introduces a fundamental architectural evolution for the vm-dhcp-controller, transforming it into a more robust, scalable, and resilient solution. The main update shifts the agent management model from one pod per IPPool to a unified Deployment, while also introducing a leader election mechanism to ensure high availability. We propose replacing the "one agent pod per IPPool" model with a single DHCP agent Deployment capable of serving multiple networks in parallel. The controller dynamically assembles the configuration (Multus annotation and JSON environment variables) for every active IPPool, and the agent starts watchers and DHCP servers for each pool using a multi-tenant allocator.

---

## Motivation

The current per-pool management spawns many pods, duplicates the same image, and keeps state that is hard to sync (CLI arguments, Multus annotations, local leases). Aggregating the configuration on the controller side removes duplication, allows reusing a single process with leader election, and lets the DHCP allocator keep per-IPPool leases aligned with the evolving `IPPool` CRD and Harvester controllers.

---

## Goals / Non-goals

- *Goals*:
  - A single agent Deployment with interfaces configured through `AGENT_NETWORK_CONFIGS` and `IPPOOL_REFS_JSON`.
  - Lease synchronization sourced directly from `IPPool.Status.IPv4.Allocated`.
  - Controller responsible for Multus annotations and JSON environment updates.
  - DHCP allocator with per-IPPool lease management and coordinated cleanup.
- *Non-goals*:
  - IPv6 support or changes to the IPAM algorithm.
  - Revisiting the webhook or existing KubeVirt controllers beyond necessary adaptations.
  - Handling invalid NAD configurations beyond current checks (it only logs and skips incomplete pools).

---

## Proposal / Design

### Components updated

- **Helm chart**: introduces a dedicated agent Deployment with `NET_ADMIN` capability, empty JSON defaults, and removal of legacy arguments (`--nic`, `--ippool-ref`, etc.).
- **Deployment controller**: exposes `AGENT_DEPLOYMENT_NAME` and `AGENT_CONTAINER_NAME` via environment variables to drive runtime patches.
- **IPPool controller**: recomposes the Multus annotation, JSON envs, validates NADs, and updates the agent Deployment whenever the list of active pools changes.
- **Agent runtime**: deserializes interface/IPPool configurations, configures NICs, and starts watchers for each pool, waiting for `InitialSyncDone` before exposing the DHCP service.
- **DHCP allocator**: stores leases in `map[ipPoolRef]map[hwAddr]Lease`, provides `Run/DryRun` on configuration slices, and runs shared cleanup.

### Data/control flow

1. The IPPool controller adds the labels `network.harvesterhci.io/ippool-namespace` and `network.harvesterhci.io/ippool-name` to the NAD to mark the pool association, then updates IPAM state/metrics and prepares the sorted list of active IPPools.
2. The controller serializes `AgentNetConfig` and `IPPoolRefs` to JSON and updates the agent Deployment Multus annotation.
3. The agent pod receives the env vars via Downward API, configures interfaces (flush, ip addr add, up), and creates a handler for each observed IPPool.
4. Each handler waits for informer sync and seeds the DHCP allocator with current leases, signaling `InitialSyncDone`.
5. After syncing, the agent launches DHCP servers for every configuration with per-pool handlers that read from the multi-tenant map.

### Interactions

- **IPPool CRD**: remains the source of truth for CIDR, server/router, exclusions, and allocated leases.
- **NAD**: the controller applies namespace/name labels to track the pool association.
- **Management context**: stores namespace and shared factories, required to patch the agent Deployment and register controllers.
- **Leader election**: both agent and controller can disable it via `--no-leader-election`; otherwise they reuse existing ConfigMap/Lease objects.

### API / manifests / configuration

- `AgentOptions` extended with configuration and IPPool reference JSON payloads.
- Agent Deployment with `AGENT_NETWORK_CONFIGS`/`IPPOOL_REFS_JSON` env vars, dynamic Multus annotations, and consistent securityContext.
- Controller Deployment with extra env vars to locate patch targets and Downward API `POD_NAMESPACE`.

### Distribution / orchestration

- The Helm chart creates dedicated RBAC so the controller can patch the agent Deployment and the agent can watch IPPools/Leases.
- The agent runs as a shared ReplicaSet, so the controller can scale it via Helm or horizontally if needed.

---

## Security Considerations

- The agent needs `NET_ADMIN`; the chart ensures the capability is present even if users customize the securityContext.
- The controller receives namespace-scoped RBAC permissions to patch Deployments, limiting the blast radius of bugs.
- DHCP servers read configurations from IPPools and expose only UDP/67 on the specific interface.
- Falling back to empty JSON keeps the agent idle until the controller applies valid configurations, avoiding accidental exposure of unmanaged interfaces.

---

## Upgrade & Migration Plan

1. Upgrade the chart: it creates the new agent Deployment, RBAC, and extra env vars for the controller.
2. Once the controller runs with the new logic, it recomposes Multus annotations and JSON for each IPPool; existing agents read the values after restart.
3. The old per-pool model can be decommissioned by deleting legacy pods; the shared agent keeps serving existing leases thanks to the initial sync.
4. For rollback, restore the previous chart: the agent Deployment is removed and the controller stops patching envs/annotations; IPAM resources are released via `cleanup`.

---

## Alternatives Considered

- **Keep per-pool pods**: would require retaining `prepareAgentPod` and dynamic Pod generation; the logic remains commented as reference but was discarded due to operational complexity.
- **Replicate the agent via StatefulSet**: unnecessary because the allocator holds no on-disk state and leases are rebuilt from IPPools; a Deployment suffices.
- **Deliver configurations via ConfigMap**: the controller already patches envs/annotations directly on the Deployment template; introducing a ConfigMap would add rotation dependencies without immediate benefits.

---

## Drawbacks / Risks

- A single Deployment becomes a single point of failure: crashes or faulty rollouts interrupt service for every pool.
- JSON serialization errors on the controller side prevent the agent from starting; logs exist but there is no centralized pre-validation.
- The DHCP server uses `server4.NewServer` per NIC but does not aggregate errors; issues on individual interfaces might go unnoticed.
- Initial synchronization depends on informer health; failures may let the agent start with incomplete caches and missing leases.

---

## Testing Plan

- Controller reconciliation tests: validate Multus annotations, generated JSON, and pruning of legacy CLI arguments.
- Agent integration tests: simulate multiple IPPools with fake informers, ensuring `InitialSyncDone` before DHCP startup.
- DHCP allocator tests: load multiple configurations, confirm `map[ipPoolRef]map[hwAddr]` isolation, and correct cleanup when contexts are deleted.
- E2E: create real IPPools and NADs, verify the agent configures interfaces (`ip address flush/add/up`) and serves OFFER/ACK consistent with CRD leases.

---

## Dependencies

- Kubernetes with Multus and Harvester `network.harvesterhci.io/v1alpha1` CRDs installed.
- `insomniacslk/dhcp` library to manage multi-interface DHCPv4 packets.
- Harvester/KubeVirt clients generated via wrangler already initialized in the management context.

---

## References

- Pull Request : https://github.com/harvester/vm-dhcp-controller/pull/64
- Issue : https://github.com/harvester/harvester/issues/8794

---

## Metrics / success criteria

- Agent convergence time after IPPool changes (<60s).
- Maximum number of IPPools handled by a single agent pod without DHCP packet loss.
- Absence of orphaned leases after deleting or pausing an IPPool (confirmed by IPAM and allocator logs).

---
