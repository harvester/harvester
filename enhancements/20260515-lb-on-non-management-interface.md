# Support Load Balancers on Non-Management Interfaces for Guest Clusters

## Summary

When a guest cluster has multiple network interfaces, all `LoadBalancer` Service VIPs are bound to the management interface regardless of which network the IP pool or DHCP server belongs to. This enhancement fixes the binding for both IP pool mode (via `serviceInterface: auto`) and DHCP mode (via user-selected interface exposed through a Node annotation).

### Related Issues

https://github.com/harvester/harvester/issues/5486

## Motivation

kube-vip defaults to the management interface when no interface hint is provided, defeating network isolation for workload-network IP pools or DHCP servers.

### Goals

- **IP pool mode**: VIPs are automatically bound to the interface matching the IP pool's subnet.
- **DHCP mode**: Users can select the correct network interface from the UI.

### Non-goals

- Deal with asymmetric network topology where nodes have different interface configurations, e.g.:

```
Management Node:  enp1s0 → net-mgmt (192.168.100.0/24)
Worker Node:      enp1s0 → net-mgmt (192.168.100.0/24)
                  enp2s0 → net-101  (192.168.101.0/24)
```

## Introduction

`harvester-cloud-provider` runs in the guest cluster and connects to both the guest cluster (to manage Services) and the Harvester cluster (to manage `LoadBalancer` CRs and query VMI resources).

When a `LoadBalancer` Service is created:

1. The CCM service controller calls `EnsureLoadBalancer()`.
2. The cloud provider creates a `LoadBalancer` CR on Harvester.
3. Harvester allocates an IP from the matching pool.
4. The cloud provider writes the IP to `kube-vip.io/loadbalancerIPs`.
5. kube-vip binds the VIP to an interface — defaulting to management.

**IP pool mode — broken state:**

```
enp1s0 → net-mgmt (192.168.100.0/24)
enp2s0 → net-101  (192.168.101.0/24)
```

```shell
$ ip addr show enp1s0
    inet 192.168.101.57/32 scope global enp1s0   ← VIP on wrong interface
```

**DHCP mode — broken state:**

In DHCP mode `kube-vip.io/loadbalancerIPs` is set to `0.0.0.0`. kube-vip creates a macvlan sub-interface and sends a DHCP request using the macvlan's unique MAC. `serviceInterface: auto` cannot work here — it finds interfaces by subnet match, and `0.0.0.0` matches nothing — so kube-vip falls back to the management interface and is not able to use non-management network.

## Proposal

### IP Pool Mode

**Interface binding**: Set `kube-vip.io/serviceInterface: auto` alongside `kube-vip.io/loadbalancerIPs`. kube-vip's `autoFindInterface()` locates the interface whose subnet contains the VIP.

**Network selection**: The frontend reads the NAD keys from the `cloudprovider.harvesterhci.io/interface-nad-mapping` annotation already written on each Node and shows them as a dropdown — e.g. `default/net123`. The user's selection sets `cloudprovider.harvesterhci.io/network: default/net123` on the Service. The cloud provider copies this to the LoadBalancer CR, and the load-balancer controller picks the matching IPPool.

### DHCP Mode

Since `auto` cannot work with `0.0.0.0`, an explicit interface name is needed. Raw Linux names (`enp1s0`, `enp2s0`) are meaningless to users, so the cloud provider builds and exposes a mapping.

**Interface mapping on Nodes**

Using `VirtualMachineInstance.spec.networks` (nic name → NAD name) joined with `VirtualMachineInstance.status.interfaces` (nic name → Linux interface name, from the KubeVirt guest agent), it writes:

```
cloudprovider.harvesterhci.io/interface-nad-mapping: '{"enp1s0":"default/mgmt-vlan1","enp2s0":"default/net123"}'
```

Interfaces without a multus `name` (calico, kube-vip macvlan, etc.) are excluded automatically.

**Frontend interface selection**

The frontend reads the annotation from any Node and shows a dropdown when DHCP mode is selected:

```
enp1s0  (default/mgmt-vlan1)
enp2s0  (default/net123)       ← user selects this
```

The frontend sets:

```yaml
cloudprovider.harvesterhci.io/ipam: dhcp
kube-vip.io/loadbalancerIPs: 0.0.0.0
kube-vip.io/serviceInterface: enp2s0
```

**Cloud provider preserves the choice**

For DHCP mode, the cloud provider must not overwrite `kube-vip.io/serviceInterface` with `"auto"`. The `expectedServiceInterface()` helper returns `"auto"` for pool mode and preserves the existing annotation value for DHCP mode.

```shell
4: vip-3fa2c1d8@enp2s0: ...   ← macvlan on correct interface
    inet 192.168.101.82/32 ...
```

### User Stories

#### Story 1

I want an app exposed on `net-101`. The UI shows the NAD names from the Node's `interface-nad-mapping` annotation. I select `default/net-101`; the frontend sets `cloudprovider.harvesterhci.io/network: default/net-101` on the Service. The cloud provider allocates an IP from the matching pool; kube-vip automatically places the VIP on `enp2s0`.

#### Story 2

I want to expose an app via a DHCP `LoadBalancer` on my workload network. The UI shows `enp1s0 (default/mgmt-vlan1)` and `enp2s0 (default/net123)`. I select `enp2s0`. kube-vip creates the macvlan on `enp2s0`, gets a DHCP address from the workload subnet, and binds the VIP there.

### API Changes

```go
// pkg/cloud-controller-manager/annotation.go
KeyKubevipServiceInterface = "kube-vip.io/serviceInterface"
KeyInterfaceNADMapping     = "cloudprovider.harvesterhci.io/interface-nad-mapping"
```

No new CRDs, no changes to the `LoadBalancer` CR schema, no RBAC changes required.

Also, [`svc_election`](https://kube-vip.io/docs/usage/kubernetes-services/#load-balancing-load-balancers-when-using-arp-mode-yes-you-read-that-correctly-kube-vip-v050) can be enabled for per-Service leader election instead of a single global leader.

## Design

### Implementation Overview

**IP pool mode**: When writing `kube-vip.io/loadbalancerIPs`, also write `kube-vip.io/serviceInterface: auto`.

**DHCP mode**:
- In `InstanceMetadata()`, build the interface → NAD mapping from VMI spec/status and write it as a Node annotation.
- Add `expectedServiceInterface()` to `loadbalancer.go`: returns `"auto"` for pool mode, returns the existing `kube-vip.io/serviceInterface` value for DHCP mode.

Action Items:

- [ ] Add `KeyKubevipServiceInterface`, `KeyInterfaceNADMapping` to `annotation.go`.
- [ ] Set `kube-vip.io/serviceInterface: auto` for primary and secondary Services in `loadbalancer.go`.
- [ ] Add `expectedServiceInterface()` helper to `loadbalancer.go`.
- [ ] Add `nodeClient` to `instanceManager`; implement `getInterfaceToNADMapping()`, `annotateNodeWithInterfaceMapping()` in `instance.go`.
- [ ] Update unit tests in `loadbalancer_test.go`.
- [ ] Update the Harvester frontend to read NAD keys from `KeyInterfaceNADMapping` for both pool mode network dropdown and DHCP mode interface dropdown.

### Test Plan

#### IP Pool Mode

1. Create a VM Network `net123` and IPPool with `selector.network: default/net123` scoped to the guest cluster.
2. Provision a guest cluster with nodes on both networks.
3. Verify the Node annotation is written:
   ```shell
   kubectl get node <node> -o jsonpath='{.metadata.annotations.cloudprovider\.harvesterhci\.io/interface-nad-mapping}' | jq .
   ```
4. Confirm the NAD keys (e.g. `default/net123`) appear in the UI network dropdown for pool mode; select `default/net123` and confirm `cloudprovider.harvesterhci.io/network: default/net123` is set on the Service.
5. Verify the VIP is bound to `enp2s0` and connectivity from the workload subnet works.

#### DHCP Mode

1. Create a VM Network with a DHCP server; confirm nodes show `AgentConnected: True`.
2. Verify the Node annotation is written:
   ```shell
   kubectl get node <node> -o jsonpath='{.metadata.annotations.cloudprovider\.harvesterhci\.io/interface-nad-mapping}' | jq .
   ```
3. Create a `LoadBalancer` Service in DHCP mode, select `enp2s0 (default/net123)` from the UI dropdown.
4. Confirm `kube-vip.io/serviceInterface: enp2s0` is set on the Service.
5. Verify the macvlan is created on `enp2s0`:
   ```shell
   ip addr show   # Expected: vip-xxxxxxxx@enp2s0
   ```
6. Confirm connectivity from a client on the workload subnet.

### Upgrade Strategy

After upgrading `harvester-cloud-provider`, the reconciler patches existing Services with `kube-vip.io/serviceInterface: auto`. A one-time kube-vip restart is required for the new annotation to take effect:

```shell
kubectl rollout restart daemonset kube-vip -n kube-system
```

New Services created post-upgrade receive the annotation automatically.

## Note

None.
