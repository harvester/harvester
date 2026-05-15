# Support Load Balancers on Non-Management Interfaces for Guest Clusters

## Summary

When a guest cluster has multiple network interfaces, all `LoadBalancer` Service VIPs are bound to the management interface regardless of which network the IP pool or DHCP server belongs to. This enhancement fixes the binding by allowing users to select a NAD name from the UI; the cloud provider resolves it to the correct Linux interface via a shared ConfigMap and sets `kube-vip.io/serviceInterface` on the Service.

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

**Network selection**: The frontend reads the NAD keys from the `harvester-nad-mapping` ConfigMap (in `kube-system`) and shows them as a dropdown — e.g. `default/net123`. The user's selection sets `cloudprovider.harvesterhci.io/network: default/net123` on the Service. The cloud provider resolves the NAD name to a Linux interface via the ConfigMap and writes `kube-vip.io/serviceInterface` accordingly; the load-balancer controller picks the matching IPPool.

### DHCP Mode

Both DHCP and IP pool modes use the same unified approach: the frontend selects a NAD name and the cloud provider resolves it to a Linux interface via the ConfigMap.

**Interface mapping ConfigMap**

Using `VirtualMachineInstance.spec.networks` (nic name → NAD name) joined with `VirtualMachineInstance.status.interfaces` (nic name → Linux interface name, from the KubeVirt guest agent), the cloud provider syncs a ConfigMap in `kube-system`:

```yaml
apiVersion: v1
data:
  interface-nad-mapping: '{"default/mgmt-vlan1":"enp1s0","default/net123":"enp2s0"}'
kind: ConfigMap
metadata:
  name: harvester-nad-mapping
  namespace: kube-system
```

Interfaces without a multus `name` (calico, kube-vip macvlan, etc.) are excluded automatically. Only NADs consistently present on the same interface across all nodes are included.

**Frontend interface selection**

The frontend reads the `harvester-nad-mapping` ConfigMap and shows a dropdown when a specific network is needed:

```
default/mgmt-vlan1
default/net123      ← user selects this
```

The frontend sets only:

```yaml
cloudprovider.harvesterhci.io/ipam: dhcp
cloudprovider.harvesterhci.io/network: default/net123
```

**Cloud provider resolves the interface**

For both DHCP and IP pool modes, when `cloudprovider.harvesterhci.io/network` is present, the cloud provider looks up the corresponding Linux interface name in the `harvester-nad-mapping` ConfigMap and writes it to `kube-vip.io/serviceInterface`. If no network is specified, the interface annotation is left unset and kube-vip falls back to its default behavior.

```shell
4: vip-3fa2c1d8@enp2s0: ...   ← macvlan on correct interface
    inet 192.168.101.82/32 ...
```

### User Stories

#### Story 1

I want an app exposed on `net-101`. The UI shows the NAD names from the `harvester-nad-mapping` ConfigMap. I select `default/net-101`; the frontend sets `cloudprovider.harvesterhci.io/network: default/net-101` on the Service. The cloud provider allocates an IP from the matching pool; kube-vip automatically places the VIP on `enp2s0`.

#### Story 2

I want to expose an app via a DHCP `LoadBalancer` on my workload network. The UI shows `default/mgmt-vlan1` and `default/net123`. I select `default/net123`; the frontend sets `cloudprovider.harvesterhci.io/network: default/net123`. The cloud provider resolves this to `enp2s0` via the ConfigMap and sets `kube-vip.io/serviceInterface: enp2s0`; kube-vip creates the macvlan on `enp2s0`, gets a DHCP address from the workload subnet, and binds the VIP there.

### API Changes

```go
// pkg/cloud-controller-manager/annotation.go
KeyKubevipServiceInterface = "kube-vip.io/serviceInterface"

// pkg/utils/consts.go
ConfigMapNADMapping    = "harvester-nad-mapping"    // ConfigMap name in kube-system
ConfigMapKeyNADMapping = "interface-nad-mapping"    // data key inside the ConfigMap
```

No new CRDs, no changes to the `LoadBalancer` CR schema. RBAC for `harvester-cloud-provider` must be extended to allow read/write on `ConfigMap` in `kube-system`.

Also, [`svc_election`](https://kube-vip.io/docs/usage/kubernetes-services/#load-balancing-load-balancers-when-using-arp-mode-yes-you-read-that-correctly-kube-vip-v050) can be enabled for per-Service leader election instead of a single global leader.

## Design

### Implementation Overview

**Both modes (unified)**: When `cloudprovider.harvesterhci.io/network` is present on a Service, the cloud provider looks up the Linux interface name in the `harvester-nad-mapping` ConfigMap and writes `kube-vip.io/serviceInterface` accordingly. If no network is specified, the annotation is left unset.

**ConfigMap sync**: In a VMI controller, build the NAD → interface mapping from VMI spec/status (intersection across all qualifying VMIs) and sync it to the `harvester-nad-mapping` ConfigMap in `kube-system`.

### Test Plan

#### IP Pool Mode

1. Create a VM Network `net123` and IPPool with `selector.network: default/net123` scoped to the guest cluster.
2. Provision a guest cluster with nodes on both networks.
3. Verify the ConfigMap is written:
   ```shell
   kubectl get configmap -n kube-system harvester-nad-mapping -o jsonpath='{.data.interface-nad-mapping}' | jq .
   ```
4. Confirm the NAD keys (e.g. `default/net123`) appear in the UI network dropdown for pool mode; select `default/net123` and confirm `cloudprovider.harvesterhci.io/network: default/net123` is set on the Service.
5. Verify the VIP is bound to `enp2s0` and connectivity from the workload subnet works.

#### DHCP Mode

1. Create a VM Network with a DHCP server.
2. Verify the ConfigMap is written:
   ```shell
   kubectl get configmap -n kube-system harvester-nad-mapping -o jsonpath='{.data.interface-nad-mapping}' | jq .
   ```
3. Create a `LoadBalancer` Service in DHCP mode, select `default/net123 (enp2s0)` from the UI dropdown.
4. Confirm `kube-vip.io/serviceInterface: enp2s0` is set on the Service.
5. Verify the macvlan is created on `enp2s0`:
   ```shell
   ip addr show   # Expected: vip-xxxxxxxx@enp2s0
   ```
6. Confirm connectivity from a client on the workload subnet.

### Upgrade Strategy

After upgrading `harvester-cloud-provider`, existing Services that already have `cloudprovider.harvesterhci.io/network` will have `kube-vip.io/serviceInterface` resolved and written by the reconciler. Services without a network annotation are not affected. A one-time kube-vip restart may be required for the new annotation to take effect:

```shell
kubectl rollout restart daemonset kube-vip -n kube-system
```

New Services created post-upgrade receive the annotation automatically.

## Note

None.
