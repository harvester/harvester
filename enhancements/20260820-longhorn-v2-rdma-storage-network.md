# Longhorn V2 Data Engine RDMA (RoCEv2) Storage Network

## Summary

The Longhorn V2 (SPDK) data engine moves replica data between the engine and its
replicas over NVMe-oF. Today that fabric is TCP-only, and on Harvester it is
carried over the Harvester storage network, which is always a Linux-bridge NAD.
This proposal adds the ability to run the V2 engine⇄replica fabric over **RDMA
(RoCEv2)** on Harvester: a bridgeless storage network bound directly to a
dedicated RDMA-capable interface (macvlan/ipvlan over the RoCE PF, with
host-device or SR-IOV as alternatives), the host-side enablement (kernel
modules, hugepages) needed for RoCEv2, and a way to select the RDMA transport per
volume through a StorageClass. The V2 instance-manager pod is already privileged
and sees the host RDMA devices, so no device plugin is needed.

RDMA lowers CPU cost and tail latency for the replica fabric. On an arm64 /
BlueField-2 RoCEv2 test bed we measured up to **3× sequential-read throughput**,
**~4× sequential-write throughput**, and **3–5× lower QD1 latency** versus TCP
for the V2 engine⇄replica path, on both the `nvme` and `aio` disk backends. Full
matrix under [Benchmark evidence](#benchmark-evidence).

The base OS ships the inbox RDMA kernel modules (`mlx5_ib`, `ib_uverbs`,
`rdma_cm`, `rdma_ucm`), and the RDMA userspace (`rdma-core`, verbs providers)
ships inside the Longhorn instance-manager image, so this is a networking +
enablement + Longhorn-plumbing change, not an OS-driver-packaging change.

### Related Issues

- Harvester feature request: https://github.com/harvester/harvester/issues/11460
- Harvester storage-network EPIC: https://github.com/harvester/harvester/issues/9628
- Harvester RDMA epic: https://github.com/harvester/harvester/issues/11989
- NAD webhook rejects non-bridge NADs: https://github.com/harvester/harvester/issues/11990
- UI: Data Engine Transport option in the V2 StorageClass form: https://github.com/harvester/harvester/issues/11991
- Longhorn V2 RDMA transport (upstream, working implementation + benchmarks):
  https://github.com/longhorn/longhorn/issues/13796
- `longhornctl` RDMA preflight: https://github.com/longhorn/longhorn/issues/14329

## Motivation

### Benchmark evidence

Measured on a 3-node arm64 cluster (NVIDIA BlueField-2 in host/NIC mode,
RoCEv2 over a dedicated 25GbE storage fabric), Longhorn V2, `numberOfReplicas: 3`,
engine and one replica co-located, replica fabric on the RoCEv2 storage network.
Each leg used a **freshly created volume on a wiped disk** so TCP and RDMA differ
*only* in transport; RDMA was confirmed per volume via SPDK
`bdev_nvme_get_controllers` reporting `trtype=RDMA` on all three engine⇄replica
controllers. `fio` against `/dev/longhorn/<vol>`, 8 SPDK reactor cores
(`data-engine-cpu-mask 0xff`).

| Metric (higher better ↑ / lower better ↓) | Backend | TCP | RDMA | Gain |
|---|---|---:|---:|---:|
| Seq read, 1M ↑ (MiB/s)      | nvme | 176.7 | 531.1 | **3.0×** |
| Seq read, 1M ↑ (MiB/s)      | aio  | 243.3 | 611.9 | **2.5×** |
| Seq write, 1M ↑ (MiB/s)     | nvme | 71.6  | 275.0 | **3.8×** |
| Seq write, 1M ↑ (MiB/s)     | aio  | 77.0  | 328.9 | **4.3×** |
| QD1 read latency ↓ (µs)     | nvme | 598   | 371   | **1.6× lower** |
| QD1 read latency ↓ (µs)     | aio  | 1201  | 332   | **3.6× lower** |
| QD1 write latency ↓ (µs)    | nvme | 1478  | 296   | **5.0× lower** |
| QD1 write latency ↓ (µs)    | aio  | 1145  | 272   | **4.2× lower** |
| Rand read, 4k ↑ (kIOPS)     | nvme | 47.4  | 57.6  | 1.2× |
| Rand read, 4k ↑ (kIOPS)     | aio  | 47.7  | 57.8  | 1.2× |
| Rand write, 4k ↑ (kIOPS)    | nvme | 6.9   | 10.5  | 1.5× |
| Rand write, 4k ↑ (kIOPS)    | aio  | 5.8   | 11.6  | 2.0× |

Takeaways: (1) the win is a **transport property** — it holds on both disk
backends, so it is not an artifact of the local disk path. (2) It is largest
where the network dominates: **sequential bandwidth (~3–4×)** and
**QD1 tail latency (up to 5× lower)**, i.e. exactly the replica-fabric cost that
RDMA offloads. (3) The 4k-random gap narrows once reactor cores are plentiful
(8-core here), which is expected — at high core counts TCP is no longer
reactor-bound for small I/O, but the sequential/latency advantage remains. A
matching 2-core run showed the same shape with the sequential gap slightly
smaller and the random gap slightly larger (transport help matters more when CPU
is scarce). Full per-leg tables and methodology are in the upstream Longhorn
tracking issue (#13796).

#### CPU cost (Longhorn master, Harvester v1.9.1, 2026-10-20)

Same hardware, Harvester v1.9.1-rc3 with Longhorn master and the RDMA PR set,
ipvlan NAD on the RDMA port as Longhorn's storage network, 3-replica V2 volume.
SPDK reactors busy-poll, so CPU was measured with SPDK's own reactor accounting
(busy vs idle ticks) at equal load:

| fio test (2 reactors) | SPDK busy per IO, engine TCP / RDMA | replicas TCP / RDMA |
|---|--:|--:|
| 4k randread @ 10k IOPS | 61 / 42 µs | 23 / 14 µs |
| 4k randwrite @ 5k IOPS | 116 / 61 µs | 65 / 31 µs |
| 1M write @ 300 MiB/s | 3494 / 1164 µs | 2017 / 468 µs |

PostgreSQL 17 (pgbench, 32 clients) in two Harvester VMs with data disks on
`dataEngineTransport: tcp` vs `rdma` StorageClasses, 8 SPDK reactors: read-write
4663 vs 5061 TPS, read-only 12426 vs 17145 TPS, and lower SPDK busy time per
transaction with RDMA (engine 198 vs 122 µs, replicas 84 vs 33 µs). Going from 2
to 8 reactors raised TPS for both transports, so the CPU RDMA saves is best spent
on more reactors for the work that stays TCP (the frontend) and for more volumes.

#### Network-layer validation (the Harvester attachment + device-access path)

The table above measures RDMA *through Longhorn*. This second set validates the
**Harvester-specific mechanism this HEP proposes** — the bridgeless
`ipvlan-over-PF` NAD (A) — at the raw fabric layer, independent of Longhorn, so
the two concerns are not conflated. (This August run used the
`k8s-rdma-shared-dev-plugin` for device access; it turned out to be unnecessary,
see (B).)
Measured 2026-08-21, cross-node pod↔pod (`ib_write_bw`/`ib_write_lat`) over the
proposed attachment, buffers bound to NUMA node0 (both NICs on node0):

| Fabric metric | Result |
|---|---:|
| Per-port throughput (single QP), PF0 | **24.51 Gb/s** (25GbE line rate) |
| Per-port throughput (single QP), PF1 | **24.51 Gb/s** (25GbE line rate) |
| Dual-PF aggregate (both ports at once) | **49.02 Gb/s** |
| QD1 write latency (typical / p99) | **~3.5 µs / ~6 µs** |

Takeaways: (1) the proposed pod-attachment mechanism reaches **wire-rate
RoCEv2** — the ipvlan child carries a correct v2 GID. (2) Using **both PFs
independently** linearly
aggregates to ~2× a single port with **zero switch or DPU configuration** —
which motivates the bond reframe below. (3) NUMA locality is mandatory for the
aggregate: parallel streams collapse (~1 Gb/s) unless each is bound to the NIC's
NUMA node — a host-prep note, not a transport limitation. macvlan-over-PF also
carries a valid GID but ran ~20× slower (per-child MAC not steered to fast-path
queues) → **ipvlan is the recommended attachment**, consistent with the matrix.

### Goals

- Allow the Longhorn V2 engine⇄replica NVMe-oF fabric to run over RDMA (RoCEv2)
  on Harvester, selectable **per volume** via a StorageClass parameter.
- Provide a **bridgeless** storage-network option that binds directly to a
  dedicated RDMA-capable interface (macvlan/ipvlan over the RoCE PF, or SR-IOV
  VF) so the pod interface carries a RoCEv2 GID — which a bridge+veth cannot.
- Ensure the Harvester base OS has the packages and tooling required to bring up
  and operate RoCEv2 (kernel modules loaded, hugepages, host QoS, and optional
  diagnostic/firmware tooling), delivered through existing Harvester mechanisms.
- Report RDMA capability/health per node so an RDMA storage network is only
  offered where the hardware and stack are present.
- Let users pick the transport when creating a V2 StorageClass in the UI.
- Keep the default behavior unchanged: TCP over the existing bridge NAD remains
  the default; RDMA is strictly opt-in.

### Non-goals

- Switch-side lossless-fabric configuration (PFC/ECN/DSCP on the physical
  switch). We document the requirement and configure the host side; the switch
  is the operator's responsibility.
- RDMA for the host-facing volume frontend (guest/VM I/O) in this phase. This
  proposal covers the internal engine⇄replica fabric; the frontend stays
  NVMe-TCP/ublk for now. Frontend RDMA is planned as the next phase, since DPU-
  equipped nodes consume volumes away from the engine; the shared-PF device
  access chosen below keeps that path open.
- RDMA to VMs / VF passthrough to guests (already covered by the SR-IOV network
  devices HEP).
- Live migration of RDMA-backed volumes across an OFED/kernel change; backing
  images and encryption for RDMA volumes (inherit V2 data-engine non-goals).
- BlueField DPU-ARM-side offload and GPUDirect (out of scope; host-mode
  ConnectX/BlueField RoCEv2 only).
- NIC firmware provisioning / OFED (MOFED/DOCA) installation as a hard
  dependency — inbox drivers are the supported path; a driver-container fallback
  is noted but not required.

## Proposal

### User Stories

#### Story 1 — Operator enables an RDMA storage backend
Before: the operator can move Longhorn replica traffic onto a dedicated storage
network, but only over a Linux bridge (TCP). RDMA is impossible because the pod
gets a veth with no RoCEv2 GID. After: the operator, on a cluster whose nodes
have a RoCE-capable NIC on a dedicated interface, configures an RDMA storage
network (bridgeless, bound to that interface) and Harvester reports each node's
RDMA readiness.

#### Story 2 — User provisions an RDMA-backed volume
Before: all V2 volumes use TCP. After: the user selects a StorageClass with the
V2 data engine and `dataEngineTransport: rdma`; new volumes' engine⇄replica
fabric runs over RoCEv2, while other volumes remain TCP on the same cluster.

#### Story 3 — Operator verifies and troubleshoots
The operator can confirm on a node that the RoCEv2 GID is present on the storage
interface, that the instance-manager pod sees `/dev/infiniband`, and that a
volume's replica controllers report `trtype=RDMA`.

### User Experience In Detail

1. Prerequisites (operator, before configuring the storage network — this
   ordering is enforced by the webhook, see Design): each participating node has
   a RoCE-capable NIC on a dedicated interface and the RDMA kernel modules
   loaded (`longhornctl check preflight --enable-rdma` verifies this); MTU and any
   lossless-fabric settings (PFC/ECN) are the operator's choice — the reference
   bed ran without PFC.
2. The operator configures the RDMA storage network (extended `storage-network`
   setting in `rdma` mode, or the equivalent UI), naming the dedicated interface
   and the subnet. Harvester validates readiness on all nodes, quiesces
   workloads (all VMs stopped / all Longhorn volumes detached — same disruptive
   day-2 flow as today's storage network), builds a bridgeless NAD, and syncs it
   to Longhorn.
3. The operator creates a V2 StorageClass and picks "Data Engine Transport:
   RDMA" in the UI (#11991), which sets `dataEngineTransport: rdma`. The option is
   disabled when no Longhorn node reports `RDMACapable`.
4. Users create PVCs from that StorageClass; replica traffic runs over RoCEv2.
5. Verification commands and expected output are in the Test plan.

### API changes

- **Harvester `storage-network` setting** gains a `mode`; the interface binding
  is *not* a new field. Current value: `{"vlan","clusterNetwork","range","exclude"}`.
  Extended (draft):

  ```json
  {
    "mode": "rdma",
    "transport": "macvlan",        // macvlan | ipvlan | host-device | sriov
    "clusterNetwork": "storage-rdma", // same field as today; per-node uplink
                                       // still resolved via VlanConfig
    "vlan": 0,
    "range": "10.25.0.0/24",
    "exclude": ["10.25.0.1"]
  }
  ```

  `clusterNetwork` stays a logical name, not a literal interface — a global
  literal interface name breaks the moment the RoCE NIC has a different device
  name on some nodes (mixed hardware, different PCIe enumeration). Reusing
  `clusterNetwork` means the per-node uplink is resolved exactly the way it is
  today, via one or more `VlanConfig` (`network.harvesterhci.io/v1beta1`)
  objects scoped by `spec.nodeSelector`, so node A can carry the RDMA NIC as
  `eno1` and node B as `enp3s0f0` under the same `clusterNetwork`. No new field,
  no new per-node config surface.

  **Open design question — maintainer input requested:** extend this setting
  with a mode, or introduce a dedicated CRD in the style of `HostNetworkConfig`
  (`network.harvesterhci.io/v1beta1`)? Both are laid out neutrally under Design;
  we are explicitly seeking a preference from the network/storage maintainers
  before committing.

- **StorageClass parameter** `dataEngineTransport: tcp|rdma` (default `tcp`),
  passed through to Longhorn's per-volume `dataEngineTransport` (upstream
  Longhorn work, #13796). It can be changed only while the volume is detached.

- **Longhorn `storage-network` setting**: Harvester keeps syncing the NAD ref
  into it (unchanged mechanism); only the NAD it points at changes shape.

- No device-plugin resource or Longhorn setting for device access: the V2
  instance-manager pod is privileged and already sees `/dev/infiniband` (an
  earlier draft proposed a `v2-data-engine-rdma-device-resource` setting; Longhorn
  maintainers asked to drop it, and RDMA volumes ran without it).

- No new guest-facing (KubeVirt/VM) API.

## Design

### Implementation Overview

Getting RDMA into the instance-manager pod is **two orthogonal concerns** that
are easy to conflate:

- **(A) Network attachment** — how the pod gets an IP on a netdev that carries a
  RoCEv2 GID (the L2/L3 plumbing).
- **(B) RDMA device access** — how the pod gets the `/dev/infiniband/*` verbs
  character devices (the RDMA plumbing).

Both must be satisfied; each has independent options. The remaining pieces are
host enablement (C), Longhorn transport selection (D), and capability discovery
(E). Every piece builds on an existing Harvester precedent.

**(A) Bridgeless network attachment (NAD).**
The storage-network controller today synthesizes a `type: "bridge"` NAD over
`<clusterNetwork>-br` with Whereabouts IPAM (`pkg/util/network/common.go`,
`CreateBridgeConfig`); the bridge builder never touches a literal interface
name, only the `clusterNetwork`-derived bridge name — the per-node uplink NIC
is resolved separately via `VlanConfig`. Add a sibling builder that, in `rdma`
mode, emits a bridgeless NAD the same way: it resolves the `clusterNetwork`'s
per-node uplink (via the existing `VlanConfig`/`nodeSelector` mechanism) and
uses that as the `master`, instead of hard-coding an interface name in the
setting. **Primary:** `macvlan` or `ipvlan` with `master: <uplink>`, which makes
the pod interface (`lhnet1`) a real child of the PF so a RoCEv2 GID exists for
the pod IP — validated on the reference homelab (ipvlan L2). **Alternatives:**
`host-device` (moves the whole PF into the pod — cleanest native GID, but
*exclusive*: the host and any future frontend consumer lose that PF while the
pod holds it — see Dual-port below); or a `sriov` NAD referencing a VF
resource. Whereabouts IPAM is reused unchanged in all cases — it is chained
purely as the `ipam` plugin in the NAD config and is independent of the main
CNI type, so it works identically whether the main plugin is `bridge`,
`macvlan`, `ipvlan`, `host-device`, or `sriov`; confirmed on the homelab with an
ipvlan-over-PF NAD.

Two existing behaviors block this today and must change:
- harvester-network-controller's NAD webhook parses every non-kube-ovn NAD as a
  bridge NAD and rejects it ("the length of bridge name is less than 3"). The fix
  (#11990) applies the bridge-specific mutation and validation only to bridge and
  kube-ovn NADs; it was validated on the reference cluster (ipvlan NAD created and
  updated, CNI type change and bridge NADs on a missing cluster network still
  rejected).
- The storage-network controller writes its own NAD back into Longhorn's
  `storage-network` setting whenever they differ, so a manual override only holds
  with Harvester's setting empty. The `rdma` mode makes the controller build and
  sync the non-bridge NAD itself.

Per-node interface names: ipvlan and macvlan take a single `master` name in a
global NAD. Proposed: the network-controller agent (already a per-node
DaemonSet that applies VlanConfig) gives the selected NIC the same alternative
name on every node (`ip link property add dev <nic> altname <cn>-rdma`), and the
NAD uses `"master": "<cn>-rdma"`. The kernel resolves altnames on link lookups
(verified on the reference nodes, kernel 6.12); an end-to-end check with the
ipvlan CNI is pending.

Rationale for bridgeless: Harvester's Linux bridge is fundamental to the
*cluster-network* model because it is the substrate for VM VLAN trunking
(`harvester-network-controller` `network/vlan/vlan.go`, `iface/bridge.go`); the
storage network merely inherits it. RoCEv2 cannot traverse a Linux software
bridge (veth has no PF-backed GID). A dedicated backend-only interface does not
need a bridge, so we bypass it.

**(B) RDMA device access.**
The pod also needs the RDMA verbs devices (`/dev/infiniband/*`). The V2
instance-manager pod is already privileged and mounts the host root, so it sees
every host device, including `/dev/infiniband`, with no extra mechanism. This was
verified on the reference cluster: RDMA volumes ran with no device plugin and no
Longhorn device setting. An RDMA device plugin (`k8s-rdma-shared-dev-plugin`, or
the SR-IOV device plugin for VFs) would only become necessary if the instance
manager stopped being privileged, which is not planned; the SR-IOV device plugin
remains the tool for VM-facing RDMA, owned by the SR-IOV network devices HEP.

**(C) Host RoCEv2 enablement (OS / node-manager).**
Reuse the Longhorn V2 data-engine host-prep mechanism
(`20240709-longhorn-v2-data-engine`): today node-manager loads
`vfio_pci`/`nvme_tcp` and allocates hugepages, persisted in
`/oem/99_settings.yaml` (Elemental/yip). Extend that list with the RDMA stack —
`ib_core`, `ib_uverbs`, `rdma_cm`, `rdma_ucm` and the NIC's provider (e.g.
`mlx5_ib`). On the reference nodes these were already loaded at boot; persisting
them makes it explicit. `nvme_rdma` is not needed for the engine⇄replica path
(SPDK does RDMA in userspace); it only matters for a future host-side RDMA
frontend. The RDMA userspace (`rdma-core`, `libibverbs`, `librdmacm`, providers)
ships in the Longhorn instance-manager image (longhorn/longhorn-instance-manager#1616),
not on the host. All modules are inbox in SUSE Linux Micro 6.2 (kernel 6.12). No
proprietary NVIDIA MOFED/DOCA component is required or added. Host PFC was not
needed on the reference bed (and enabling it on the BlueField-2 DPU uplink broke
the fabric), so lossless configuration stays an operator choice.

`longhornctl check preflight --enable-rdma` (longhorn/longhorn#14329,
longhorn/cli#660) checks the modules, ACTIVE RoCE v2 ports, `/dev/infiniband`,
`ib_core` netns mode, the NIC's NUMA node versus the V2 CPU mask, and the iobuf
pools versus the reactor count; `install preflight --enable-rdma` loads and
persists the modules. Harvester can reuse these checks.

Base-OS tooling (optional, via OBS baseos requests + `harvester/os2` wiring,
per the kernel-module-devel HEP process): `mstflint` (host firmware/RoCE
inspection) and the `rdma-core` diagnostic CLIs (`ibv_devinfo`/`ibstat`,
iproute2 `rdma`, which the reference nodes lack) for bring-up/support. Hugepages
enablement is inherited from the V2 data-engine flow (not RDMA-specific).

Fallback (documented, not required): if a future kernel/NIC ever needs an
out-of-tree `mlx5`/OFED newer than SLE ships, use the sanctioned
`harvester-kernel-module-devel` image + CloudInit-CR loading (with MOK signing
for Secure Boot), or an NVIDIA MOFED driver-container delivered as a Harvester
AddOn. This keeps SUSE shipping nothing proprietary — the operator pulls
NVIDIA's container under NVIDIA's EULA at their discretion.

**(D) Longhorn transport selection.**
Harvester exposes the StorageClass parameter `dataEngineTransport: rdma`, which
maps to Longhorn's per-volume `dataEngineTransport` (upstream #13796). Transport
is chosen per volume (changeable only while detached) — a single RDMA-capable
storage network carries both TCP and RDMA volumes simultaneously (validated:
sibling TCP and RDMA volumes on the same fabric). Harvester's StorageClass mutator
already adds the CDI clone/snapshot annotations to any `dataEngine: v2` class,
including RDMA ones, and VM data disks on an RDMA class worked unchanged.

Longhorn also reports an **`RDMACapable` node condition**, rejects an RDMA
volume at admission unless the data engine is V2 and at least one RDMA-capable
node exists, rejects attaching it to a node that is not RDMA-capable, and
confines RDMA replicas to capable nodes (longhorn-manager, #13796).

**CPU mask and SPDK buffer pools.** RDMA lowers SPDK busy time per IO, and more
reactors (`data-engine-cpu-mask`) raised throughput for both transports. With
RDMA the engine node's target runs two NVMe-oF transports (TCP for the frontend,
RDMA for the local replica), and each takes a per-poll-group iobuf cache, so the
iobuf pools must grow with the reactor count. At 8 reactors with the default
pools (`data-engine-iobuf-small-pool-size` 8192 / `-large-pool-size` 1024), the
engine node logged 126,912 large-pool retries for `nvmf_TCP` and 9,145 for
`nvmf_RDMA` during 1M writes; 32768 / 4096 gave zero retries. A rule of 4096
small / 512 large buffers per reactor matches both data points (the defaults at 2
reactors). Users need to be able to set both the CPU allocation and the pools;
Longhorn already exposes them as settings, and Harvester exposes the V2 hugepage
settings since v1.9. Upstream is discussing whether Longhorn should derive the
pool defaults from the reactor count (longhorn/longhorn-manager#5107); Harvester
should follow that outcome and, until then, document the rule and surface the
preflight check.

**(E) RDMA capability discovery & health.**
Longhorn now publishes an `RDMACapable` node condition (set by its environment
check monitor from `/sys/class/infiniband`, upstream #13796). Harvester surfaces
that condition to the UI so the RDMA storage network is only offered/enabled
where valid, rather than re-detecting the NIC/GID itself. The webhook (below)
consults the same signal to reject enabling on non-capable nodes.

**Webhook changes.** The storage-network validator currently requires a Ready
cluster-network + a VlanConfig spanning all nodes
(`pkg/webhook/resources/setting/validator.go`, `checkVlanStatusReady`,
`checkVCSpansAllNodes`). Add an `rdma`-mode branch that instead validates the
named master interface exists (and is RoCE-capable) on all non-witness nodes,
that the subnet has ≥ node-count addresses, and preserves the
volumes-detached / VMs-stopped guard.

### Mechanism decision matrix

Picking the attachment is a trade between simplicity, sharing, and isolation.
The recommended default is **ipvlan over the PF** — proven at line rate,
shareable, and it does not spend the frontend option (macvlan also works but ran
~20× slower on BlueField-2).

| Attachment (A) | Device access (B) | NIC sharing | Frontend later? | Isolation/QoS | Complexity |
|---|---|---|---|---|---|
| ipvlan (or macvlan) over PF | privileged IM pod (no plugin) | host + many pods share PF | **yes** | none | low — **recommended** |
| host-device (whole PF) | privileged IM pod | **exclusive** to the pod | no (PF is gone from host) | full (owns PF) | low, but forecloses frontend |
| SR-IOV VF | SR-IOV device plugin | VFs partitioned per consumer | yes (spare VFs) | hardware per-VF | high (firmware SR-IOV + VF pool) |

### Dual-port topology & frontend forward-compatibility

Typical RoCE NICs are dual-port (the reference bed has two: `mlx5_0`/PF0 and
`mlx5_1`/PF1). Two related questions:

- **Can both ports be given to the IM pod?** Yes (Multus multi-attach), but
  Longhorn V2 is **single-path** today, so a second port yields nothing until
  multipath / aggregation is added (SPDK `bdev_nvme` multi-path + dual listeners)
  — a future enhancement, not MVP.
- **Does using these ports for the backend foreclose a future RDMA frontend?**
  Only with **exclusive** mechanisms. `host-device` moves the whole PF into the
  IM pod, so a same-node frontend initiator (host-kernel `nvme-rdma` or a
  virt-launcher pod) can no longer use it. The **ipvlan** path shares the PF,
  and **SR-IOV** partitions it — both keep
  the frontend door open. Because frontend RDMA is a current non-goal but a
  plausible Phase 2, the recommended default deliberately preserves it; operators
  who want the clean exclusive `host-device` model can also **reserve PF1 for the
  frontend** and dedicate PF0 to the backend.

### HA storage NIC: dual-PF-independent (recommended) vs RoCE LAG bond

For HA / higher aggregate bandwidth there are two options; the reference bed
tested both and **recommends dual-PF-independent over a bond.**

**Recommended — both PFs, independently.** Each PF carries its own ipvlan
attachment; the privileged pod already sees both RDMA devices (`mlx5_0`+`mlx5_1`)
= two independent active RoCEv2 paths. This **linearly aggregates to 49 Gb/s**
(measured with two shared-device resources in August, Network-layer
validation above) with **zero switch or DPU-side configuration**, and HA lives at
Longhorn's existing cross-node replica layer (a dead port loses one replica leg,
not the volume). This is the design's default and needs no bond at all.

**Alternative — RoCE LAG bond (host-side did NOT form on BlueField-2).** A bond
would be transparent to Longhorn/SPDK in principle — the RDMA path is resolved
**by IP** (`rdma_cm`, `adrfam=ipv4`, `traddr=<pod IP>`), never by device name, so
a `mlx5_bond_0` would be used automatically with no code change. **But on the
reference bed it did not materialize:** a host-side Linux `active-backup` bond
over the two PFs came up at L2 (active slave carried the IP), yet **no RoCE LAG
formed** — `rdma link` still showed separate `mlx5_0`(DOWN)/`mlx5_1`, no
`mlx5_bond_0`, and the backup port went `DISABLED`. Root cause: these are
**BlueField-2 host PFs whose physical ports are owned by the DPU eSwitch**, so a
genuine RoCE LAG/LACP must be configured **DPU-ARM-side**, not by a host bond —
a larger blast radius that is out of scope for MVP. Even where a bond *does* form
(plain ConnectX), active-backup only provides failover (idles a port, no
aggregation); real aggregation additionally needs 802.3ad/LACP + switch config.
Given dual-PF-independent already delivers aggregation **and** HA with none of
this, the bond is documented as a future/advanced option, not the recommended
path. (If pursued: use active-backup/balance-xor/802.3ad — never balance-rr,
which flushes GIDs; put the IP on the bond/upper child never the raw slaves;
prefer ipvlan; persist via `/oem`; and expect the real work to be DPU-ARM-side.)

**Design question — maintainer input requested before full implementation.**
Two paths, presented neutrally; we are seeking the network/storage maintainers'
preference rather than pre-deciding:

- *(A) Extend the `storage-network` setting with an `rdma`/bridgeless mode.*
  Smaller change; reuses the existing controller, Longhorn-setting sync, and
  disruptive day-2 flow. Weaker fit for per-node heterogeneity.
- *(B) A dedicated CRD modeled on `HostNetworkConfig`
  (`network.harvesterhci.io/v1beta1`).* Cleaner for per-node heterogeneity via
  `nodeSelector`, routed L3 IPs, and reboot-persistent programming via netlink;
  larger surface, and needs its own sync into the Longhorn setting.

A plausible hybrid — extend the setting for the flat macvlan/ipvlan-over-PF case
and reuse `HostNetworkConfig` for routed-L3/heterogeneous nodes — is noted only
as one option, not a recommendation. Input welcome on #9628 and this HEP.

### Test plan

Manual verification (mirrors the reference e2e harness; per-volume transport is
asserted, which upstream tests do not do):

1. Configure the RDMA storage network; confirm each node's storage interface has
   a RoCEv2 GID: `ibstat` / `rdma link` / `cat /sys/class/infiniband/*/ports/*/gids/*`
   shows a v2 GID with `ndev=<storage iface>`.
2. Confirm the instance-manager pod has `/dev/infiniband` and a `lhnet1` IP on
   the storage subnet.
3. Create a V2 StorageClass with `dataEngineTransport: rdma`; create a PVC + a
   pod; write and read back data (dd + sha256sum integrity).
4. On the engine node's V2 instance-manager, `bdev_nvme_get_controllers` (SPDK
   JSON-RPC) shows every engine⇄replica controller `trtype=RDMA` on the storage
   subnet; a sibling TCP StorageClass volume shows `trtype=TCP` (proves
   per-volume selection).
5. Snapshot, replica-rebuild, and instance-manager restart on an RDMA volume
   succeed.
6. Negative: on a node without a RoCE NIC, the RDMA storage network is rejected
   / reported unhealthy.
7. `longhornctl check preflight --enable-rdma --rdma-interface <nic>` passes on
   every node (modules, RoCE v2 ports, NUMA locality, iobuf pools vs reactors).
8. UI: the V2 StorageClass form shows "Data Engine Transport"; RDMA is disabled
   when no node is `RDMACapable`.

### Upgrade strategy

- Default is unchanged (TCP bridge NAD); clusters not opting into RDMA are
  unaffected.
- Enabling RDMA is a disruptive day-2 operation (all VMs stopped / volumes
  detached), identical to today's storage-network reconfiguration.
- Host RDMA modules/QoS are persisted via `/oem` cloud-init and re-applied on
  reboot. A kernel bump that changes inbox `mlx5` is handled by the base OS
  image; only the OOT fallback path would need a module rebuild
  (kernel-module-devel HEP covers this).
- Existing TCP V2 volumes are not converted automatically; `dataEngineTransport`
  can be switched to `rdma` while a volume is detached, provided all its replicas
  are on RDMA-capable nodes (Longhorn webhook).

## Note

Stakeholders to engage (from HEP/commit history): @tserong (Longhorn V2
data-engine + kernel-module-devel), @tjjh89017 (storage-network),
@FrankYang0529 (vm-migration-network), @rrajendran17 (HostNetworkConfig /
clusternetwork L3), @Yu-Jack (NAD↔interface, driver-by-annotation),
@ibrokethecloud (SR-IOV network devices).

Open technical questions:
- HA fabric: the reference bed recommends **dual-PF-independent** (both PFs
  used independently → 49 Gb/s aggregate, HA at Longhorn's replica layer,
  zero switch/DPU config) over a bond. A host-side active-backup bond did **not**
  form a RoCE LAG on our BlueField-2 (DPU eSwitch owns the physical ports → LAG
  is a DPU-ARM-side change; see Design → HA storage NIC). Is dual-PF-independent
  acceptable as the shipped HA story, with RoCE LAG left as an advanced/plain-
  ConnectX option? Maintainer input welcome.
- Re-implementing the "spans all nodes" guarantee against a raw interface name
  rather than a cluster network, or via the per-node altname proposed in (A).
- Interaction with the SR-IOV HEP exclusion rule (a NIC is today either a
  cluster-network bridge member or an SR-IOV device, never both).
