# Guest RWX Volume Resilience

## Summary

This design document outlines a long-term resiliency improvement for guest cluster RWX volumes in Harvester. Today, guest RWX volumes can become unavailable when the underlying Longhorn Share Manager pod is recreated because the guest-side `NetworkFilesystem` may point directly to the Share Manager pod IP. If that pod is rescheduled during maintenance, node drain, upgrade, or failure recovery, the pod IP can change and existing guest node mounts become stale.

The proposed design introduces a stable VIP-backed access path on the configured RWX network. Harvester reserves addresses from the Whereabouts CIDR, prepares host networking for the RWX network, runs a kube-vip based data-plane component, and exposes each Share Manager through a stable Service IP. The external [`harvester/networkfs-manager`](https://github.com/harvester/networkfs-manager) controller then records that stable Service IP in `NetworkFilesystem.status.endpoint`, and the Harvester CSI driver publishes it to the guest cluster instead of the transient Share Manager pod IP.

### Related Issues

- [Issue #10174](https://github.com/harvester/harvester/issues/10174)
- [Issue #10047](https://github.com/harvester/harvester/issues/10047)

## Motivation

### Goals

- **Provide a Stable RWX Mount Endpoint**: Guest clusters should mount RWX volumes through a stable Service IP on the RWX network instead of the Share Manager pod IP.
- **Survive Share Manager Pod Recreation**: Guest workloads should continue to access RWX volumes when the Share Manager pod is recreated and receives a different pod IP.
- **Reserve Network Addresses Explicitly**: VIPs used by Harvester RWX infrastructure and Share Manager Services must be reserved from the Whereabouts CIDR to avoid IP conflicts.
- **Integrate with Existing RWX Network Settings**: The solution should use the configured `rwx-network` setting and source NetworkAttachmentDefinition (NAD) cluster network, VLAN, CIDR, and exclude list.
- **Keep Storage Network Allocation Safe**: When RWX shares the storage network, Harvester must avoid addresses already allocated by the storage network IPPool and update the storage network exclude list as needed.
- **Automate Host Network Preparation**: Harvester should create or reconcile the required host network configuration for the RWX network when the RWX network is configured.

### Non-goals

- Redesign Longhorn Share Manager or replace Longhorn NFS serving behavior.
- Change the guest Kubernetes RWX user workflow.
- Support arbitrary external load balancers for Share Manager Services in the first implementation.
- Provide cross-subnet VIP routing for RWX traffic. The initial design expects VIPs to come from the same subnet as the source RWX network.

## Proposal

### User Stories

**Story 1: As a guest cluster user**
I want RWX volumes to remain mounted during Harvester maintenance so that workloads do not lose access to shared data when a Share Manager pod is recreated.

**Story 2: As a Harvester administrator**
I want Harvester to reserve RWX network addresses automatically so that VIPs do not collide with pod IPs, node IPs, or storage-network allocations.

**Story 3: As a Harvester maintainer**
I want RWX network host configuration, VIP infrastructure, and Share Manager endpoint reconciliation to be controller-driven so that recovery is idempotent and observable.

**Story 4: As a storage operator**
I want the Harvester CSI driver to publish a stable RWX endpoint to the guest cluster so that guest-side `NetworkFilesystem` resources do not depend on transient host pod IPs.

### User Experience In Detail

An administrator configures the `rwx-network` setting. The setting either provides a dedicated RWX network or shares the existing storage network.

When the setting is valid, Harvester reconciles the source NAD and prepares the related network resources:

- A host network configuration is created from the source NAD cluster network and VLAN.
- One address per Harvester node is reserved from the Whereabouts CIDR for the node-level VIP component.
- A kube-vip DaemonSet is created in `longhorn-system`, for example `kube-vip-storage`.

When a guest RWX volume is created, Longhorn creates a Share Manager. Harvester then creates a stable Service and EndpointSlice for that Share Manager:

- The Service receives a reserved VIP from the source NAD CIDR.
- The EndpointSlice points to the current Share Manager pod IP.
- `harvester/networkfs-manager` records the Service IP in `NetworkFilesystem.status.endpoint`, which the Harvester CSI driver consumes.
- The Harvester CSI driver exposes the stable endpoint to the guest cluster.

If the Share Manager pod is recreated, Harvester updates the EndpointSlice to the new pod IP while keeping the Service IP unchanged. The guest mount continues to target the same stable IP. Harvester also sends gratuitous ARP from the restarted Share Manager path so Harvester hosts refresh stale ARP entries.

### API Changes

This proposal requires extending the host network configuration API so Harvester can distinguish RWX-managed host network configuration from normal user-managed configuration.

The exact field name is open for implementation, but the API should carry the resource purpose, for example:

```go
type HostNetworkConfigSpec struct {
    // Type identifies the owner or purpose of this host network configuration.
    // Supported values include normal user-managed configuration and RWX-managed configuration.
    Type HostNetworkConfigType `json:"type,omitempty"`
}

type HostNetworkConfigType string

const (
    HostNetworkConfigTypeDefault HostNetworkConfigType = "default"
    HostNetworkConfigTypeRWX     HostNetworkConfigType = "rwx"
)
```

RWX-managed host network configuration should allow stricter validation:

- Static mode only.
- Node IPs must come from the same subnet as the source RWX network.
- Underlay cannot be enabled.
- VIP addresses must be excluded from Whereabouts allocation.

The `rwx-network` setting may also be extended later with an explicit VIP field if Harvester needs to make the infrastructure VIP user-visible or user-selectable. The first implementation can allocate the VIP automatically and record it in the managed resources.

## Design

### Architecture Overview

The design introduces three cooperating reconciliation areas:

- **RWX Network Reconciler**: Watches the `rwx-network` setting and source NAD. It creates or updates the host network configuration, reserves per-node addresses, and reconciles the kube-vip DaemonSet.
- **Share Manager VIP Reconciler**: Watches Longhorn Share Manager pods or related volume state. It allocates a stable Service IP, creates the Service and EndpointSlice, and updates the EndpointSlice when the Share Manager pod changes.
- **networkfs-manager Integration**: Updates the external `harvester/networkfs-manager` controller so `NetworkFilesystem.status.endpoint` is populated from the VIP-backed Share Manager Service. The Harvester CSI driver continues to consume the `NetworkFilesystem` endpoint and publish it to guest clusters.

The stable endpoint is a Kubernetes Service IP on the RWX network. kube-vip provides the host-side VIP handling needed to make those addresses reachable through the selected RWX interface.

### RWX Network Configuration Flow

When `rwx-network` is configured:

1. Decode the `rwx-network` setting and resolve the source NAD.
2. Read the source NAD cluster network, VLAN, CIDR, and current `exclude` list.
3. If the RWX network shares the storage network, read the storage network IPPool and exclude already allocated addresses from RWX reservation candidates.
4. Select and reserve one IP per Harvester node from the Whereabouts CIDR by updating the source NAD `exclude` field.
5. Create or update the RWX-managed host network configuration from the source NAD cluster network and VLAN.
6. Create or update the kube-vip DaemonSet, for example `longhorn-system/kube-vip-storage`.
7. Configure `vip_interface` from the RWX host network configuration so kube-vip advertises on the correct interface.

The reconciler must be idempotent. If the setting changes, it should preserve already assigned stable IPs when they remain valid and release only addresses no longer owned by RWX infrastructure.

### Share Manager VIP Flow

When a Share Manager is brought up for a guest RWX volume:

1. Locate the Share Manager pod and determine its current pod IP.
2. Reserve one Service IP from the Whereabouts CIDR by updating the source NAD `exclude` field.
3. Create or update a Service in `longhorn-system`, for example `share-manager-vip`.
4. Create or update an EndpointSlice, for example `share-manager-vip-slice`.
5. Set the EndpointSlice endpoint address to the current Share Manager pod IP.
6. Record ownership labels and owner references where possible so the Service IP can be released when the Share Manager is deleted.
7. Publish the Service IP by updating `NetworkFilesystem.status.endpoint` through `harvester/networkfs-manager`.

The Service name should be derived from the Longhorn volume or Share Manager identity rather than using one global Service name. This keeps multiple RWX volumes independent.

### Share Manager Pod Recreation Flow

When a Share Manager pod is recreated:

1. The Share Manager VIP reconciler observes the new pod IP.
2. The related EndpointSlice is updated to point to the new pod IP.
3. The Service IP remains unchanged.
4. Harvester triggers gratuitous ARP from the restarted Share Manager path so Harvester hosts refresh ARP entries for the stable Service IP.
5. The guest workload continues to mount the same Service IP and avoids stale NFS endpoint configuration.

### IP Reservation Rules

All addresses owned by this feature must be represented in the source NAD `exclude` field:

- Per-node VIP addresses used by the kube-vip infrastructure.
- Per-Share Manager Service IPs.
- Any future explicit RWX infrastructure VIP.

When the RWX network shares the storage network, Harvester must also account for addresses already allocated in the storage network IPPool. The RWX allocator must skip those addresses and update the storage network exclude list if the shared network requires a single authoritative exclusion source.

Reserved addresses should be tracked with labels or annotations on the owning Service, DaemonSet, or host network configuration so reconciliation can distinguish Harvester-owned exclusions from user-provided exclusions.

### Failure Handling

- If no free IP is available, the affected RWX volume should surface a clear condition or event and retry when the network range changes.
- If the source NAD is missing or malformed, reconciliation should stop with a clear error event.
- If kube-vip is not ready, Service and EndpointSlice reconciliation can continue, but `NetworkFilesystem.status.endpoint` publication should wait until the stable endpoint is reachable.
- If the Share Manager pod is temporarily unavailable, the EndpointSlice should be updated when the pod returns rather than changing the Service IP.

## Implementation Details

### Part 1: Host Network Configuration

Add an RWX purpose field to the host network configuration CRD. The validating webhook should apply RWX-specific restrictions:

- Static configuration mode only.
- All configured node IPs must be in the same subnet as the RWX source NAD.
- Underlay is rejected.
- Referenced cluster network and VLAN must match the source NAD.

The controller creates this host network configuration from the `rwx-network` source NAD. It should not overwrite user-managed host network configuration unless it owns the object.

### Part 2: RWX IP Allocator

Add a small allocator helper for RWX-managed reservations. The helper should:

- Parse the source NAD bridge and Whereabouts IPAM config.
- Read and preserve existing `exclude` entries.
- Skip IPs already allocated in the storage network IPPool when RWX shares the storage network.
- Allocate deterministic addresses when possible to reduce churn.
- Add and remove only Harvester-owned exclusions.

The allocator should use structured JSON parsing for the NAD config rather than string replacement.

### Part 3: kube-vip DaemonSet

Create or update a DaemonSet in `longhorn-system`, for example `kube-vip-storage`.

The DaemonSet should:

- Run on eligible Harvester nodes.
- Use the interface discovered from the RWX host network configuration as `vip_interface`.
- Use the reserved per-node addresses from the RWX network.
- Be owned or labeled by Harvester so upgrades and cleanup are safe.

### Part 4: Share Manager Service and EndpointSlice Controller

Add a controller that reconciles Share Manager VIP resources:

- Watch Share Manager pods and the related Longhorn volume state.
- Allocate one stable Service IP per guest RWX Share Manager.
- Create or update a Service in `longhorn-system`.
- Create or update a matching EndpointSlice that targets the current Share Manager pod IP.
- Keep the Service IP stable across pod recreation.
- Release the Service IP reservation when the Share Manager no longer exists.

### Part 5: networkfs-manager and CSI Publishing

Update the external `harvester/networkfs-manager` repository so it publishes the Share Manager VIP Service IP to `NetworkFilesystem.status.endpoint`.

The current networkfs-manager controller already reads the Longhorn Service for the `NetworkFilesystem` name. If the Service is not headless, it uses `service.spec.clusterIP`; if the Service is headless, it falls back to the Endpoints address. This proposal should preserve that behavior for existing Longhorn RWX volumes and make the VIP-backed Service the preferred source for guest RWX resilience.

The networkfs-manager changes should include:

- Recognize the VIP-backed Share Manager Service as the endpoint source for guest RWX volumes.
- Avoid falling back to the Share Manager pod IP when the VIP-backed Service exists but is not fully ready yet.
- Keep `NetworkFilesystem.status.endpoint`, `status.type`, `status.mountOptions`, and endpoint changed conditions compatible with the existing Harvester CSI driver contract.
- Add RBAC for EndpointSlice read access if networkfs-manager is responsible for checking the VIP EndpointSlice readiness.
- Keep Longhorn VolumeAttachment handling unchanged unless the stable endpoint flow requires a different Share Manager attachment trigger.

The CSI-facing behavior should remain the same from the guest cluster perspective: guest workloads request RWX volumes normally and receive a mount endpoint through the existing Harvester CSI flow.

### Part 6: Gratuitous ARP

When the Share Manager pod is recreated and the EndpointSlice target changes, Harvester should trigger gratuitous ARP from the restarted Share Manager path. This refreshes ARP entries on Harvester hosts for the stable Service IP and reduces disruption caused by stale L2 neighbor state.

The implementation detail is open. Options include a small helper container, an init action, or a controller-triggered command in the relevant network namespace.

## Upgrade Strategy

The feature is additive.

- Existing clusters without `rwx-network` configured continue to behave as before.
- Existing guest RWX volumes continue using their current endpoint until reconciled by the new controller.
- When `rwx-network` is configured after upgrade, Harvester creates the managed host network configuration, reserves addresses, starts kube-vip, and begins publishing stable Service IPs for new or reconciled guest RWX volumes.
- Existing NAD `exclude` entries must be preserved. Harvester should add only its managed reservations.

Rollback should leave guest mounts using any already-published endpoint until the old controller path reconciles them. The rollback process should document whether administrators need to remove Harvester-managed `exclude` entries and VIP resources manually.

## Limitations and Implementation

- The initial design depends on Whereabouts-backed RWX networking.
- VIP and Share Manager Service IPs must be in the same subnet as the source RWX network.
- The first implementation assumes kube-vip is the VIP data-plane component.
- The design does not change Longhorn Share Manager internals.
- The design does not remove the need for enough free addresses in the RWX network CIDR.
- Gratuitous ARP behavior may depend on the selected interface and network namespace used by the Share Manager path.

## Test Plan

### Unit Tests

- Validate RWX host network configuration accepts only static same-subnet configuration.
- Validate RWX host network configuration rejects underlay.
- Validate the RWX IP allocator preserves user-provided NAD excludes.
- Validate the RWX IP allocator skips storage network IPPool allocations when sharing the storage network.
- Validate Service and EndpointSlice builders produce stable names, labels, and endpoint addresses.
- Validate networkfs-manager publishes the VIP-backed Service IP to `NetworkFilesystem.status.endpoint` when a Share Manager VIP Service exists.
- Validate networkfs-manager does not publish the transient Share Manager pod IP when the VIP-backed Service exists but its EndpointSlice is still converging.

### Integration Tests

- Configure a dedicated `rwx-network` and verify Harvester creates the RWX host network configuration.
- Verify one IP per node is reserved in the source NAD `exclude` field.
- Verify kube-vip DaemonSet is created with the expected `vip_interface`.
- Create a guest RWX volume and verify the Share Manager VIP Service and EndpointSlice are created.
- Verify the Service IP is added to the source NAD `exclude` field.
- Verify `NetworkFilesystem.status.endpoint` contains the Service IP rather than the Share Manager pod IP.
- Verify the guest cluster receives the Service IP from the Harvester CSI driver.
- Delete or restart the Share Manager pod and verify the EndpointSlice updates to the new pod IP while the Service IP remains unchanged.
- Verify guest workload RWX access continues after Share Manager pod recreation.
- Validate the shared-storage-network case where storage network IPPool allocations already exist before RWX reservations.

### Upgrade Tests

- Upgrade a cluster with `rwx-network` unset and verify no RWX VIP resources are created.
- Upgrade a cluster with `rwx-network` configured and verify Harvester creates managed RWX resources without removing existing NAD excludes.
- Upgrade a cluster with existing guest RWX volumes and verify they reconcile to stable Service IPs.
