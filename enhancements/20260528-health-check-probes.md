# Protect Workload Uptime With Probes, Watchdog and Pod Disruption Budget

## Summary

Harvester runs virtual machines through KubeVirt `virt-launcher` pods. Today, a `virt-launcher` pod can be reported as running and ready when the actual workload inside the guest operating system is not yet ready, has stopped serving traffic, or has become unhealthy. This mismatch can mislead load balancers, health-checking services, Harvester upgrade automation, and operators into treating an unhealthy VM as a healthy service endpoint.

This enhancement proposes exposing KubeVirt liveness and readiness probes, guest OS watchdog configuration, and Pod Disruption Budget (PDB) guidance/support in Harvester. Together, these mechanisms allow service owners to express workload health more accurately, recover from guest OS hangs, and reduce the risk of voluntary disruptions affecting too many VM-backed replicas at once.

The proposal focuses on making existing Kubernetes and KubeVirt primitives easier and safer to use from Harvester. Harvester should surface the relevant KubeVirt VM fields in the UI, and ensure upgrade/drain workflows respect readiness and disruption constraints.

### Related Issues

- <https://github.com/harvester/harvester/issues/9523>

## Motivation

Harvester users increasingly run production services and Rancher-managed guest clusters on top of Harvester VMs. For these workloads, a VM being powered on is not equivalent to the application being available. A web server may have an invalid TLS certificate, a guest cluster node may have a failed kubelet, or a database replica may still be starting after a reboot.

When Harvester and Kubernetes only see the `virt-launcher` pods as healthy, they can make unsafe decisions:

- Route traffic to a VM before the guest workload is ready.
- Keep an unhealthy VM in service because the pod is still running.
- Drain nodes during upgrades while dependent VM replicas are not actually available.
- Evict too many VM-backed replicas at the same time, causing application downtime or quorum loss.

Kubernetes already has well-understood concepts for these problems: liveness probes, readiness probes, watchdog devices, and Pod Disruption Budgets. KubeVirt exposes several of these capabilities to VM workloads. Harvester should make them accessible to VM owners in a way that fits Harvester workflows.

### Goals

- Allow Harvester users to configure VM readiness probes so traffic and operational decisions reflect guest workload readiness.
- Allow Harvester users to configure VM liveness probes so unhealthy guest workloads can be detected and remediated.
- Allow Harvester users to configure a guest OS watchdog device for detecting and reacting to guest kernel or OS hangs.
- Provide a clear user experience in the Harvester UI for configuring probe mechanisms and timing parameters.
- Document and, where appropriate, expose PDB usage for VM-backed replicated workloads.
- Ensure Harvester upgrade and node drain workflows benefit from more accurate readiness and disruption information.
- Preserve backward compatibility for existing VMs.

### Non-goals [optional]

- Implementing a new health-checking system independent of Kubernetes or KubeVirt.
- Implementing KubeVirt startup probes before KubeVirt supports them for VMs.
- Guaranteeing application-level correctness for arbitrary probe commands or endpoints.
- Automatically inferring correct probes for all guest workloads.
- Replacing application-native high availability, replication, quorum, or backup strategies.
- Changing Longhorn volume attachment and recovery semantics.

## Proposal

Harvester will expose and document three complementary protection layers:

| Layer | Primary failure addressed | Harvester behavior |
| --- | --- | --- |
| Readiness probe | Application is not ready to receive traffic | Surface KubeVirt readiness probe configuration on VMs and show readiness status in VM details |
| Liveness probe | Application or guest service is unhealthy and should be restarted/remediated | Surface KubeVirt liveness probe configuration on VMs and show failures/events |
| Guest OS watchdog | Guest kernel or operating system is no longer responsive | Surface KubeVirt watchdog device configuration on VMs |
| Pod Disruption Budget | Too many VM-backed replicas are disrupted by voluntary operations | Document and optionally expose PDB creation for VM groups using labels/selectors |

The first implementation should prioritize probe and watchdog configuration because these are VM-local settings already supported by KubeVirt. PDB support can be delivered as documentation plus UI/API improvements for users who manage groups of VMs.

### User Stories

#### Story 1: Web service readiness reflects actual TLS/application availability

Before this enhancement, a VM running Nginx can be marked ready as soon as the `virt-launcher` pod is ready. If Nginx has not started or its TLS certificate is invalid, traffic can still be routed to the VM.

After this enhancement, the service owner configures an HTTPS readiness probe against the Nginx endpoint. Harvester and Kubernetes only treat the VM as ready when the probe succeeds. During startup, certificate problems, or service reload failures, the VM remains not ready and can be excluded from traffic routing or upgrade decisions.

#### Story 2: Rancher guest cluster node health can be represented on Harvester VMs

Before this enhancement, a VM that hosts a downstream guest cluster node can appear healthy from Harvester even if the kubelet inside the guest is down. This can confuse remediation flows involving guest workloads, Harvester CSI, and Longhorn volumes.

After this enhancement, the guest cluster operator can configure a readiness or liveness probe that checks kubelet health or another node-local health endpoint. Harvester has a better signal for whether the VM-backed guest node is usable before disruptive actions are attempted.

#### Story 3: Database replicas are protected during Harvester upgrades

Before this enhancement, a three-node PostgreSQL cluster running on three VMs can lose quorum if an upgrade workflow drains nodes while one replica is already not serving traffic.

After this enhancement, each VM exposes workload readiness, and a PDB protects the replica set from voluntary disruption below the required availability threshold. Node drains wait until the disruption budget allows eviction, reducing the risk of quorum loss.

### User Experience In Detail

#### VM probe configuration

A new **Health Checks** section will be added to the VM create/edit flow in the Harvester UI.

Users can configure the following probe types:

| Probe type | Purpose | Typical action on failure |
| --- | --- | --- |
| Readiness probe | Determines whether the VM workload should receive traffic or be considered available | Mark VM workload as not ready |
| Liveness probe | Determines whether the VM workload is unhealthy and requires remediation | Restart/remediate according to KubeVirt behavior |

For each probe, the UI supports these mechanisms where supported by the installed KubeVirt version:

| Mechanism | Description | Example use case |
| --- | --- | --- |
| `exec` | Runs a command in the guest through KubeVirt guest execution support | Check a local process or file inside the guest |
| `grpc` | Executes a gRPC health check | Check a service implementing the gRPC health checking protocol |
| `guestAgentPing` | Checks whether the qemu-guest-agent is responsive | Check basic guest responsiveness without a specific command |
| `httpGet` | Sends an HTTP GET request to the pod/VM IP on a configured path and port | Check a web service endpoint |
| `tcpSocket` | Opens a TCP connection to a configured port | Check whether a listener is accepting connections |

The UI exposes common **optional** timing and threshold parameters:

| Field | Description |
| --- | --- |
| `initialDelaySeconds` | Delay before the first probe is executed after VM startup |
| `periodSeconds` | Interval between probe executions |
| `timeoutSeconds` | Timeout for each probe execution |
| `successThreshold` | Consecutive successes required after failure |
| `failureThreshold` | Consecutive failures required before marking the probe failed |

If these parameters are left blank, Kubernetes defaults apply.

Example HTTP readiness probe in a KubeVirt VM template:

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: nginx-vm
  namespace: web
spec:
  runStrategy: Always
  template:
    metadata:
      labels:
        app: nginx
        tier: frontend
    spec:
      readinessProbe:
        httpGet:
          path: /healthz
          port: 8443
          scheme: HTTPS
        initialDelaySeconds: 30
        periodSeconds: 10
        timeoutSeconds: 3
        successThreshold: 1
        failureThreshold: 3
      domain:
        devices: {}
```

Example TCP liveness probe:

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: postgres-replica-1
spec:
  runStrategy: Always
  template:
    spec:
      livenessProbe:
        tcpSocket:
          port: 5432
        initialDelaySeconds: 60
        periodSeconds: 20
        timeoutSeconds: 5
        failureThreshold: 3
      domain:
        devices: {}
```

Example `exec` readiness probe:

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: guest-node-1
spec:
  runStrategy: Always
  template:
    spec:
      readinessProbe:
        exec:
          command:
            - /bin/sh
            - -c
            - systemctl is-active --quiet kubelet
        initialDelaySeconds: 60
        periodSeconds: 15
        timeoutSeconds: 5
        failureThreshold: 2
      domain:
        devices: {}
```

For `exec` probes, the UI should display a warning that guest execution depends on the qemu-guest-agent package and that poorly designed commands can consume guest and node resources.

Example GRPC readiness probe:

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: etcd-1
spec:
  runStrategy: Always
  template:
    spec:
      livenessProbe:
        grpc:
          port: 2379
        initialDelaySeconds: 60
        periodSeconds: 20
        timeoutSeconds: 5
        failureThreshold: 3
      domain:
        devices: {}

```

Example `guestAgentPing` readiness probe:

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: vm-1
spec:
  runStrategy: Always
  template:
    spec:
      readinessProbe:
        guestAgentPing: {}
        initialDelaySeconds: 60
        periodSeconds: 20
        timeoutSeconds: 5
        failureThreshold: 3
      domain:
        devices: {}
```

#### Guest OS watchdog configuration

A new **Watchdog** subsection will be added to the VM create/edit flow. Users can enable a watchdog device and select the action that KubeVirt should take when the guest stops sending heartbeats.

Supported actions:

| Action | Behavior |
| --- | --- |
| `poweroff` | Power off the VM |
| `reset` | Reset the VM |
| `shutdown` | Attempt to shut down the VM |

Example VM watchdog configuration:

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: workload-with-watchdog
spec:
  runStrategy: Always
  template:
    spec:
      domain:
        devices:
          watchdog:
            name: guest-watchdog
            i6300esb:
              action: reset
```

Inside the guest OS, the user must install and run watchdog software. For example:

```sh
sudo busybox watchdog -t 2000ms -T 4000ms /dev/watchdog
```

Harvester should document that enabling the virtual watchdog device alone is not enough. The guest must actively write heartbeats to `/dev/watchdog`.

#### Pod Disruption Budget usage

Harvester should document how service owners can protect groups of VM-backed replicas with Kubernetes PDBs. A PDB is most useful when VMs are labeled consistently and represent replicas of the same application or guest cluster.

Example PDB protecting three PostgreSQL VMs labeled `app=postgres`:

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: postgres-vm-pdb
  namespace: database
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: postgres
```

Example PDB for VMs backing a Rancher guest cluster:

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: guest-rke2-pdb
  namespace: demo-green
spec:
  minAvailable: 2
  unhealthyPodEvictionPolicy: IfHealthyBudget
  selector:
    matchLabels:
      guestcluster.harvesterhci.io/name: guest-rke2
```

The initial implementation should include documentation and validation guidance. If a UI workflow is added, it should be placed under an advanced **Availability Policy** section and support:

- `minAvailable` as an absolute integer.
- `unhealthyPodEvictionPolicy` with `IfHealthyBudget` and `AlwaysAllow`.
- Label selector preview showing which VM `virt-launcher` pods would be selected.

Harvester should not silently create broad PDBs without explicit user intent because an overly strict PDB can block maintenance and upgrades.

#### Operational flow

```text
Guest workload health
        │
        ▼
KubeVirt probe or watchdog result
        │
        ▼
virt-launcher pod readiness/events
        │
        ▼
Kubernetes Service, Eviction API, PDB and Harvester upgrade/drain workflows
        │
        ▼
Traffic and voluntary disruption decisions reflect actual VM workload health
```

### API changes

This proposal should reuse existing KubeVirt and Kubernetes API fields wherever possible.

#### VirtualMachine probe fields

Harvester VM create/edit APIs should preserve and validate KubeVirt probe fields under the VM template spec. No new Harvester CRD is required for the core probe capability if the existing VM API can pass through these fields.

Relevant fields:

```yaml
spec:
  template:
    spec:
      readinessProbe: {}
      livenessProbe: {}
```

Each probe may contain one probe handler:

- `exec`
- `grpc`
- `httpGet`
- `tcpSocket`

and optional timing/threshold fields:

- `initialDelaySeconds`
- `periodSeconds`
- `timeoutSeconds`
- `successThreshold`
- `failureThreshold`
- `terminationGracePeriodSeconds`

#### VirtualMachine watchdog fields

Harvester should preserve and validate KubeVirt watchdog fields:

```yaml
spec:
  template:
    spec:
      domain:
        devices:
          watchdog:
            name: guest-watchdog
            i6300esb:
              action: reset
```

#### PodDisruptionBudget fields

If Harvester adds UI support for PDB creation, it should create standard Kubernetes `policy/v1` `PodDisruptionBudget` resources. No Harvester-specific PDB API should be introduced unless a later design identifies a concrete need.

Supported initial PDB fields:

```yaml
spec:
  minAvailable: 2
  unhealthyPodEvictionPolicy: IfHealthyBudget
  selector:
    matchLabels:
      app: postgres
```

## Design

### Implementation Overview

#### UI changes

Add a **Health Checks** section to the VM create and edit pages.

Recommended layout:

1. **Readiness Probe**
   - Enable/disable toggle.
   - Probe mechanism selector.
   - Mechanism-specific fields.
   - Timing and threshold fields.
2. **Liveness Probe**
   - Enable/disable toggle.
   - Probe mechanism selector.
   - Mechanism-specific fields.
   - Timing and threshold fields.
3. **Watchdog**
   - Enable/disable toggle.
   - Device name.
   - Action selector.
   - Guest setup documentation link.

Add an **Availability Policy** section to the Namespace config page for PDB guidance or creation.

Recommended layout:

1. **Pod Disruption Budget**
   - Label selector input.
   - `minAvailable`.
   - `unhealthyPodEvictionPolicy`.

#### Backend/controller changes

The expected backend work is small if Harvester already preserves arbitrary KubeVirt VM fields.

If existing Harvester APIs strip unknown nested KubeVirt fields, the API layer must be updated to include these fields explicitly in the VM schema.

#### Status and event visibility

Probe configuration is only useful when users can understand its effect. Harvester should display related status and events, including:

- Last probe failure messages from pods conditions.
- Watchdog-triggered shutdown/reset/poweroff events.

### Test plan

#### Unit tests

- Validate probe form serialization and deserialization for `exec`, `grpc`, `httpGet`, and `tcpSocket`.
- Validate form constraints for missing ports, empty commands, invalid thresholds, and multiple handlers.
- Validate watchdog form serialization and allowed action values.
- Validate PDB form behavior if UI-based PDB creation is implemented.

#### Integration tests

1. **HTTP readiness probe**
   - Create a VM with an HTTP readiness probe against a test service.
   - Verify the VM is not ready before the service starts.
   - Start the service and verify the VM becomes ready.

2. **TCP liveness probe**
   - Create a VM with a TCP liveness probe.
   - Stop the listening process in the guest.
   - Verify probe failures are surfaced through events and KubeVirt performs the expected remediation.

3. **Exec probe with qemu-guest-agent**
   - Create a VM with qemu-guest-agent installed.
   - Configure an exec probe that checks a known file or service.
   - Verify readiness changes when the command result changes.

4. **Watchdog reset/poweroff**
   - Create a VM with a watchdog device.
   - Start guest watchdog heartbeats.
   - Stop heartbeats.
   - Verify KubeVirt applies the configured action.

5. **PDB-respected drain**
   - Create three labeled VMs with readiness probes.
   - Create a PDB with `minAvailable: 2`.
   - Mark one VM not ready.
   - Attempt node drain for another VM.
   - Verify eviction is blocked until the budget allows disruption.

6. **Upgrade workflow**
   - Run a Harvester upgrade or simulated upgrade drain with VM readiness probes and a PDB.
   - Verify the workflow waits instead of disrupting below the defined availability threshold.

#### Manual tests

- Confirm UI edit flows preserve existing YAML-defined probes.
- Confirm disabled probes are removed or left unchanged according to user intent.
- Confirm probe events are visible and understandable to operators.
- Confirm documentation links are available from UI help text.

### Upgrade strategy

This enhancement should be backward compatible.

- Existing VMs without probes, watchdogs, or PDBs continue to behave as they do today.
- No automatic probes are added during upgrade.
- No automatic watchdog devices are added during upgrade.
- No automatic PDBs are created during upgrade.
- Existing KubeVirt VM manifests that already include probes or watchdogs must be preserved by Harvester after upgrade.

For users upgrading into this feature, release notes should recommend:

1. Add readiness probes before relying on PDBs for application availability.
2. Start with conservative probe periods and thresholds.
3. Test liveness probes in staging before enabling them in production.
4. Use labels consistently across VM replicas before creating PDBs.
5. Verify guest OS watchdog support before enabling watchdog actions.

## Note [optional]

### Limitations and risks

#### Probe limitations

- KubeVirt does not currently provide VM startup probes. Harvester should not present startup probes until KubeVirt supports them.
- Bad readiness probes can keep a healthy VM out of service.
- Bad liveness probes can cause repeated restarts and cascading failures.
- Frequent `exec` probes can create excessive process and CPU overhead, especially in dense clusters.
- `exec` probes require suitable guest support, including qemu-guest-agent.
- HTTP and TCP probe behavior in dual-stack clusters may be limited by KubeVirt/Kubernetes handling of pod IP selection. Users should validate IPv6 behavior before relying on it.

#### Watchdog limitations

- The guest OS must support the watchdog device.
- Guest-side watchdog software must be installed and running.
- Aggressive watchdog timeouts can reset or power off VMs during temporary load spikes.

#### PDB limitations

- PDBs apply to voluntary disruptions only. They do not prevent hardware failures, kernel crashes, node power loss, or involuntary pod termination.
- For bare pods such as `virt-launcher` pods, PDBs have limitations compared with controller-managed workloads.
- `minAvailable` should be used as an absolute integer for VM-backed workloads.
- Overly strict PDBs can block maintenance and upgrades.

### Recommended defaults

Harvester should avoid enforcing global defaults because correct health checks are application-specific. However, UI examples can recommend safe starting values:

| Field | Suggested starting value |
| --- | --- |
| `initialDelaySeconds` | `30` to `120`, depending on guest boot time |
| `periodSeconds` | `10` to `30` |
| `timeoutSeconds` | `3` to `5` |
| `successThreshold` | `1` |
| `failureThreshold` | `3` |

### References

- Harvester documentation: <https://docs.harvesterhci.io/v1.5/>
- KubeVirt user guide: <https://kubevirt.io/user-guide/>
- KubeVirt liveness, readiness probes and watchdog: <https://kubevirt.io/user-guide/user_workloads/liveness_and_readiness_probes/>
- Kubernetes probes: <https://kubernetes.io/docs/concepts/workloads/pods/probes/>
- Kubernetes disruptions and PDBs: <https://kubernetes.io/docs/concepts/workloads/pods/disruptions/>
- Longhorn documentation: <https://longhorn.io/docs/1.9.0/>
