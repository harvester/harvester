# Longhorn V2 Data Engine RDMA (RoCEv2) Storage Network

## Summary

The Longhorn V2 (SPDK) data engine moves replica data between the engine and its
replicas over NVMe-oF. Today that fabric is TCP-only, and on Harvester it is
carried over the Harvester storage network, which is always a Linux-bridge NAD.
This proposal adds the ability to run the V2 engine⇄replica fabric over **RDMA
(RoCEv2)** on Harvester: a bridgeless storage network bound directly to a
dedicated RDMA-capable interface (macvlan/ipvlan over the RoCE PF, with
host-device or SR-IOV as alternatives), RDMA device access into the pod via the
shared RDMA device plugin, the host-side enablement (kernel modules, hugepages,
QoS) needed for RoCEv2, and a way to select the RDMA transport per volume through
a StorageClass.

RDMA lowers CPU cost and tail latency for the replica fabric. On an arm64 /
BlueField-2 RoCEv2 test bed we measured up to **3× sequential-read throughput**,
**~4× sequential-write throughput**, and **3–5× lower QD1 latency** versus TCP
for the V2 engine⇄replica path, on both the `nvme` and `aio` disk backends. Full
matrix under [Benchmark evidence](#benchmark-evidence).

The base OS already ships everything on the data path (`rdma-core` incl. the
`mlx5` provider, and inbox `mlx5_ib`/`rdma_cm`/`nvme-rdma` kernel modules), so
this is primarily a networking + enablement + Longhorn-plumbing change, not an
OS-driver-packaging change.

### Related Issues

- Harvester feature request: https://github.com/harvester/harvester/issues/11460
- Harvester storage-network EPIC: https://github.com/harvester/harvester/issues/9628
- Longhorn V2 RDMA transport (upstream, working implementation + benchmarks):
  https://github.com/longhorn/longhorn/issues/13796

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

#### Network-layer validation (the Harvester attachment + device-access path)

The table above measures RDMA *through Longhorn*. This second set validates the
**Harvester-specific mechanism this HEP proposes** — the bridgeless
`ipvlan-over-PF` NAD (A) plus the `k8s-rdma-shared-dev-plugin` (B) — at the raw
fabric layer, independent of Longhorn, so the two concerns are not conflated.
Measured 2026-08-21, cross-node pod↔pod (`ib_write_bw`/`ib_write_lat`) over the
proposed attachment, buffers bound to NUMA node0 (both NICs on node0):

| Fabric metric | Result |
|---|---:|
| Per-port throughput (single QP), PF0 | **24.51 Gb/s** (25GbE line rate) |
| Per-port throughput (single QP), PF1 | **24.51 Gb/s** (25GbE line rate) |
| Dual-PF aggregate (both ports at once) | **49.02 Gb/s** |
| QD1 write latency (typical / p99) | **~3.5 µs / ~6 µs** |

Takeaways: (1) the proposed pod-attachment mechanism reaches **wire-rate
RoCEv2** — the ipvlan child carries a correct v2 GID and the shared-dev-plugin
scopes the right `/dev/infiniband` device into the pod, no bind-mount. (2) Using
**both PFs independently** (advertised as two shared resources) linearly
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
- Keep the default behavior unchanged: TCP over the existing bridge NAD remains
  the default; RDMA is strictly opt-in.

### Non-goals

- Switch-side lossless-fabric configuration (PFC/ECN/DSCP on the physical
  switch). We document the requirement and configure the host side; the switch
  is the operator's responsibility.
- RDMA for the host-facing volume frontend (guest/VM I/O). This proposal covers
  only the internal engine⇄replica fabric; the frontend stays NVMe-TCP/ublk.
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
   a RoCE-capable NIC on a dedicated interface; host RoCE QoS (trust=dscp / PFC
   prio3) and MTU are set; the switch is configured lossless.
2. The operator configures the RDMA storage network (extended `storage-network`
   setting in `rdma` mode, or the equivalent UI), naming the dedicated interface
   and the subnet. Harvester validates readiness on all nodes, quiesces
   workloads (all VMs stopped / all Longhorn volumes detached — same disruptive
   day-2 flow as today's storage network), builds a bridgeless NAD, and syncs it
   to Longhorn.
3. The operator (or the Harvester UI) creates/uses a V2 StorageClass with
   `dataEngineTransport: rdma`.
4. Users create PVCs from that StorageClass; replica traffic runs over RoCEv2.
5. Verification commands and expected output are in the Test plan.

### API changes

- **Harvester `storage-network` setting** gains a `mode` and interface binding.
  Current value: `{"vlan","clusterNetwork","range","exclude"}`. Extended (draft):

  ```json
  {
    "mode": "rdma",
    "transport": "macvlan",        // macvlan | ipvlan | host-device | sriov
    "clusterNetwork": "",           // optional; empty in bridgeless mode
    "masterInterface": "enp1s0f0np0", // the RoCE PF (or bond) to bind onto
    "vlan": 0,
    "range": "10.25.0.0/24",
    "exclude": ["10.25.0.1"]
  }
  ```

  **Open design question — maintainer input requested:** extend this setting
  with a mode, or introduce a dedicated CRD in the style of `HostNetworkConfig`
  (`network.harvesterhci.io/v1beta1`)? Both are laid out neutrally under Design;
  we are explicitly seeking a preference from the network/storage maintainers
  before committing.

- **StorageClass parameter** `dataEngineTransport: tcp|rdma` (default `tcp`),
  passed through to Longhorn's per-volume `dataEngineTransport` (upstream
  Longhorn work, #13796). Immutable per volume.

- **Longhorn `storage-network` setting**: Harvester keeps syncing the NAD ref
  into it (unchanged mechanism); only the NAD it points at changes shape.

- **Longhorn `v2-data-engine-rdma-device-resource` setting** (new, upstream
  #13796): Harvester sets this to the shared-dev-plugin resource name (e.g.
  `rdma/hca_shared_f0`) so the instance-manager pod requests the RDMA device
  declaratively. Empty by default (legacy host-mount behavior). This is the
  concrete shared interface between the Harvester device plugin (B) and Longhorn.

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
`CreateBridgeConfig`). Add a sibling builder that, in `rdma` mode, emits a
bridgeless NAD. **Primary:** `macvlan` or `ipvlan` with `master:
<masterInterface>` (the RoCE PF), which makes the pod interface (`lhnet1`) a real
child of the PF so a RoCEv2 GID exists for the pod IP — validated on the
reference homelab (ipvlan L2). **Alternatives:** `host-device` (moves the whole
PF into the pod — cleanest native GID, but *exclusive*: the host and any future
frontend consumer lose that PF while the pod holds it — see Dual-port below); or
a `sriov` NAD referencing a VF resource. Whereabouts IPAM is reused unchanged in
all cases.

Rationale for bridgeless: Harvester's Linux bridge is fundamental to the
*cluster-network* model because it is the substrate for VM VLAN trunking
(`harvester-network-controller` `network/vlan/vlan.go`, `iface/bridge.go`); the
storage network merely inherits it. RoCEv2 cannot traverse a Linux software
bridge (veth has no PF-backed GID). A dedicated backend-only interface does not
need a bridge, so we bypass it.

**(B) RDMA device access (device plugin).**
The pod also needs the RDMA verbs devices. On the homelab we proved this with a
manual `/dev/infiniband` bind-mount in shared RDMA mode — functional, but not a
supportable product mechanism. **Primary:** the NVIDIA-maintained,
open-source [`k8s-rdma-shared-dev-plugin`](https://github.com/Mellanox/k8s-rdma-shared-dev-plugin),
a device plugin that advertises a PF's RDMA device as a **shared** resource
(`rdma/<name>`) that many pods can request at once — no SR-IOV, no VF pool. It is
the declarative, sanctioned form of the bind-mount, and its sharing is what keeps
a **future frontend consumer** (or a second port) able to use the same PF (see
Dual-port). **Alternative:** the SR-IOV device plugin, which advertises VFs
(exclusive per pod) and adds hardware isolation/QoS — the right tool for
VM-facing RDMA, already owned by the SR-IOV network devices HEP.

Delivery: this is a **container workload** (DaemonSet), not an OS package — so it
carries no OS-image licensing/packaging concern and ships through an existing
Harvester mechanism (managed Helm chart / AddOn, the same model as Multus and
Whereabouts today). The NVIDIA Network Operator bundles the plugin + RDMA-CNI +
Multus + Whereabouts; Harvester would more likely vendor just the shared-dev
plugin chart rather than adopt the whole operator. (License to confirm for the
HEP; expected Apache-2.0.)

**(C) Host RoCEv2 enablement (OS / node-manager).**
Reuse the Longhorn V2 data-engine host-prep mechanism
(`20240709-longhorn-v2-data-engine`): today node-manager loads
`vfio_pci`/`nvme_tcp` and allocates hugepages, persisted in
`/oem/99_settings.yaml` (Elemental/yip). Extend that list with the RDMA stack —
`mlx5_ib`, `ib_core`, `rdma_cm`/`rdma_ucm`, `nvme_rdma` — and persist host RoCE
QoS (trust=dscp, PFC prio3) analogous to a `host-roce-qos` unit. All of these
modules are inbox in the shipping OS (SUSE Linux Micro 6.2, kernel 6.12); the
`rdma-core` userspace (incl. the `mlx5` provider) is already shipped (v56.1).
No proprietary NVIDIA MOFED/DOCA component is required or added.

Base-OS tooling deliverables (via OBS baseos requests + `harvester/os2` wiring,
per the kernel-module-devel HEP process): keep `rdma-core`/`libibverbs`/
`librdmacm`/`mlx5` provider (already present); optionally add `mstflint` (host
firmware/RoCE inspection) and the `rdma-core` diagnostic CLIs
(`ibv_devinfo`/`ibstat`, iproute2 `rdma`) for bring-up/support. Hugepages
enablement is inherited from the V2 data-engine flow (not RDMA-specific).

Fallback (documented, not required): if a future kernel/NIC ever needs an
out-of-tree `mlx5`/OFED newer than SLE ships, use the sanctioned
`harvester-kernel-module-devel` image + CloudInit-CR loading (with MOK signing
for Secure Boot), or an NVIDIA MOFED driver-container delivered as a Harvester
AddOn. This keeps SUSE shipping nothing proprietary — the operator pulls
NVIDIA's container under NVIDIA's EULA at their discretion.

**(D) Longhorn transport selection + shared-device interface.**
Harvester exposes the StorageClass parameter `dataEngineTransport: rdma`, which
maps to Longhorn's per-volume `dataEngineTransport` (upstream #13796). Transport
is chosen per volume and is immutable — a single RDMA-capable storage network
carries both TCP and RDMA volumes simultaneously (validated: sibling TCP and
RDMA volumes on the same fabric).

The device-access handoff between (B) and Longhorn is now a **concrete, shared
interface** rather than a bind-mount: the upstream V2 data engine gained a
cluster setting **`v2-data-engine-rdma-device-resource`** (longhorn-manager,
tracked in #13796). Its value is the name of the extended resource the V2
instance-manager pod should request; when non-empty, longhorn-manager injects
`resources.limits[<name>] = 1` into the IM pod. Harvester's role is therefore
minimal and declarative: deploy the shared-dev-plugin (B) so it advertises e.g.
`rdma/hca_shared_f0`, then set the Longhorn setting to that same string. The
plugin scopes the correct `/dev/infiniband` device into the pod and the NAD from
(A) supplies the GID-bearing interface — no privileged host mount, and many IM
pods (and a future frontend consumer) share the one PF. Leaving the setting
empty preserves the legacy privileged-host-mount behavior, so the change is
backward compatible.

Longhorn also now reports an **`RDMACapable` node condition** and rejects an
RDMA volume at admission unless enough RDMA-capable nodes exist, confining RDMA
replicas to capable nodes (longhorn-manager, #13796). Harvester's capability
discovery (E) can surface this condition directly instead of re-detecting.

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

Picking (A) × (B) is a trade between simplicity, sharing, and isolation. The
recommended default is **macvlan/ipvlan + shared-dev-plugin** — proven, shareable,
and it does not spend the frontend option.

| Attachment (A) | Device access (B) | NIC sharing | Frontend later? | Isolation/QoS | Complexity |
|---|---|---|---|---|---|
| macvlan / ipvlan over PF | shared-dev-plugin | host + many pods share PF | **yes** | none | low — **recommended** |
| host-device (whole PF) | shared-dev-plugin / bind-mount | **exclusive** to the pod | no (PF is gone from host) | full (owns PF) | low, but forecloses frontend |
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
  virt-launcher pod) can no longer use it. The **shared-dev-plugin (+
  macvlan/ipvlan)** path shares the PF, and **SR-IOV** partitions it — both keep
  the frontend door open. Because frontend RDMA is a current non-goal but a
  plausible Phase 2, the recommended default deliberately preserves it; operators
  who want the clean exclusive `host-device` model can also **reserve PF1 for the
  frontend** and dedicate PF0 to the backend.

### HA storage NIC: dual-PF-independent (recommended) vs RoCE LAG bond

For HA / higher aggregate bandwidth there are two options; the reference bed
tested both and **recommends dual-PF-independent over a bond.**

**Recommended — both PFs, independently, via the device plugin.** The
shared-dev-plugin advertises each PF as its own resource (`rdma/hca_shared_f0`,
`rdma/hca_shared_f1`); a single pod can request both and receives both
`/dev/infiniband` devices (`mlx5_0`+`mlx5_1`) = two independent active RoCEv2
paths. This **linearly aggregates to 49 Gb/s** (measured, Network-layer
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

### Upgrade strategy

- Default is unchanged (TCP bridge NAD); clusters not opting into RDMA are
  unaffected.
- Enabling RDMA is a disruptive day-2 operation (all VMs stopped / volumes
  detached), identical to today's storage-network reconfiguration.
- Host RDMA modules/QoS are persisted via `/oem` cloud-init and re-applied on
  reboot. A kernel bump that changes inbox `mlx5` is handled by the base OS
  image; only the OOT fallback path would need a module rebuild
  (kernel-module-devel HEP covers this).
- Existing TCP V2 volumes are not converted; `dataEngineTransport` is chosen at
  volume creation and is immutable.

## Note

Stakeholders to engage (from HEP/commit history): @tserong (Longhorn V2
data-engine + kernel-module-devel), @tjjh89017 (storage-network),
@FrankYang0529 (vm-migration-network), @rrajendran17 (HostNetworkConfig /
clusternetwork L3), @Yu-Jack (NAD↔interface, driver-by-annotation),
@ibrokethecloud (SR-IOV network devices).

Open technical questions:
- HA fabric: the reference bed recommends **dual-PF-independent** (both PFs as
  separate shared resources → 49 Gb/s aggregate, HA at Longhorn's replica layer,
  zero switch/DPU config) over a bond. A host-side active-backup bond did **not**
  form a RoCE LAG on our BlueField-2 (DPU eSwitch owns the physical ports → LAG
  is a DPU-ARM-side change; see Design → HA storage NIC). Is dual-PF-independent
  acceptable as the shipped HA story, with RoCE LAG left as an advanced/plain-
  ConnectX option? Maintainer input welcome.
- Re-implementing the "spans all nodes" guarantee against a raw interface name
  rather than a cluster network.
- Interaction with the SR-IOV HEP exclusion rule (a NIC is today either a
  cluster-network bridge member or an SR-IOV device, never both).
