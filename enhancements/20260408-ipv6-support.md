# IPv6 Support

This enhancement introduces first-class IPv6 support in Harvester host and VM networking for IPv6-only and dual-stack environments.

The file name is lowercase and uses dashes, following HEP conventions.

## Table of Contents

- [Summary](#summary)
  - [References](#references)
- [Terminology](#terminology)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-goals](#non-goals)
- [IP Family Modes](#ip-family-modes)
  - [Advantages and trade-offs](#advantages-and-trade-offs)
- [Proposal](#proposal)
  - [Scope](#scope)
  - [User Stories](#user-stories)
  - [User Experience In Detail](#user-experience-in-detail)
  - [API changes](#api-changes)
- [Design](#design)
  - [Implementation Overview](#implementation-overview)
  - [Validation Rules](#validation-rules)
  - [Failure Modes and Reporting](#failure-modes-and-reporting)
  - [Test plan](#test-plan)
  - [Upgrade strategy](#upgrade-strategy)
  - [References](#references)
- [Note](#note)


## Summary

Harvester users have requested IPv6 support for multiple years, especially for environments where IPv4 addresses are limited or unavailable. The current networking flows are primarily IPv4-oriented for host management and VM network route configuration, which prevents installation and operations in IPv6-only environments and creates friction for modern dual-stack deployments.

This proposal adds IPv6 support across installation, host network lifecycle, VM network configuration, VM live migration, dedicated Longhorn storage network, management access, and upgrade behavior, while preserving current IPv4 behavior. The scope covers static addressing, SLAAC-based dynamic addressing, IPv6 route settings, Ingress and console proxy reachability over IPv6, and validation. DHCPv6 is out of scope; see Non-goals.

The proposal aligns with Harvester v1.7+ networking architecture, which uses NetworkManager on hosts. Wicked is treated only as a legacy migration concern for upgrades from older releases.

## Terminology

- IPv6-only: Environment where host and cluster networking use only IPv6 addresses.
- Dual-stack: Environment where IPv4 and IPv6 are both enabled and routable.
- SLAAC: Stateless Address Autoconfiguration for IPv6. The host derives its address from a Router Advertisement prefix — no server is required. This is the native IPv6 dynamic addressing mechanism, analogous to DHCPv4 for IPv4 in environments that do not use manually assigned addresses.
- RA: Router Advertisement for IPv6 route and prefix discovery. Required for SLAAC and default gateway assignment.
- RDNSS/DNSSL: Router Advertisement extensions (RFC 6106) that carry DNS recursive server addresses (RDNSS) and domain search lists (DNSSL) in RA messages. NetworkManager processes RDNSS/DNSSL options automatically when `ipv6.method=auto`. Environments whose routers do not send RDNSS must supply DNS nameserver addresses at install time via the installer config.
- Privacy Extensions: RFC 8981 mechanism where the OS generates temporary, randomly-derived IPv6 addresses alongside its stable address. Enabled by default on SLE Micro and most modern Linux distributions. For Harvester cluster nodes, Privacy Extensions must be explicitly disabled (`ipv6.ip6-privacy=0`) on all IPv6 NetworkManager connection profiles to prevent unexpected temporary addresses appearing on cluster interfaces. For SLAAC interfaces, `ipv6.addr-gen-mode=eui64` must additionally be set so the RA-derived address is stable and MAC-based rather than a randomly rotating token. RFC 6106 (RDNSS) and RFC 8981 (Privacy Extensions) are independent: a node can receive DNS from RA while keeping a stable, non-rotating IP address.
- DHCPv6: A server-based dynamic address assignment mechanism for IPv6. Not in scope for this HEP; see Non-goals.
- NDP: Neighbor Discovery Protocol. The IPv6 equivalent of ARP. Used by kube-vip to advertise an IPv6 VIP on the local management network segment.
- VIP: Virtual IP address. The stable management address for the Harvester UI and API, advertised by kube-vip. In IPv6-only mode this is an IPv6 address accessed via bracket notation, for example `https://[2001:db8::10]`.
- node-ip: The address a Kubernetes node advertises to the control plane. RKE2 exposes this as `--node-ip`. In dual-stack, two values are required, one per address family.
- Migration network: A dedicated cluster network (VLAN) used exclusively for KVM/QEMU live migration traffic between nodes. Configured via `HostNetworkConfig` and selected by KubeVirt through a `NetworkAttachmentDefinition`. Carries VM memory state during live migration and is separate from both the management network and the storage network. Must be reachable on every node a VM may migrate to.

## Motivation

### Goals

- Support Harvester installation on IPv6-only and dual-stack management networks.
- Enable IPv6 at the kernel level on all cluster nodes before any IPv6 interface configuration is applied. Where the OS disables IPv6 by default (as on Harvester hosts), the installer and network controller ensure it is enabled as a prerequisite; failure to do so is surfaced as a node condition.
- Support IPv6 route configuration and validation for VM networks where route settings are currently IPv4-specific.
- Support IPv6-aware host network configuration on management and custom cluster networks.
- Support SLAAC mode for host VLAN sub-interfaces as the IPv6-native dynamic addressing mechanism.
- Ensure name resolution is available on first boot in IPv6-only and dual-stack deployments: accept DNS nameserver addresses at install time, and document RDNSS/DNSSL (RFC 6106) via Router Advertisements as the alternative for environments that do not supply static nameservers.
- Ensure stable, non-rotating IPv6 addresses on all Harvester cluster interfaces by disabling IPv6 Privacy Extensions (RFC 8981). Addresses that rotate break components binding to a specific IP (Whereabouts, KubeVirt migration, Longhorn storage). The implementation details are in the Design section.
- Preserve backward compatibility for IPv4-only clusters. All API changes are additive; existing `HostNetworkConfig` resources using only IPv4 fields (`mode`, `ips`) continue to work without modification.
- Ensure IPv6-only RKE2 etcd liveness checks pass by guaranteeing `::1 localhost` is present in `/etc/hosts` on each node at install time.
- The IP family mode (IPv4-only, dual-stack, IPv6-only) is chosen at install time and is immutable for the lifetime of the cluster. Changing address family post-install is not supported and is rejected by the admission webhook. A fresh install is required to change the IP family of a cluster.
- Provide explicit upgrade and rollback guidance for IPv6-related configuration changes.
- Configure kube-vip to advertise an IPv6 VIP via NDP for IPv6-only and dual-stack management networks, so the management URL is reachable over IPv6.
- Configure RKE2 control-plane settings (node-ip, cluster-cidr, service-cidr, tls-san, stack-preference) for the selected IP family so the Kubernetes cluster operates correctly in IPv6 and dual-stack environments.
- Enable IPv6 in the kube-ovn overlay that carries VM network traffic, so VM networks support IPv6 CIDR and gateway settings end-to-end.
- Ensure the Harvester web UI, API, and VM VNC/Serial console proxy are reachable via the IPv6 management VIP. This requires the Ingress controller to bind on IPv6 and the console proxy to accept IPv6 connections, so that operators and users in IPv6-only environments can reach all management surfaces.
- Support IPv6 addressing on the VM live migration network so VMs can be live-migrated between nodes in IPv6-only and dual-stack clusters. The migration network is a `HostNetworkConfig`-managed VLAN; IPv6 support there follows directly from the host network config changes in this HEP. KubeVirt selects the migration network via a `NetworkAttachmentDefinition` and binds the migration server on the interface address — both Multus and KubeVirt v1.7.0 support IPv6 for this path.
- Support IPv6 addressing on the dedicated Longhorn storage network (VLAN). The storage network is configured via the Harvester `storage-network` setting which references a Multus `NetworkAttachmentDefinition`; IPv6 support requires extending the setting schema to accept an IPv6 or dual-stack CIDR, updating the Whereabouts IP pool configuration, and generating the NAD with the correct CIDR. Longhorn v1.11.1 imposes no IPv4-only restriction on the storage network — the address family is determined by the NAD delegate config.

### Non-goals

- Replacing current CNI architecture in this HEP.
- Implementing NAT64, DNS64, or other protocol translation services.
- Automatic conversion of all existing IPv4-only user network definitions to IPv6.
- Guest OS IPv6 configuration: injecting IPv6 addresses via Cloud-Init after NIC attachment, or displaying IPv6 addresses reported by the qemu-guest-agent in the UI. The VM NIC is attached with IPv6 capability at the network layer; address configuration inside the guest is guest-OS-specific and out of scope for this HEP.
- DHCPv6 support. DHCPv6 requires server infrastructure that is not present in all IPv6 environments. IPv4 parity for dynamic addressing is satisfied by SLAAC for this HEP, since SLAAC is the infrastructure-free native mechanism. DHCPv6 can be revisited in a future HEP once SLAAC support is stable.
- Rancher server IPv6 support. Rancher must be reachable via IPv6 for Harvester to register in IPv6-only deployments. Ensuring Rancher's own IPv6 reachability is out of scope for this HEP; the dependency is noted in the design. This is tracked as follow-on work for a subsequent release.
- RKE2 guest cluster dual-stack provisioning. Provisioning RKE2 guest clusters with dual-stack networking via the Harvester node driver follows from Harvester host IPv6 support but is tracked as follow-on work for a subsequent release.
- VM management network (`mgmt`) IPv6 egress via NAT/Masquerade. VMs using the default `mgmt` network rely on NAT/Masquerade for external connectivity. Enabling IPv6 egress through this path requires changes to the `mgmt` bridge and masquerade rules that are independent from host and cluster networking scope. This is tracked as follow-on work for a subsequent release.
- Backup target IPv6: configuring NFS or S3 backup targets reachable only via IPv6 addresses. Longhorn backup and restore traffic to external NFS/S3 endpoints involves storage and data protection paths that are outside the networking scope of this HEP. This is tracked as follow-on work for a subsequent release.
- In-place IP family migration: converting a running cluster from IPv4-only to dual-stack or IPv6-only without reinstalling. The cluster-level IP family is baked into RKE2 config, etcd peer URLs, `tls-san`, and kube-vip configuration at install time. A migration path between families is not in scope for this HEP and would require a separate effort.

## IP Family Modes

Harvester management and host networking can be configured in three IP family modes. The mode is chosen at install time and is immutable for the lifetime of the cluster. Changing the IP family mode post-install is not supported: doing so would invalidate the RKE2 `node-ip`, etcd peer URLs, `tls-san`, and kube-vip VIP configuration that were committed at install time. A webhook validation rule rejects any attempt to change the cluster-level IP family after the initial installation is complete. The mode determines how nodes, the management VIP, the Kubernetes control plane, and host interfaces are addressed.

| IPv4-only | Dual-stack | IPv6-only |
|---|---|---|
| Management addresses | IPv4 | IPv4 + IPv6 | IPv6 |
| Kubernetes cluster-cidr | IPv4 prefix | IPv4 + IPv6 prefixes | IPv6 prefix |
| Kubernetes service-cidr | IPv4 prefix | IPv4 + IPv6 prefixes | IPv6 prefix |
| Pod IPs | IPv4 | IPv4 + IPv6 | IPv6 |
| Service IPs | IPv4 | IPv4 + IPv6 | IPv6 |
| Management VIP | IPv4 | IPv4 + IPv6 | IPv6 |
| VIP advertisement | ARP | ARP + NDP | NDP |
| Management URL | `https://<vip>` | `https://<vip-v4>` | `https://[<vip-v6>]` |
| Addressing mode (mgmt) | DHCP or static | Static required | Static required |
| Addressing mode (VLAN sub-interfaces) | DHCP or static | IPv4: DHCP or static; IPv6: static or SLAAC | static or SLAAC |
| Storage network (dedicated VLAN) | IPv4 CIDR | IPv4 CIDR or dual-stack CIDR | IPv6 CIDR |
| Migration network | IPv4 | IPv4 + IPv6 | IPv6 |

### Advantages and trade-offs

**IPv4-only** — Current default. Maximum compatibility with clients, tooling, and integrations. No IPv6 infrastructure required. Use where IPv4 addresses are available and IPv6 is not a requirement.

**Dual-stack** — Both address families are routable simultaneously. Clients can reach the cluster via either family. Adds configuration complexity: each management interface has two addresses, RKE2 requires dual-prefix `cluster-cidr` and `service-cidr`, kube-vip must advertise two VIPs (ARP for IPv4, NDP for IPv6), and both must appear in `tls-san`. Failure modes can occur independently per family.

**IPv6-only** — Required where IPv4 addresses are exhausted or not allocated. Eliminates NAT on the management segment. The management URL uses mandatory bracket notation (`https://[2001:db8::10]`); clients, DNS resolvers, and tooling must support IPv6 and AAAA records. Legacy IPv4-only integrations cannot reach the cluster directly. Rancher must also be reachable via IPv6 for cluster registration to work.

## Proposal

### Scope

This enhancement covers:

- Installer and host network configuration accept and validate IPv6 management settings. Management interfaces use static addressing; stable addresses are required for cluster control-plane stability. The installer sets `net.ipv6.conf.all.disable_ipv6=0` in a persistent sysctl drop-in before activating any IPv6 NetworkManager profile; the network controller verifies this setting on running nodes before reconciling IPv6 config.
- IPv6 DNS nameserver configuration at install time: the installer accepts nameserver addresses for IPv6-only and dual-stack management interface setup and writes them into the generated NetworkManager profile. Environments where the upstream router sends RDNSS options in RAs are documented explicitly as an alternative.
- VM network route settings allow IPv6 CIDR and gateway where applicable.
- Host network config APIs and controllers support IPv6 addresses for L3 interface configuration, covering both static and SLAAC modes.
- SLAAC is the dynamic addressing mode for IPv6, equivalent in role to DHCPv4 for IPv4: it allows nodes to self-configure addresses from Router Advertisement prefixes without requiring manual per-node assignment or server infrastructure.
- Validation and status conditions report IPv6-specific errors clearly.
- Dual-stack support, meaning IPv4 and IPv6 enabled together.
- RKE2 control-plane configuration (node-ip, cluster-cidr, service-cidr, tls-san, stack-preference) is generated by the installer for the selected IP family.
- kube-vip VIP advertisement is configured for the selected IP family: ARP for IPv4, NDP for IPv6, or both for dual-stack. The installer config schema accepts an IPv6 VIP.
- kube-ovn overlay IPv6: VM network traffic through the kube-ovn data plane supports IPv6 CIDR and gateway settings.
- Ingress controller and console proxy IPv6: the Ingress controller is configured to bind on IPv6 so the management UI and API are reachable at `https://[<vip-v6>]`; the VM VNC/Serial console proxy accepts IPv6 connections from the management VIP.
- IPv6 addressing on the VM live migration network: the migration network is a `HostNetworkConfig`-managed VLAN and follows directly from host network config IPv6 support; no separate KubeVirt or Multus changes are required.
- IPv6 addressing on the dedicated Longhorn storage network: the Harvester `storage-network` setting schema is extended to accept an IPv6 or dual-stack CIDR, and the Whereabouts IP pool and generated `NetworkAttachmentDefinition` are updated accordingly.

#### Network Types Extended by This HEP

The table below maps each Harvester network type to its current IPv4 state, the IPv6 extension work this HEP introduces, and the responsible component. Networks marked _deferred_ are explicitly out of scope; see Non-goals.

| Network type | Current (IPv4) | IPv6 extension in this HEP | Responsible component |
|---|---|---|---|
| Management network (node-ip / VIP) | Static or DHCP; kube-vip ARP VIP | Static only; kube-vip NDP VIP; installer writes `node-ip`, `tls-san`, etcd peers with IPv6 values | `harvester-installer` |
| Cluster / Pod networking (RKE2, Canal) | IPv4 `cluster-cidr` / `service-cidr` | Dual-stack or IPv6-only `cluster-cidr` / `service-cidr`; Canal auto-configures from RKE2 values | `harvester-installer` |
| VM networking — VLAN (L2) | L2 bridge; VM route config IPv4 | VM route `cidrV6` / `gatewayV6` fields; L2 bridge itself unchanged | `network-controller-harvester` |
| VM networking — kube-ovn overlay | Subnet `cidrBlock` / `gateway` IPv4 | Subnet `cidrBlock` / `gateway` accept IPv6 or dual-stack values (config change, no kube-ovn code change) | `kube-ovn` (config), `network-controller-harvester` (route controller) |
| VM live migration network | IPv4 `HostNetworkConfig`-managed VLAN | IPv6 static or SLAAC on same VLAN; KubeVirt and Multus already support IPv6 | `network-controller-harvester` |
| Dedicated Longhorn storage network | IPv4 CIDR; Whereabouts pool; NAD | `storage-network` setting accepts IPv6 or dual-stack CIDR; Whereabouts pool and NAD updated | `harvester` |
| Guest clusters (RKE2 node driver) | IPv4 only | _Deferred — follow-on HEP_ | — |
| HostNetworkConfig (host VLAN L3 interfaces) | `mode: dhcp\|static`; `ips` map (IPv4) | Adds `family`, `assignments` (optional), and `ipv6CIDR` per node alongside existing fields; `slaac` added as new valid `mode` value; Privacy Extensions disabled | `network-controller-harvester` |

### User Stories

The stories below use a three-node cluster unless specified otherwise.

Initial management network example:
- Node 1: 2001:db8:100:10::11/64
- Node 2: 2001:db8:100:10::12/64
- Node 3: 2001:db8:100:10::13/64
- Default gateway: 2001:db8:100:10::1

#### Story 1: Install Harvester on IPv6-only management network

TODO: detail

#### Story 2: Install Harvester on dual-stack management network

TODO: detail

#### Story 3: Configure IPv6 route settings on VM network

TODO: detail

#### Story 4: Configure IPv6 on host vlan sub-interface (static)

TODO: detail

#### Story 5: Configure IPv6 on host vlan sub-interface (SLAAC)

TODO: detail

#### Story 6: Node lifecycle behavior

TODO: detail

#### Story 7: Upgrade behavior

TODO: detail

#### Story 8: Access web UI and VM console in IPv6-only environment

TODO: detail

#### Story 9: Configure IPv6 on migration network

TODO: detail

#### Story 10: Configure IPv6 on dedicated storage network

TODO: detail

### User Experience In Detail

#### Installation

- Existing install flow is extended with IPv6 fields where management settings are provided. Management interfaces always use static addressing. The reason is specific to installer sequencing: the installer must record the management address before first boot to write `node-ip` into the RKE2 config, derive etcd peer URLs, populate `tls-san` for the API server certificate, and configure the kube-vip VIP — all of which require a known, stable address at install time. SLAAC cannot satisfy this because the address is only assigned after the interface is up and a Router Advertisement is received, which happens after the installer has already committed the node config. This is installer and host-networking work, not only Kubernetes API work; see [install.management_interface](https://docs.harvesterhci.io/v1.8/install/harvester-configuration#installmanagement_interface) for the current management interface configuration schema.
- Validation includes IPv6 address/prefix format, gateway format, and family consistency.
- For dual-stack, at least one reachable route is required for cluster bootstrap.
- IPv6-only and dual-stack installs require at least one DNS nameserver to be supplied in the install config unless the upstream router sends RDNSS options in Router Advertisements (RFC 6106). Without a valid nameserver source, DNS resolution fails on first boot and may impact etcd peer discovery. The installer emits a warning if no DNS source is provided but does not block installation; RDNSS-capable routers satisfy this requirement automatically.

#### Cluster Network and Host Network Configuration

- HostNetworkConfig is extended to represent IPv6 L3 addresses in both static and SLAAC modes. For the practical meaning of cluster network, bridge, bond, VLAN, and host-side networking, see [Cluster Network](https://docs.harvesterhci.io/v1.8/networking/index) and the [Harvester Network Deep Dive](https://docs.harvesterhci.io/v1.8/networking/deep-dive).
- Users can choose static or SLAAC for host-side L3 interfaces on VLAN sub-interfaces — these are routed host interfaces, not VM bridge ports. VM network bridges are L2-only; IPv6 addressing (static or SLAAC) applies only when the operator creates an additional L3 host interface on a VLAN via `HostNetworkConfig`, not to the bridge interfaces used for VM traffic attachment.
- For SLAAC, the default gateway is not a config field — it is populated by the upstream router via Router Advertisement. The controller does not need to set a gateway explicitly.
- Status conditions show per-node apply state and failure reason.

#### VM Network Configuration

- VM network route settings support IPv6 CIDR and gateway. This is largely extending current IPv4-oriented route and validation behavior rather than inventing a new network model; see [VM Network](https://docs.harvesterhci.io/v1.8/networking/harvester-network).
- Connectivity checks include IPv6 path validation from all applicable nodes.

#### Management Access and Console

- In IPv6-only and dual-stack deployments, the Harvester web UI and API are reachable at `https://[<vip-v6>]`. Clients must support IPv6 and the browser must accept bracket notation in the address bar, which all modern browsers do.
- The VM VNC and Serial console are opened via a WebSocket connection from the browser. In an IPv6-only environment the WebSocket URL uses `wss://[<vip-v6>]/...`. The UI front end constructs this URL using bracket notation when the management VIP is IPv6; the console proxy backend accepts connections on both IPv4 and IPv6 sockets.
- The TLS certificate presented at the management VIP must include the IPv6 VIP in its SAN. This is ensured by the `tls-san` entry added to the RKE2 config by the installer.

#### Operational Workflows

- Reboot, maintenance mode, and node replacement preserve and reconcile IPv6 configuration.
- Documentation provides equivalent commands and examples for IPv4-only, IPv6-only, and dual-stack setups.

### API changes

All changes are additive at `network.harvesterhci.io/v1beta1`. No fields are removed. Existing `HostNetworkConfig` resources that use only `mode` and `ips` continue to work unchanged — the new fields are all optional. `slaac` is added as a new valid value for the existing `mode` field. The new `family` and `assignments` fields are optional and coexist with the existing `ips` map; during the transition period both are accepted, with `assignments` taking precedence when present. `VMNetworkRouteSpec` gains two optional fields (`cidrV6`, `gatewayV6`) alongside the existing IPv4 fields. No schema version bump is required.

```go
type IPFamily string

const (
    IPFamilyIPv4 IPFamily = "IPv4"
    IPFamilyIPv6 IPFamily = "IPv6"
    IPFamilyDual IPFamily = "DualStack"
)

// IPAssignmentMode extends the existing mode field.
// Existing values "dhcp" and "static" are unchanged.
// "slaac" is new for IPv6 dynamic addressing.
type IPAssignmentMode string

const (
    IPAssignmentModeStatic IPAssignmentMode = "static"
    IPAssignmentModeDHCPv4 IPAssignmentMode = "dhcpv4"
    IPAssignmentModeSLAAC  IPAssignmentMode = "slaac"
    // DHCPv6 is not in scope for this HEP.
)

// HostIPAssignment is the per-node entry in the new assignments slice.
// It coexists with the existing ips map — assignments takes precedence
// for listed nodes. IPv4-only nodes may continue to use the ips map.
type HostIPAssignment struct {
    NodeName string           `json:"nodeName"`
    IPv4CIDR string           `json:"ipv4CIDR,omitempty"`
    IPv6CIDR string           `json:"ipv6CIDR,omitempty"`
    Mode     IPAssignmentMode `json:"mode"`
}

// HostNetworkConfigSpec — additive delta only.
// Existing fields clusterNetwork, vlanID, nodeSelector, mode, ips are unchanged.
// New optional fields: family, assignments.
// When assignments is present it takes precedence over ips for the listed nodes.
type HostNetworkConfigSpec struct {
    // --- existing fields (unchanged) ---
    ClusterNetwork string                `json:"clusterNetwork"`
    VlanID         uint16                `json:"vlanID"`
    NodeSelector   *metav1.LabelSelector `json:"nodeSelector,omitempty"`
    Mode           IPAssignmentMode      `json:"mode,omitempty"`         // existing
    IPs            map[string]string     `json:"ips,omitempty"`          // existing IPv4 map
    // --- new optional fields ---
    Family         IPFamily              `json:"family,omitempty"`
    Assignments    []HostIPAssignment    `json:"assignments,omitempty"`
}

// VMNetworkRouteSpec — additive delta only.
// Existing cidrV4 and gatewayV4 fields are unchanged.
// New optional fields: cidrV6, gatewayV6.
type VMNetworkRouteSpec struct {
    CIDRv4   string `json:"cidrV4,omitempty"`   // existing
    Gateway4 string `json:"gatewayV4,omitempty"` // existing
    CIDRv6   string `json:"cidrV6,omitempty"`   // new
    Gateway6 string `json:"gatewayV6,omitempty"` // new
}
```

For SLAAC mode, the `assignments` list is optional. When mode is `slaac`, no per-node `ipv6CIDR` is needed because the address comes from the upstream Router Advertisement. The `nodeName` field may still be used to scope SLAAC mode to specific nodes.

Example HostNetworkConfig (static IPv6):

```yaml
apiVersion: network.harvesterhci.io/v1beta1
kind: HostNetworkConfig
metadata:
  name: storage-v6-2012
spec:
  clusterNetwork: br-storage
  vlanID: 2012
  family: IPv6
  assignments:
    - nodeName: node-1
      mode: static
      ipv6CIDR: 2001:db8:2012::11/64
    - nodeName: node-2
      mode: static
      ipv6CIDR: 2001:db8:2012::12/64
    - nodeName: node-3
      mode: static
      ipv6CIDR: 2001:db8:2012::13/64
```

Example HostNetworkConfig (SLAAC):

```yaml
apiVersion: network.harvesterhci.io/v1beta1
kind: HostNetworkConfig
metadata:
  name: storage-v6-slaac-2012
spec:
  clusterNetwork: br-storage
  vlanID: 2012
  family: IPv6
  assignments:
    - nodeName: node-1
      mode: slaac
    - nodeName: node-2
      mode: slaac
    - nodeName: node-3
      mode: slaac
```

Example VM Network route (dual-stack):

```yaml
route:
  cidrV4: 192.168.100.0/24
  gatewayV4: 192.168.100.1
  cidrV6: 2001:db8:4000::/64
  gatewayV6: 2001:db8:4000::1
```

Example installer config additions for IPv6 VIP (dual-stack):

```yaml
install:
  vip: 192.168.1.10       # existing IPv4 VIP
  vipMode: static         # existing mode
  vipV6: 2001:db8::10     # new: IPv6 VIP for dual-stack or IPv6-only
```

Example RKE2 config generated by the installer for dual-stack (`90-harvester-server.yaml`):

```yaml
cluster-cidr: 10.52.0.0/16,fd00:10:52::/56
service-cidr: 10.53.0.0/16,fd00:10:53::/108
node-ip: 192.168.1.11,2001:db8:100:10::11
tls-san:
  - 192.168.1.10     # IPv4 VIP
  - 2001:db8::10     # IPv6 VIP
stack-preference: dual  # controls loopback address for internal health probes (ipv4/ipv6/dual)
```

## Design

### Implementation Overview

#### Architecture Principles

- Keep existing IPv4 behavior unchanged.
- Add IPv6 in an additive manner with clear validation and status reporting.
- Maintain compatibility with Harvester v1.7+ host networking stack (NetworkManager).

#### Component-by-Component Work

1. harvester-installer
- Extend install management interface parsing and validation for IPv6 and dual-stack (static only for management interfaces).
- Generate NetworkManager connection profiles with IPv6 settings for new installs.
- Ensure generated profiles are persisted and re-generated consistently.
- Accept an IPv6 VIP in the install config (`vipV6` field) for IPv6-only and dual-stack modes.
- Generate RKE2 config (`90-harvester-server.yaml`) with appropriate node-ip, cluster-cidr, service-cidr (dual-prefix for dual-stack), tls-san for all configured VIPs, and stack-preference (ipv4/ipv6/dual) to control the loopback address used for internal health probes.
- Configure kube-vip with ARP (IPv4), NDP (IPv6), or both (dual-stack) based on the selected IP family.
- For IPv6-only: ensure `/etc/hosts` contains `::1 localhost` on each node, as required by RKE2 etcd liveness checks.
- For IPv6 and dual-stack: write `net.ipv6.conf.all.disable_ipv6=0` to `/etc/sysctl.d/99-harvester-ipv6.conf` and apply with `sysctl --system` before any NetworkManager IPv6 profile is activated. If IPv6 is still disabled at the kernel level when the NM profile is applied, address assignment silently fails with no error from NetworkManager.
- For IPv6-only and dual-stack: if DNS nameserver addresses are provided in the install config, write them as `ipv6.dns` entries in the generated NetworkManager connection profile for the management interface. If no nameservers are supplied, omit `ipv6.dns` and document RDNSS from RA as the expected DNS source; NetworkManager processes RDNSS options automatically when received.
- For all NetworkManager connection profiles generated at install time that carry IPv6 (both static management and any sub-interface profiles), set `ipv6.ip6-privacy=0` to prevent Privacy Extension temporary addresses (RFC 8981) from being generated alongside the configured address. For SLAAC-mode profiles, additionally set `ipv6.addr-gen-mode=eui64` to ensure the RA-derived address is MAC-derived and stable across reboots. SLE Micro enables Privacy Extensions by default; omitting these settings causes SLAAC address rotation approximately every 24 hours.

2. harvester
- Extend API validation/webhooks for IPv6 fields in relevant resources.
- Extend controllers and status reporting to handle IPv6 conditions.
- Ensure upgrade workflows preserve and validate IPv6 settings.
- Configure the Ingress controller (nginx) to listen on IPv6 addresses so the web UI, API, and VNC/Serial console proxy are reachable in IPv6-only and dual-stack deployments. Ensure the console proxy service binds on IPv6. Front-end URL construction for bracket notation is a UI concern and is covered in component 4.
- Extend the `storage-network` setting schema to accept IPv6 or dual-stack CIDR in addition to IPv4 CIDR. Update the Whereabouts IP pool configuration and the generated `NetworkAttachmentDefinition` to carry the correct address family. This is the only Harvester-side change required; Longhorn v1.11.1 imposes no address-family restriction on the storage network NAD it receives.

3. network-controller-harvester
- Extend host network config reconciliation for static and SLAAC modes.
- For SLAAC, configure NetworkManager profiles with `ipv6.method=auto`, `ipv6.addr-gen-mode=eui64`, and `ipv6.ip6-privacy=0`. Do not set a gateway field; the gateway is derived from Router Advertisements. The `addr-gen-mode=eui64` setting ensures the RA-derived address is stable and MAC-derived — without it, SLE Micro's default Privacy Extensions behavior would cause the address to rotate approximately every 24 hours, silently breaking Whereabouts IP allocations, KubeVirt migration bindings, and Longhorn storage network connectivity.
- For static mode, set `ipv6.ip6-privacy=0` on generated NetworkManager profiles to prevent Privacy Extension temporary addresses from appearing alongside the statically configured address.
- Before applying IPv6 config to a running node, verify `net.ipv6.conf.all.disable_ipv6=0`; if not set, emit a `KernelIPv6Disabled` node condition and block reconciliation. The operator must write the sysctl to `/etc/sysctl.d/99-harvester-ipv6.conf`, apply it with `sysctl --system`, and then either reload the affected NetworkManager connection (`nmcli connection up <name>`) or reboot the node. A reboot is the safest remediation if the interface was initialized with IPv6 disabled.
- Add IPv6 route and reachability checks for VM network route configuration.
- Surface per-node condition details for IPv6 failures.

4. harvester-ui-extension
- Extend forms and validation for IPv6 and dual-stack inputs.
- Expose static and SLAAC as explicit mode choices; do not offer DHCPv6.
- Show family-aware warnings and connectivity status in UI.
- Fix console and VNC URL construction to emit bracket notation (`wss://[<vip-v6>]/...`) when the management VIP is an IPv6 address. Without this, the browser rejects the URL before a connection is attempted.

5. docs
- Add user and operator guides for IPv6-only and dual-stack deployment.
- Document that SLAAC requires a router sending Router Advertisements on the segment and that the gateway is not a user-configured field.
- Add troubleshooting guidance for common IPv6 issues, including RA not received.
- Document how to access the web UI and VM console in IPv6-only and dual-stack deployments, including the required browser URL format (`https://[<vip-v6>]`), TLS certificate SAN requirements, and expected console WebSocket URL form.

6. kube-ovn
- The kube-ovn overlay transport (VXLAN/Geneve) is address-family agnostic and requires no code changes for IPv6. The required work is configuration only: update kube-ovn subnet `spec.cidrBlock` to accept an IPv6 or dual-stack CIDR value, and `spec.gateway` to accept an IPv6 or comma-separated dual-stack gateway. These fields already support IPv6 in kube-ovn v1.15.4 (see upstream evidence table).
- Align the Harvester VM network route controller to pass the configured `cidrV6`/`gatewayV6` values from `VMNetworkRouteSpec` into the corresponding kube-ovn subnet fields.

7. charts and addons where applicable
- Validate default values and schema do not block IPv6 deployment.
- Document required settings for integrated addons when dual-stack is enabled.

**Note: Canal (Calico + Flannel) IPv6 overlay.** Canal automatically detects dual-stack when `cluster-cidr` and `service-cidr` contain both IPv4 and IPv6 prefixes — no separate HelmChartConfig is needed (see [RKE2 docs: dual-stack configuration](https://docs.rke2.io/networking/basic_network_options#dual-stack-configuration)). Flannel VXLAN dual-stack support has been in upstream Flannel since [flannel-io/flannel#1448](https://github.com/flannel-io/flannel/pull/1448) (Jul 2021) and is present in the hardened Flannel builds Harvester ships (`rancher/hardened-flannel:v0.28.1+`). Flannel also supports IPv6-only mode via `EnableIPv4: false`. The only required work is installer generation of the correct dual-stack or IPv6-only `cluster-cidr` and `service-cidr` values in `90-harvester-server.yaml`.

### Validation Rules

- CIDR and gateway must match selected family.
- When IPv6 or dual-stack family is configured, `net.ipv6.conf.all.disable_ipv6` must be `0` on the target node. The installer enforces this before activating the interface profile; the network controller emits a `KernelIPv6Disabled` condition if the setting is absent on a running node.
- For static mode, per-node assignment with `ipv6CIDR` is required for each targeted node.
- For SLAAC mode, `ipv6CIDR` must not be set; the address is derived from the RA prefix.
- Family mismatches (for example IPv6 gateway with IPv4 CIDR) are rejected.
- Duplicate IP assignment across nodes in the same HostNetworkConfig is rejected.
- SLAAC and static mode must not be mixed within the same HostNetworkConfig.
- The `vipV6` installer field must be a valid IPv6 address (not a CIDR prefix) when present. An address with a prefix length is rejected at install validation time.
- The cluster-level IP family mode is immutable post-install. Any attempt to change the IP family of an existing cluster is rejected by the admission webhook.
- The `storage-network` setting value must be a valid IPv4 CIDR, IPv6 CIDR, or comma-separated dual-stack CIDR pair. A bare address without prefix length is rejected. The CIDR must match the address family configured in the referenced `NetworkAttachmentDefinition`.
- For IPv6-only installs, the installer emits a warning if no DNS nameserver is supplied in the install config and RDNSS is not documented as available. The install is not blocked, but the operator must ensure either a nameserver is provided or the upstream router sends RDNSS options in Router Advertisements.
- All IPv6 NetworkManager connection profiles generated or reconciled by Harvester must include `ipv6.ip6-privacy=0`. SLAAC profiles must additionally include `ipv6.addr-gen-mode=eui64`. The network controller verifies these settings are present when reconciling an existing profile on a running node; if absent, it emits an `IPv6PrivacyExtensionsEnabled` condition and rewrites the profile.

### Failure Modes and Reporting

- Node-level condition for each apply step: vlan create, link up, address apply, route checks.
- If `net.ipv6.conf.all.disable_ipv6=1` on a node when IPv6 config is applied, the network controller emits a `KernelIPv6Disabled` condition with remediation: write `net.ipv6.conf.all.disable_ipv6=0` to `/etc/sysctl.d/99-harvester-ipv6.conf`, run `sysctl --system`, then run `nmcli connection up <name>` for the affected interface. Reboot the node if the interface was already initialized with IPv6 disabled.
- For SLAAC, an additional condition surfaces when no RA-derived address is observed within a timeout window (indicating the upstream router is not sending RAs on the segment).
- If a SLAAC NetworkManager profile is applied without `ipv6.addr-gen-mode=eui64` and `ipv6.ip6-privacy=0`, the kernel may generate Privacy Extension temporary addresses and the primary SLAAC address will rotate after the temporary address lifetime expires (typically 24 hours on SLE Micro). This causes the node to bind new addresses, silently breaking components that registered the original address — including Whereabouts IP pool entries, KubeVirt migration server bindings, and Longhorn storage network NAD annotations — without any Kubernetes condition being raised. Diagnosis: run `ip -6 addr show <interface>` and look for addresses flagged `temporary`. Remediation: ensure `ipv6.addr-gen-mode=eui64` and `ipv6.ip6-privacy=0` are present in the NM profile, run `nmcli connection up <name>`, and verify only the stable EUI-64-derived address remains. This failure mode must be caught by Host Network Tests and documented in the troubleshooting guide.
- In IPv6-only deployments, if no DNS nameserver was configured at install time and the upstream router does not send RDNSS options in RAs, `/etc/resolv.conf` will have no nameserver entries after boot. This is diagnosable via `resolvectl status` on the affected node. The installer emits a warning at install time; post-install diagnosis and remediation steps are documented in the troubleshooting guide.
- Explicit reason codes for invalid config, interface unavailable, and external dependency failures.
- Reconciliation retries are bounded and observable.
- If the Ingress controller or console proxy is not correctly bound to IPv6, clients in IPv6-only environments receive a connection refusal at the socket level rather than an HTTP error. This failure mode is not surfaced as a Kubernetes condition; it must be caught by the E2E acceptance tests and documented in the troubleshooting guide.

### Test plan

#### Environment Matrix

- Single-node and 3-node clusters.
- IPv4-only baseline.
- IPv6-only.
- Dual-stack.

#### Installation Tests

1. Install with IPv6-only management interface (static), verify cluster bootstrap.
2. Install with dual-stack management interface, verify node registration and reboot stability.
3. After IPv6-only install, verify the web UI is reachable at `https://[<vip-v6>]` from a client that has no IPv4 route to the cluster.
4. Negative tests for malformed IPv6 addresses, invalid prefixes, invalid gateways, and `vipV6` supplied as a CIDR rather than a plain address.
5. Verify `/etc/sysctl.d/99-harvester-ipv6.conf` contains `net.ipv6.conf.all.disable_ipv6=0` after IPv6-mode installation; verify `sysctl net.ipv6.conf.all.disable_ipv6` returns `0` on each node.
6. Install with IPv6-only management interface and an explicit DNS nameserver; verify the nameserver appears in `resolvectl status` output on each node after first boot.
7. Install with IPv6-only management interface and no DNS nameserver supplied; verify the installer emits a warning and that no `ipv6.dns` entry is written to the NM profile (relying on RDNSS from RA).

#### Host Network Tests

1. Create HostNetworkConfig with static IPv6 per node.
2. Create HostNetworkConfig with SLAAC mode; verify RA-derived address appears in status.
3. Verify SLAAC gateway is populated from RA and not from a user config field.
4. Verify the SLAAC-assigned address uses EUI-64 derivation: confirm it matches the expected format for the node's MAC address and that no address with the `temporary` flag appears on the interface (`ip -6 addr show <vlan-interface>`).
5. Verify `ipv6.addr-gen-mode=eui64` and `ipv6.ip6-privacy=0` are present in the generated NetworkManager profile for SLAAC connections; verify `ipv6.ip6-privacy=0` is present in static connection profiles.
6. Reboot nodes and verify persistence of static and SLAAC configurations.
7. Add and remove nodes; verify reconciliation and status reporting.
8. Negative test: SLAAC mode with no RA on segment surfaces clear condition/error.

#### VM Network Tests

1. Create VM network route with IPv6 CIDR/gateway.
2. Verify connectivity checks and status updates.
3. Validate dual-stack route settings.
4. Validate webhook rejection of mismatched family input.
5. L2 VM-to-VM IPv6 ping on the same node over a VLAN network: attach two VMs to the same VLAN and verify ICMPv6 echo reply.
6. L2 VM-to-VM IPv6 ping across nodes over a VLAN network: schedule two VMs on different nodes and verify ICMPv6 echo reply, confirming the overlay underlay carries IPv6 frames correctly.

#### kube-ovn Overlay Tests

1. Create a VM network backed by a kube-ovn subnet with an IPv6 `spec.cidrBlock` and `spec.gateway`; verify the subnet reaches `Ready` state and the provisioned `cidrBlock` matches the configured IPv6 value.
2. Attach two VMs to the kube-ovn IPv6 subnet; verify each VM receives an IPv6 address from the subnet range, visible via VM network status.
3. ICMPv6 echo between two VMs on the same kube-ovn subnet scheduled on the same node; verify echo reply received.
4. ICMPv6 echo between two VMs on the same kube-ovn subnet scheduled on different nodes; verify echo reply received, confirming VXLAN/Geneve encapsulation carries IPv6 traffic correctly.
5. Dual-stack kube-ovn subnet: configure `spec.cidrBlock` with both IPv4 and IPv6 prefixes; verify both families are provisioned and VMs receive addresses from each family.

#### Migration Network Tests

1. Create `HostNetworkConfig` with static IPv6 for a VLAN designated as the migration network; verify address appears on each node.
2. Configure KubeVirt to use the migration network NAD; trigger a live migration and verify it completes over the IPv6 interface.
3. Verify migration network IPv6 configuration persists across node reboots.

#### Storage Network Tests

1. Configure the Harvester `storage-network` setting with an IPv6 CIDR; verify the generated NAD carries the correct CIDR and Whereabouts IP pool is updated.
2. Configure the `storage-network` setting with a dual-stack CIDR pair; verify both address families appear in the generated NAD.
3. Verify Longhorn instance-manager and backing-image-manager pods receive annotations with the storage network NAD and bind on the assigned IPv6 addresses.
4. Verify Longhorn replication traffic flows over the IPv6 storage network interface rather than the pod network.
5. Negative test: supplying a bare IPv6 address (no prefix length) to the `storage-network` setting is rejected by the webhook.

#### Upgrade and Regression Tests

1. Upgrade IPv4-only clusters and verify no regression.
2. Upgrade clusters with IPv6 configuration and verify persistence.
3. Validate fallback and recovery guidance for invalid post-upgrade custom networking.

#### E2E Acceptance

- VM to external IPv6 connectivity over VLAN network.
- VM scheduling behavior remains correct with cluster network selectors.
- VM live migration succeeds between nodes in IPv6-only and dual-stack clusters: migrate a running VM across nodes over an IPv6-addressed migration network and verify the VM is reachable at its IPv6 address after migration completes.
- Web UI and API accessible at the IPv6 management VIP (`https://[<vip-v6>]`) in IPv6-only and dual-stack deployments.
- VM VNC and Serial console accessible via the IPv6 management VIP in IPv6-only and dual-stack deployments.
- Longhorn volume replication traffic flows over the IPv6-addressed dedicated storage network; verify replication completes between nodes and volume data is consistent.

### Upgrade strategy

- New installs: generate and apply IPv6-capable NetworkManager profiles from install config.
- Upgrades: preserve existing behavior and add IPv6 fields in backward-compatible API versions.
- Pre-upgrade checks identify invalid or incomplete IPv6 configuration and block unsafe rollout.
- Provide a clear rollback path to last known-good network configuration if post-upgrade reconciliation fails.

For upgrades from older versions where wicked artifacts still exist, wicked data is treated only as historical input during migration. Runtime host networking remains NetworkManager-based in supported v1.7+ upgrades.

### References

#### Related Issues

- https://github.com/harvester/harvester/issues/934
- https://github.com/harvester/harvester/issues/2962
- https://github.com/k3s-io/k3s/issues/284
- https://github.com/flannel-io/flannel/pull/1398
- https://github.com/flannel-io/flannel/pull/1448

#### Upstream Component IPv6 Support

Each component integrated in this HEP has documented first-class IPv6 support in isolation. The table below links the upstream evidence so reviewers can verify that the claimed IPv6 capability exists in each project before Harvester wires them together.

| Component | Role in Harvester | IPv6 capability confirmed | Reference |
|---|---|---|---|
| **kube-vip** v1.0.4 | Management VIP advertisement (ARP/NDP) | `--dnsMode (first,ipv4,ipv6,dual)` and `--dhcpMode (ipv4,ipv6,dual)` flags select the address family; `VIP_IFACE` and `address` fields in the kube-vip ConfigMap accept IPv6; v1.0.4 fixes endpointslice handling in dual-stack clusters | https://kube-vip.io/docs/about/features/ |
| **kube-ovn** v1.15.4 | VM network overlay (SDN) | Subnet `spec.cidrBlock` accepts `IPv4CIDR,IPv6CIDR` pair; `spec.gateway` accepts comma-separated dual-stack gateways; KubeVirt VMs inherit both addresses | https://kubeovn.github.io/docs/v1.15.x/en/guide/dual-stack/ · https://kubeovn.github.io/docs/v1.15.x/en/kubevirt/dual-stack/ |
| **Canal** (Calico v3.31.3 / Flannel v0.28.1) | Pod and cluster network CNI | Canal reads `cluster-cidr` and `service-cidr` from RKE2 config and auto-configures dual-stack IPAM — no separate HelmChartConfig needed; Calico `ipPools` supports IPv4-only, dual-stack, and IPv6-only; Flannel VXLAN dual-stack pre-dates v0.28.1 | https://docs.tigera.io/calico/3.31/networking/ipam/ipv6 · https://docs.rke2.io/networking/basic_network_options#dual-stack-configuration |
| **RKE2** v1.35.2 | Kubernetes distribution | `cluster-cidr`, `service-cidr` accept comma-separated dual-stack prefixes; `node-ip` accepts comma-separated IPv4,IPv6 pair; `tls-san` lists both VIPs; `stack-preference` (ipv4/ipv6/dual) controls health-probe loopback; harvester-installer generates these into `90-harvester-server.yaml` | https://docs.rke2.io/networking/basic_network_options#dual-stack-configuration · https://ranchermanager.docs.rancher.com/reference-guides/cluster-configuration/rancher-server-configuration/rke2-cluster-configuration#networking |
| **nginx Ingress Controller** v1.14.3 | Web UI, API, and console proxy exposure | Service `spec.ipFamilyPolicy: PreferDualStack` assigns both cluster IPs; nginx binds `[::]` natively; `tls-san` must include the IPv6 VIP; **Note**: ingress-nginx archived March 2026 — Harvester 1.8 ships v1.14.3; RKE2 v1.36+ defaults to Traefik | https://kubernetes.io/docs/concepts/services-networking/dual-stack/ |
| **NetworkManager** 1.52.0 | Host interface IPv6 lifecycle | `ipv6.method=auto` (SLAAC — address derived from RA prefix, no gateway field required; must also set `ipv6.addr-gen-mode=eui64` to force stable MAC-based address and `ipv6.ip6-privacy=0` to disable Privacy Extension rotation) or `ipv6.method=manual` (static, requires `ipv6.addresses` and `ipv6.gateway`; must also set `ipv6.ip6-privacy=0` to suppress temporary addresses); `ipv6.routes` for additional prefixes; network-controller-harvester writes these fields via `nmcli`/keyfile | https://networkmanager.dev/docs/api/latest/settings-ipv6.html |
| **KubeVirt** v1.7.0 | VM networking | VM `spec.template.spec.domain.devices.interfaces[].masquerade` binding; user guide covers both IPv4+IPv6 dual-stack masquerade and IPv6 single-stack masquerade; no code change required in KubeVirt itself | https://kubevirt.io/user-guide/network/interfaces_and_networks/#masquerade-ipv4-and-ipv6-dual-stack-support |
| **Multus CNI** v4.2.3 | Secondary network attachment (VLAN NICs) | `NetworkAttachmentDefinition` `spec.config` carries the delegate CNI JSON (bridge, VLAN, host-device); Multus passes it to the delegate unmodified — IPv6 support is in the delegate config, not Multus itself | https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/quickstart.md |
| **Longhorn** v1.11.1 | Persistent block storage and volume replication (storage network via Multus NAD) | `storage-network` setting accepts any Multus NAD; address family is determined by the NAD delegate config — no IPv4-only restriction exists in Longhorn itself; CI pipeline runs backup tests with `NETWORK_STACK=ipv6`; Harvester-side change required: extend `storage-network` setting schema and Whereabouts IP pool to accept IPv6 or dual-stack CIDR | https://longhorn.io/docs/1.11.0/advanced-resources/deploy/storage-network/ |

## Note

This HEP defines a technical, package-by-package implementation scope so the resulting work can be split into sequenced PRs across harvester-installer, harvester, network-controller-harvester, harvester-ui-extension, and docs, while preserving existing IPv4 behavior and upgrade safety.

DHCPv6 is not included in this HEP. SLAAC covers the dynamic addressing need for IPv6 without requiring server infrastructure and is the right scope boundary for this work.