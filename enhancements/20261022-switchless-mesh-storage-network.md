# Switchless full-mesh storage network

## Summary

Small edge clusters, typically three nodes in a store, restaurant or branch office, need a
dedicated storage network so that Longhorn replication does not compete with VM and management
traffic. Today that requires a storage switch, and a 10/25 GbE switch per site is often the most
expensive and least reliable item in the rack. A **switchless full mesh** avoids it: each node uses
two spare NICs, cabled directly to the other two nodes.

Harvester can already put the storage network on a mesh, but only with a `broadcast` bond, the one
bond mode that is correct when each link reaches a different neighbor. Broadcast sends every frame
out every cable, which roughly halves the replica write bandwidth.

This HEP adds an **isolated uplink mode** to the cluster network. Instead of a bond, each NIC is
attached directly to the cluster network bridge as an *isolated* bridge port
(`IFLA_BRPORT_ISOLATED`). The kernel never forwards between isolated ports, so the triangle has no
loop and no node relays traffic for the others. Pods (Longhorn instance managers, virt-handler) sit
on ordinary bridge ports and reach both neighbors. Unicast leaves only on the cable to the right
peer. An isolated cluster network is reserved for the storage, RWX and VM live migration networks.

On a 3-node test cluster (1 GbE mesh), the 3-replica VM write throughput went from 38.2 MB/s
(broadcast bond) to 69.8 MB/s (isolated mode, MTU 9000), and each cable carried exactly one copy of
the data.

### Related Issues

- Epic: https://github.com/harvester/harvester/issues/12019
- Backend: https://github.com/harvester/harvester/issues/11993, PR harvester/network-controller-harvester#445 (draft)
- UI: https://github.com/harvester/harvester/issues/12018, PR harvester/harvester-ui-extension#1203 (draft)

## Motivation

Edge customers such as supermarkets and restaurants run Harvester on three small nodes per site,
across hundreds of sites. The per-site budget does not stretch to a 10 GbE switch for storage in
every store, and a switch adds a single point of failure, a device to power, mount and patch, and
another part to replace remotely. Direct cables (DAC or Cat6) are cheap, and most servers already
have two unused ports. What is missing is a supported way to use them efficiently.

Measured on the same hardware (3 × Ampere Altra, Intel I350 1 GbE, Longhorn v1, 3 replicas, 1 GiB
`dd oflag=direct` inside a VM):

| Setup | Guest write | Data per cable (engine node) |
|---|---|---|
| Broadcast bond (works today), MTU 1500 | 38.2 MB/s | 2150 MiB on each cable: both replica streams on both cables |
| Isolated ports, MTU 1500 | 60.0 MB/s | 1072 MiB on each cable |
| Isolated ports, MTU 9000 | 69.8 MB/s | 1034 MiB on each cable, 0 on the third cable |

iperf3 between storage network pods reached 990 Mbit/s on every cable, and 1.98 Gbit/s from one node
to both neighbors at once. That is the reason a mesh is worth having: each node gets the sum of its
links, with no switch to oversubscribe.

### Goals

- Run the Harvester storage network on a switchless full mesh with one copy of each frame per
  cable, and no loops.
- Also allow the RWX network and the VM live migration network on the mesh. Both are point-to-point
  between nodes, as storage replication is.
- Report the state of each mesh link per node.
- Prevent configurations that do not work on a mesh (VM networks, HostNetworkConfigs, mixed modes).
- Keep today's behavior unchanged for every existing cluster network.

### Non-goals

- **Path redundancy per node pair** with one cable per pair (see [Failure behavior](#failure-behavior-and-constraints)).
  Redundancy comes from Longhorn replicas. Per-pair fault tolerance needs four NICs per host
  (two links per peer), which is a follow-up.
- **Relaying through a third node** when a cable fails (a routed mesh). See [Alternatives](#alternatives-considered).
- VM networks or host networks (`HostNetworkConfig`) on the mesh. A VM network needs one L2 segment
  that reaches every node and the outside world, which a mesh is not. A HostNetworkConfig is blocked
  for a similar reason: its use cases (reaching external storage, a backup target or a gateway, DHCP
  from an upstream server) need a segment that leaves the cluster, and on a mesh each node only
  reaches its direct neighbors, with no DHCP server and no gateway. Node-to-node host traffic would
  work in static mode, so this can be relaxed later if a use case appears.
- The management network on the mesh.
- Automatic discovery of which NIC is cabled to which node (possible follow-up, for example with LLDP).

## Proposal

### User Stories

#### Story 1: storage network for a 3-node store without a switch

A retailer runs a 3-node Harvester cluster in each store. The management network uses the store's
existing 1 GbE switch. They want Longhorn replication on its own network.

**Before:** they either buy a storage switch per store, share the management network with storage,
or cable a mesh with a broadcast bond and lose about half of the bandwidth.

**After:** they cable each node's two spare ports to the other two nodes, create a cluster network
with the uplink mode *Isolated ports (switchless mesh)*, and select it in the `storage-network`
setting. Replication runs at line rate on each cable.

#### Story 2: faster live migration during maintenance

The same cluster runs node maintenance and upgrades, which live-migrate every VM off a node. With
`vm-migration-network` on the mesh, migration traffic runs on the direct cable between source and
target node and does not load the management network. On the test cluster, a VM with about 2 GB of
RAM in use migrated in about 23 s, with 2433 MiB on the direct cable and about 1 MiB on the
management network.

#### Story 3: an operator sees a bad cable

A cable is unplugged during a store refit. The cluster network link monitor shows the NIC as `down`
on that node, and Longhorn shows the affected volumes as `degraded` instead of failed. After the
cable is reconnected, the port returns as an isolated port and the replicas rebuild (delta rebuild,
not a full copy).

#### Story 4: a cable breaks permanently

A cable or transceiver fails and stays dead until someone replaces it (suggested by
@w13915984028). The management network is fine, so the cluster stays up, but the direct path
between two nodes, say A and B, is gone until the cable is replaced.

- **Storage:** volumes whose engine runs on A or B lose the replica on the other end and stay
  `degraded`. Longhorn cannot rebuild that replica while the cable is down: with 3 replicas on 3
  nodes, the only node it can place it on is the one it cannot reach.
- **Migration:** with `vm-migration-network` on the mesh, a live migration between A and B fails,
  because nothing relays through C. Migrations to and from C still work.
- **No self-healing:** this is the limitation of a static mesh with isolated ports and no routing
  (see [Non-goals](#non-goals)). The LinkMonitor shows the NIC as `down` on both nodes until the
  cable is replaced.
- **Mitigation until the repair:** C still reaches both A and B. Live-migrating the affected VMs to
  C moves their volume engines there, the missing replicas rebuild, and the volumes return to
  `healthy`. The cluster then runs with reduced placement freedom (A and B cannot exchange storage
  traffic) until the cable is replaced. The docs will describe this procedure and recommend an
  alert on mesh link state.
- **Fault tolerance needs four NICs per host:** surviving a cable failure without degraded
  volumes takes two links per peer, so four NICs per host on a 3-node mesh, grouped as one bond
  per peer (see [Multiple cables per node pair](#multiple-cables-per-node-pair-four-nics-per-node)).

### User Experience In Detail

1. Cable the mesh: on each node, one NIC to each of the other nodes. NIC names should be the same
   on all nodes. Otherwise use one Network Configuration per node group, as today.
2. **Networks > Cluster Networks > Create**, for example `storage`.
3. **Create Network Configuration** for it: select the two mesh NICs and set **Uplink Mode** to
   **Isolated ports (switchless mesh)**. Bond options are hidden. Optionally set MTU 9000.
4. The cluster network is shown as *switchless mesh* and the network configuration type as
   *VLAN, isolated ports*. Each mesh NIC appears in the link status per node.
5. **Settings > storage-network** (and optionally `rwx-network` and `vm-migration-network`): select
   `storage (switchless mesh)` and a range, as today.
6. The cluster network is not offered in the VM network and host network forms. The webhook rejects
   such objects if they are created through the API.

Changing the uplink mode later follows the existing storage network procedure: stop the VMs, clear
`storage-network`, edit the network configuration, then set `storage-network` again.

### API changes

One new optional field on `VlanConfig`:

```yaml
apiVersion: network.harvesterhci.io/v1beta1
kind: VlanConfig
metadata:
  name: storage-mesh
spec:
  clusterNetwork: storage
  uplink:
    mode: isolated          # new: "" (default) or "bond" = today's behavior; "isolated" = no bond
    nics: [ens2f0, ens2f1]
    linkAttributes:
      mtu: 9000
```

- `bondOptions` is ignored, and the UI does not save it, when `mode: isolated`.
- The manager labels the cluster network `network.harvesterhci.io/uplink-mode=isolated` while an
  isolated VlanConfig exists, so that the UI and other components can identify it. The label is
  informational: the webhooks enforce the rules from the VlanConfigs, so editing the label does
  not bypass them.

No changes to the ClusterNetwork, NAD or setting schemas. The storage network NAD already targets
`<cn>-br`, so Harvester core needs no change.

## Design

### Implementation Overview

All changes are in harvester-network-controller (PR #445) and the UI extension (PR #1203).

**Agent (per node).** For `mode: isolated`, instead of creating `<cn>-bo`:

- each NIC is set down, attached to `<cn>-br`, marked isolated (`netlink.LinkSetIsolated`), gets its
  MTU, and is brought up. It is isolated before it comes up, so it never forwards, even during boot.
- VLAN IDs are applied to every uplink instead of the single bond (`vlan.Vlan` now holds a list of
  uplinks), so tagged storage VLANs work.
- switching modes deletes a leftover bond, or releases directly attached NICs before building a bond.

**Manager.** It sets and clears the `uplink-mode` label, and adds the mesh NICs to the cluster
network's LinkMonitor name rule, so each link reports up or down per node.

**Mesh completeness check.** In bond mode the VlanConfig does not care how many NICs it has. In
isolated mode the NIC count matters: a full mesh of N nodes needs N−1 links with carrier on every
node. The agent reports a `MeshComplete` condition on each VlanStatus: `False` when fewer than N−1
isolated uplinks have carrier, where N is the number of non-witness nodes covered by the cluster
network. The UI shows it as a warning on the cluster network. This is a status check, not an
admission rule, because N changes over time: a node being added, replaced or re-cabled is a valid
temporary partial mesh, and rejecting the VlanConfig would make those procedures harder. Which peer
each NIC is cabled to (two NICs to the same node, or one NIC to a switch) is not verified, so
cabling stays the user's responsibility in this HEP. Verifying peers with LLDP is a follow-up (see
[Non-goals](#non-goals)).

**Webhooks.**

- isolated mode needs at least one NIC, no duplicate NICs, and is rejected on `mgmt`.
- **One mode per cluster network.** All VlanConfigs of a cluster network must use the same mode.
  Mixed mode (some nodes bonded, some isolated) is not allowed. A bonded node on a mesh needs
  `broadcast` to be correct, and each isolated node would then receive both copies. It also makes
  the failure behavior depend on the node, which is hard to reason about and to support.
- an isolated cluster network only carries the storage, RWX and VM migration networks. Other NADs
  and HostNetworkConfigs are denied. A cluster network cannot become isolated while such objects
  use it.

#### How traffic finds the right cable (no routes)

The mesh is one L2 segment with one subnet, the same as today's storage network (bridge CNI +
Whereabouts). No per-peer `/32` routes and no route controller are needed:

- A pod's `net1` sits on `<cn>-br`. To reach a peer pod in the same subnet, it sends ARP. The bridge
  floods the broadcast to both isolated uplinks; isolation only blocks uplink-to-uplink forwarding,
  not pod-to-uplink.
- The peer replies on its direct cable, and the bridge learns the peer MAC on that port (FDB).
  Unicast to that MAC then goes out only that cable.
- When Whereabouts gives a pod a new IP, or a pod restarts with a new MAC, ARP and MAC learning
  update the path automatically. A link-down event flushes the FDB entries of that port.

Verified on the test cluster with per-port counters and FDB dumps:

- every remote MAC was learned only on the cable directly connected to its host, never on the
  third cable.
- traffic between two nodes moved 0 bytes on the cable not involved.
- 400 ARP broadcasts for a non-existent IP were never relayed to the third cable.

#### Failure behavior and constraints

With one cable per node pair (two NICs per node in a 3-node cluster):

- **A single cable or NIC failure cuts the direct path of exactly one node pair.** Nothing reroutes
  through the third node, since isolation prevents relaying by design. The other two pairs are not
  affected.
- **Longhorn provides the redundancy.** A 3-replica volume whose engine is on one end of the dead
  cable loses one replica, stays attached and serves I/O on the other two, and rebuilds once the
  cable is back. In tests, VM writes completed with 0 guest I/O errors. The volume was `healthy`
  again 62–129 s after link-up (20 GiB volume, 1 GbE, delta rebuild).
- **A second failure** can cut one node off the storage network completely. Its replicas fail, and
  volumes keep running on the remaining replicas, as with a node failure today.
- **One-sided failures** (one end of a cable down, the far end keeps carrier) were observed with
  `ip link set down`. In isolated mode this behaves like a dead cable for that pair. Longhorn
  detects it at the replica level.
- **Two replicas or fewer:** volumes with fewer than 3 replicas can lose their only remote replica
  on a single cable failure. The docs will recommend 3 replicas on a mesh.
- **Scale:** a full mesh of N nodes needs N−1 mesh NICs per node. The design is not limited to 3
  nodes, but 3 is the target and the only tested size. Nodes added later need the same cabling.
- **Two nodes and a witness:** a witness node runs no Longhorn replicas, instance managers or VMs,
  and the storage network checks already skip witness nodes. The storage network then only connects
  the two data nodes, so one direct cable is enough and isolated mode is not needed: a VlanConfig
  with that single NIC, selecting the two data nodes, has no loop. This is cheaper still, but
  volumes have at most 2 replicas, so the reduced redundancy described below applies. The docs will
  cover this layout; it is not part of the isolated-mode work.
- **Shared BMC ports:** on servers whose BMC shares a mesh port (NC-SI), BMC broadcasts (for example
  DHCP) appear on the mesh. They are harmless and not relayed, but the docs will recommend a
  dedicated BMC port.

#### Multiple cables per node pair (four NICs per node)

A user with four spare NICs could cable two links per pair, expecting per-pair redundancy. Plain
isolated ports are the wrong tool for that. With two isolated ports leading to the same peer, the
bridge decides the egress port only by where it last learned the peer MAC. There is no control over
which link carries the traffic, no load balancing, no detection of one-sided failures, and every
broadcast reaches the peer twice. (Raised by @rrajendran17 during review.)

The right design for that case is **one bond per peer, with each bond as an isolated port**. Each
bond handles failover and hashing between its two links, with ARP or LACP monitoring for one-sided
failures, and isolation still prevents relaying between peers. That needs a per-peer NIC grouping in
the API and is a **follow-up**. This HEP covers one link per peer, and the docs will say so.

### Alternatives considered

| Option | Why not |
|---|---|
| Broadcast bond (works today) | Correct but sends every frame on every cable: about 55% of the isolated-mode write throughput, double TX on every node. |
| Other bond modes (active-backup, balance-*, 802.3ad) | Treat links as interchangeable paths to one L2 domain. In a mesh each link reaches a different node, so frames go to the wrong neighbor. 802.3ad also needs a switch. |
| STP on `<cn>-br` | Blocks one cable to break the loop, so one pair relays through the third node: a wasted cable and a node acting as a forwarder. The controller runs with STP off. |
| Routed mesh (FRR/OSPF or static `/32` routes, one subnet per link) | Allows relaying on failure, but the storage network is a single L2 subnet with dynamic Whereabouts IPs. It would need a route controller that watches Longhorn pods, and changes to Longhorn's network model. |
| A storage switch | The cost and extra failure point this feature exists to avoid. Remains the right choice for larger clusters. |
| Out-of-band DaemonSet that isolates ports (the first prototype) | Works, but edits a bridge owned by the network controller, and is invisible to VlanStatus, LinkMonitor and the UI. |

#### Where the mode is configured

The mode is on the **VlanConfig** (`uplink.mode`), next to the bond options it replaces, and
follows the same pattern as the uplink MTU (suggested by @w13915984028). The first VlanConfig of a
cluster network sets the mode, and the manager records it on the ClusterNetwork (the `uplink-mode`
label, plus the source VlanConfig, as `uplink-mtu` and `mtu-source-vc` do for the MTU). The webhook
rejects later VlanConfigs with a different mode, and the UI preselects and locks the mode when
another VlanConfig is created for that cluster network. The ClusterNetwork stays a name for a
network with no uplink settings of its own.

A "Switchless mesh" option on the ClusterNetwork was considered. It would make "one mode per
cluster network" structural, but it adds the first uplink-related field to the ClusterNetwork,
while MTU already established the "first VlanConfig decides" pattern.

### Test plan

Unit tests (in PR #445): webhook mode rules, one mode per cluster network, NAD and
HostNetworkConfig guardrails, LinkMonitor rule generation. UI model tests in PR #1203.

**Test setup.** Integration testing needs a dedicated cluster with a direct-cabled mesh: on each
node, one NIC cabled to each other node, plus a separate management network. Regular QA clusters
behind a switch cannot run these cases. For functional (not performance) tests, a nested setup
should also work: three Harvester VMs and three virtual networks, each connecting exactly two of the
VMs, so every virtual link behaves like a cable. The nested setup still needs to be validated, and
QA should plan for one of the two before testing starts. Failure cases need a way to take a single
link down (unplug the cable, or set the virtual link down from the hypervisor).

Integration (3 nodes with a direct-cabled mesh):

1. Create an isolated VlanConfig: on every node, the NICs are isolated ports of `<cn>-br`, no
   `<cn>-bo` exists, VlanStatus is Ready, the cluster network is labeled, and idle links stay
   silent (no loop).
2. Storage network on the mesh: instance managers get `lhnet1`. A 3-replica write puts one copy on
   each cable of the engine node and 0 on the third cable.
3. RWX network and VM migration network on the mesh. Migration traffic uses the direct cable
   between source and target.
4. Guardrails: a VM network NAD, a HostNetworkConfig, a bond VlanConfig on the same cluster network,
   and isolated mode on `mgmt` are all denied.
5. Failures during a VM write: one cable down, one node's both cables down, node reboot. The write
   completes, the volume degrades and recovers, the port returns isolated after link-up or reboot,
   and the LinkMonitor shows the state.
6. Permanent cable failure (Story 4): with the A–B cable down, a migration between A and B fails,
   migrating the affected VMs to C restores healthy volumes, and `MeshComplete` is `False` on A
   and B.
7. Switch an existing cluster network bond → isolated → bond using the documented procedure.
8. Tagged storage VLAN on an isolated cluster network.
9. A second VlanConfig on an isolated cluster network gets the mode from the first; a different
   mode is rejected.

### Upgrade strategy

- The field is optional and defaults to today's behavior. Existing VlanConfigs and cluster networks
  are not changed. No migration is needed.
- The CRD gains the optional `uplink.mode` enum, shipped with the network controller chart.
- The UI shows the option only on clusters with the new controller (feature flag for v1.10.0).
- **Downgrade:** an older controller ignores `uplink.mode` and would build a bond. Remove isolated
  VlanConfigs, or switch them to a broadcast bond, before downgrading. This will be documented.
- Users who run a broadcast bond on a mesh today can switch to isolated mode with the existing
  "change a storage network VlanConfig" procedure (VMs stopped).

## Note

Prototype and measurements: Harvester v1.9.1-rc3, 3 × Ampere Altra (arm64), Intel I350 1 GbE
direct-cabled triangle, management on a separate 10 GbE network. Faster links (10/25 GbE DAC)
change the absolute numbers but not the design. Detailed results are in the Epic.
