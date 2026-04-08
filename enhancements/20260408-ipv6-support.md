# IPv6 Support

This enhancement introduces first-class IPv6 support in Harvester host and VM networking for IPv6-only and dual-stack environments.

The file name is lowercase and uses dashes, following HEP conventions.

## Summary

Harvester users have requested IPv6 support for multiple years, especially for environments where IPv4 addresses are limited or unavailable. The current networking flows are primarily IPv4-oriented for host management and VM network route configuration, which prevents installation and operations in IPv6-only environments and creates friction for modern dual-stack deployments.

This proposal adds IPv6 support across installation, host network lifecycle, VM network configuration, and upgrade behavior, while preserving current IPv4 behavior. The scope covers static addressing, SLAAC-based dynamic addressing, IPv6 route settings, and validation. DHCPv6 is out of scope; see Non-goals.

The proposal aligns with Harvester v1.7+ networking architecture, which uses NetworkManager on hosts. Wicked is treated only as a legacy migration concern for upgrades from older releases.

### Related Issues

- https://github.com/harvester/harvester/issues/934
- https://github.com/harvester/harvester/issues/2962
- https://github.com/k3s-io/k3s/issues/284
- https://github.com/flannel-io/flannel/pull/1398

## Terminology

- IPv6-only: Environment where host and cluster networking use only IPv6 addresses.
- Dual-stack: Environment where IPv4 and IPv6 are both enabled and routable.
- SLAAC: Stateless Address Autoconfiguration for IPv6. The host derives its address from a Router Advertisement prefix — no server is required. This is the native IPv6 dynamic addressing mechanism, analogous to DHCPv4 for IPv4 in environments that do not use manually assigned addresses.
- RA: Router Advertisement for IPv6 route and prefix discovery. Required for SLAAC and default gateway assignment.
- DHCPv6: A server-based dynamic address assignment mechanism for IPv6. Not in scope for this HEP; see Non-goals.

## Motivation

### Goals

- Support Harvester installation on IPv6-only and dual-stack management networks.
- Support IPv6 route configuration and validation for VM networks where route settings are currently IPv4-specific.
- Support IPv6-aware host network configuration on management and custom cluster networks.
- Support SLAAC mode for host VLAN sub-interfaces as the IPv6-native dynamic addressing mechanism.
- Preserve backward compatibility for IPv4-only clusters.
- Provide explicit upgrade and rollback guidance for IPv6-related configuration changes.

### Non-goals

- Replacing current CNI architecture in this HEP.
- Implementing NAT64, DNS64, or other protocol translation services.
- Automatic conversion of all existing IPv4-only user network definitions to IPv6.
- Guest OS-specific network configuration inside the VM after NIC attachment.
- DHCPv6 support. DHCPv6 requires server infrastructure that is not present in all IPv6 environments. IPv4 parity for dynamic addressing is satisfied by SLAAC for this HEP, since SLAAC is the infrastructure-free native mechanism. DHCPv6 can be revisited in a future HEP once SLAAC support is stable.

## Proposal

### Scope

This enhancement covers:

- Installer and host network configuration accept and validate IPv6 management settings. Management interfaces use static addressing; stable addresses are required for cluster control-plane stability.
- VM network route settings allow IPv6 CIDR and gateway where applicable.
- Host network config APIs and controllers support IPv6 addresses for L3 interface configuration, covering both static and SLAAC modes.
- SLAAC is the dynamic addressing mode for IPv6, equivalent in role to DHCPv4 for IPv4: it allows nodes to self-configure addresses from Router Advertisement prefixes without requiring manual per-node assignment or server infrastructure.
- Validation and status conditions report IPv6-specific errors clearly.
- Dual-stack support, meaning IPv4 and IPv6 enabled together.

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

### User Experience In Detail

#### Installation

- Existing install flow is extended with IPv6 fields where management settings are provided. Management interfaces always use static addressing, since stable predictable addresses are required for cluster control-plane communication. This is installer and host-networking work, not only Kubernetes API work; see [install.management_interface](https://docs.harvesterhci.io/v1.8/install/harvester-configuration#installmanagement_interface) for the current management interface configuration schema.
- Validation includes IPv6 address/prefix format, gateway format, and family consistency.
- For dual-stack, at least one reachable route is required for cluster bootstrap.

#### Cluster Network and Host Network Configuration

- HostNetworkConfig is extended to represent IPv6 L3 addresses in both static and SLAAC modes. For the practical meaning of cluster network, bridge, bond, VLAN, and host-side networking, see [Cluster Network](https://docs.harvesterhci.io/v1.8/networking/) and the [Harvester Network Deep Dive](https://docs.harvesterhci.io/v1.8/networking/deep-dive).
- Users can choose static or SLAAC for host VLAN sub-interfaces.
- For SLAAC, the default gateway is not a config field — it is populated by the upstream router via Router Advertisement. The controller does not need to set a gateway explicitly.
- Status conditions show per-node apply state and failure reason.

#### VM Network Configuration

- VM network route settings support IPv6 CIDR and gateway. This is largely extending current IPv4-oriented route and validation behavior rather than inventing a new network model; see [VM Network](https://docs.harvesterhci.io/v1.8/networking/harvester-network).
- Connectivity checks include IPv6 path validation from all applicable nodes.

#### Operational Workflows

- Reboot, maintenance mode, and node replacement preserve and reconcile IPv6 configuration.
- Documentation provides equivalent commands and examples for IPv4-only, IPv6-only, and dual-stack setups.

### API changes

The exact schema names may vary with final implementation, but the API shape below captures required capabilities.

```go
type IPFamily string

const (
    IPFamilyIPv4 IPFamily = "IPv4"
    IPFamilyIPv6 IPFamily = "IPv6"
    IPFamilyDual IPFamily = "DualStack"
)

type IPAssignmentMode string

const (
    IPAssignmentModeStatic IPAssignmentMode = "static"
    IPAssignmentModeDHCPv4 IPAssignmentMode = "dhcpv4"
    IPAssignmentModeSLAAC  IPAssignmentMode = "slaac"
    // DHCPv6 is not in scope for this HEP.
)

type HostIPAssignment struct {
    NodeName string           `json:"nodeName"`
    IPv4CIDR string           `json:"ipv4CIDR,omitempty"`
    IPv6CIDR string           `json:"ipv6CIDR,omitempty"`
    Mode     IPAssignmentMode `json:"mode"`
}

type HostNetworkConfigSpec struct {
    ClusterNetwork string                `json:"clusterNetwork"`
    VlanID         uint16                `json:"vlanID"`
    NodeSelector   *metav1.LabelSelector `json:"nodeSelector,omitempty"`
    Family         IPFamily              `json:"family"`
    Assignments    []HostIPAssignment    `json:"assignments,omitempty"`
}

type VMNetworkRouteSpec struct {
    CIDRv4   string `json:"cidrV4,omitempty"`
    Gateway4 string `json:"gatewayV4,omitempty"`
    CIDRv6   string `json:"cidrV6,omitempty"`
    Gateway6 string `json:"gatewayV6,omitempty"`
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

2. harvester
- Extend API validation/webhooks for IPv6 fields in relevant resources.
- Extend controllers and status reporting to handle IPv6 conditions.
- Ensure upgrade workflows preserve and validate IPv6 settings.

3. network-controller-harvester
- Extend host network config reconciliation for static and SLAAC modes.
- For SLAAC, configure NetworkManager profiles with `ipv6.method=auto` and do not set a gateway field; the gateway is derived from Router Advertisements.
- Add IPv6 route and reachability checks for VM network route configuration.
- Surface per-node condition details for IPv6 failures.

4. harvester-ui-extension
- Extend forms and validation for IPv6 and dual-stack inputs.
- Expose static and SLAAC as explicit mode choices; do not offer DHCPv6.
- Show family-aware warnings and connectivity status in UI.

5. docs
- Add user and operator guides for IPv6-only and dual-stack deployment.
- Document that SLAAC requires a router sending Router Advertisements on the segment and that the gateway is not a user-configured field.
- Add troubleshooting guidance for common IPv6 issues, including RA not received.

6. charts and addons where applicable
- Validate default values and schema do not block IPv6 deployment.
- Document required settings for integrated addons when dual-stack is enabled.

### Validation Rules

- CIDR and gateway must match selected family.
- For static mode, per-node assignment with `ipv6CIDR` is required for each targeted node.
- For SLAAC mode, `ipv6CIDR` must not be set; the address is derived from the RA prefix.
- Family mismatches (for example IPv6 gateway with IPv4 CIDR) are rejected.
- Duplicate IP assignment across nodes in the same HostNetworkConfig is rejected.
- SLAAC and static mode must not be mixed within the same HostNetworkConfig.

### Failure Modes and Reporting

- Node-level condition for each apply step: vlan create, link up, address apply, route checks.
- For SLAAC, an additional condition surfaces when no RA-derived address is observed within a timeout window (indicating the upstream router is not sending RAs on the segment).
- Explicit reason codes for invalid config, interface unavailable, and external dependency failures.
- Reconciliation retries are bounded and observable.

### Test plan

#### Environment Matrix

- Single-node and 3-node clusters.
- IPv4-only baseline.
- IPv6-only.
- Dual-stack.

#### Installation Tests

1. Install with IPv6-only management interface (static), verify cluster bootstrap.
2. Install with dual-stack management interface, verify node registration and reboot stability.
3. Negative tests for malformed IPv6 addresses, invalid prefixes, and invalid gateways.

#### Host Network Tests

1. Create HostNetworkConfig with static IPv6 per node.
2. Create HostNetworkConfig with SLAAC mode; verify RA-derived address appears in status.
3. Verify SLAAC gateway is populated from RA and not from a user config field.
4. Reboot nodes and verify persistence of static and SLAAC configurations.
5. Add and remove nodes; verify reconciliation and status reporting.
6. Negative test: SLAAC mode with no RA on segment surfaces clear condition/error.

#### VM Network Tests

1. Create VM network route with IPv6 CIDR/gateway.
2. Verify connectivity checks and status updates.
3. Validate dual-stack route settings.
4. Validate webhook rejection of mismatched family input.

#### Upgrade and Regression Tests

1. Upgrade IPv4-only clusters and verify no regression.
2. Upgrade clusters with IPv6 configuration and verify persistence.
3. Validate fallback and recovery guidance for invalid post-upgrade custom networking.

#### E2E Acceptance

- VM to external IPv6 connectivity over VLAN network.
- VM scheduling behavior remains correct with cluster network selectors.
- Storage and migration workflows remain functional in dual-stack setup where configured.

### Upgrade strategy

- New installs: generate and apply IPv6-capable NetworkManager profiles from install config.
- Upgrades: preserve existing behavior and add IPv6 fields in backward-compatible API versions.
- Pre-upgrade checks identify invalid or incomplete IPv6 configuration and block unsafe rollout.
- Provide a clear rollback path to last known-good network configuration if post-upgrade reconciliation fails.

For upgrades from older versions where wicked artifacts still exist, wicked data is treated only as historical input during migration. Runtime host networking remains NetworkManager-based in supported v1.7+ upgrades.

## Note

This HEP defines a technical, package-by-package implementation scope so the resulting work can be split into sequenced PRs across harvester-installer, harvester, network-controller-harvester, harvester-ui-extension, and docs, while preserving existing IPv4 behavior and upgrade safety.

DHCPv6 is not included in this HEP. SLAAC covers the dynamic addressing need for IPv6 without requiring server infrastructure and is the right scope boundary for this work.
