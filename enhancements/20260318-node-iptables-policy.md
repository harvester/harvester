# Node-level iptables Policy Guide

## Summary

Harvester nodes are exposed to potential malicious scans and unauthorized access because there is currently no default inbound/forward traffic filtering policy. This HEP provides a documentation-only guide that describes the services and ports used on each Harvester node role, along with ready-to-use iptables rules that users can manually apply to harden their clusters.

**No Harvester code changes are required.** Users apply the iptables rules directly on each node via SSH or CloudInit CRD.

For background, harvester-installer [PR 1213](https://github.com/harvester/harvester-installer/pull/1213) applied iptables rules via a script.

### Related Issues

- https://github.com/harvester/harvester/issues/5681

## Motivation

### Goals

- Document all services and their listening ports on each Harvester node role (control-plane, worker, witness).
- Provide ready-to-use iptables rule sets that users can review, customize, and apply manually.
- Ensure the documentation clearly explains which ports are required and why, so users can make informed decisions about their security posture.

### Non-goals

- Egress filtering (out of scope).
- Target pod/vm-level traffic.
- Modifying any Harvester source code, controllers, or CRDs.
- Automated rule management or reconciliation via a controller.

## Proposal

### Proposed Solution

#### 1. Documentation of Services and Ports

Provide a comprehensive reference document that lists every service running on each Harvester node role, its listening port(s), protocol, scope (external vs. internal), and purpose. This enables users to understand the network surface of their cluster before applying any filtering rules.

#### 2. Ready-to-Use iptables Rules

Provide sample iptables commands for each node role (control-plane, worker, witness) that users can:

1. **Review** — understand which ports are opened and why.
2. **Customize** — add or remove rules to match their specific network topology and requirements.
3. **Apply manually** — run the commands via SSH on each node.

The rules use a custom chain (`HARVESTER_INPUT`) inserted into the `INPUT` hook, making them easy to manage without interfering with other iptables rules on the system.

#### 3. No Default Rules Shipped

No iptables rules are applied automatically on installation or upgrade. This ensures existing cluster traffic is never disrupted.

#### 4. Rule Persistence via CloudInit CRD

iptables rules are not persistent across reboots by default. The recommended approach for Harvester is to use the [`CloudInit` CRD](https://docs.harvesterhci.io/v1.7/advanced/cloudinitcrd/) to persist rules.

Example `CloudInit` resource for control-plane nodes:

```yaml
apiVersion: node.harvesterhci.io/v1beta1
kind: CloudInit
metadata:
  name: iptables-control-plane
spec:
  matchSelector:
    node-role.kubernetes.io/control-plane: "true"
    node-role.kubernetes.io/etcd: "true"
  filename: 99-iptables-control-plane
  contents: |
    stages:
      boot:
        - name: "Apply iptables rules for control-plane node"
          commands:
            - iptables -D INPUT -i mgmt-br -j HARVESTER_INPUT 2>/dev/null || true
            - iptables -F HARVESTER_INPUT 2>/dev/null || true
            - iptables -X HARVESTER_INPUT 2>/dev/null || true
            - iptables -N HARVESTER_INPUT
            - iptables -I INPUT -i mgmt-br -j HARVESTER_INPUT
            - iptables -A HARVESTER_INPUT -m conntrack --ctstate ESTABLISHED,RELATED -m comment --comment "Accept return traffic from established connections" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p icmp -m comment --comment "ICMP (ping, MTU discovery, etc.)" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --sport 67 --dport 68 -m comment --comment "DHCP server reply" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --sport 68 --dport 67 -m comment --comment "DHCP client request" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 22 -m comment --comment "SSH" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 80 -m comment --comment "Harvester UI HTTP" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 443 -m comment --comment "Harvester UI HTTPS" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 2112 -m comment --comment "RKE2 kube-vip Prometheus metrics" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 2379:2380 -m comment --comment "etcd client/peer" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 6443 -m comment --comment "Kubernetes API server" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 6641:6644 -m comment --comment "OVN NB/SB DB and JSON-RPC" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 8080 -m comment --comment "kube-ovn-webhook HTTP" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 8443 -m comment --comment "kube-ovn-webhook HTTPS" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 9091 -m comment --comment "calico-node metrics endpoint (Prometheus format)" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 9345 -m comment --comment "RKE2 supervisor API" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 9796 -m comment --comment "Prometheus node-exporter metrics" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 10250 -m comment --comment "kubelet API" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 10661 -m comment --comment "kube-ovn-monitor metrics" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 10665 -m comment --comment "kube-ovn-daemon" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 30000:32767 -m comment --comment "NodePort TCP range" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --dport 30000:32767 -m comment --comment "NodePort UDP range" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --dport 8472 -m comment --comment "VXLAN Flannel/Canal" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --dport 4789 -m comment --comment "VXLAN Kube-OVN" -j ACCEPT
            - iptables -A HARVESTER_INPUT -m limit --limit 10/min -m comment --comment "Rate-limit logging of dropped packets" -j LOG --log-prefix HARVESTER_DROP_ --log-level 4
            - iptables -A HARVESTER_INPUT -m comment --comment "Drop all other traffic" -j DROP
  paused: false
```

Key points about the `CloudInit` CRD approach:

- **`matchSelector`** targets nodes by label — use `node-role.kubernetes.io/control-plane: "true"` for control-plane nodes, or `harvesterhci.io/managed: "true"` for all nodes.
- **`filename`** determines the file name under `/oem` and the alphabetical ordering of application. Using `99-` prefix ensures iptables rules are applied after other cloud-init stages.
- **`stages.boot`** runs commands at boot time, before the network is fully up, minimizing the unprotected window.
- **Reboot required** — after creating or updating the `CloudInit` resource, nodes must be rebooted for the cloud-init changes to take effect. To apply rules immediately without waiting for a reboot, also run the commands via SSH.
- **Deletion** — deleting the `CloudInit` resource removes the file from `/oem` on all matched nodes. The iptables rules remain in effect until the next reboot (when the cloud-init file is no longer present to re-apply them).

Create separate `CloudInit` resources for each node role (control-plane, worker, witness) with the appropriate rule set for that role.

### User Stories

#### Story 1 – Baseline hardening

A user wants to lock down inbound traffic on all Harvester nodes.

They read the port reference documentation, review the sample iptables rules for each node role, adjust the rules to match their network topology, and apply them via SSH on each node.

#### Story 2 – Upgrade with no behavior change

A user upgrades Harvester from v1.7 to v1.8. Since no automatic rules are shipped, all traffic continues as before. If the user had previously applied manual iptables rules, they should remove the iptables temporarily for the better upgrade flow.

**Note:** Users who have applied iptables rules (either via `CloudInit` CRD or manually) should review the updated port reference after upgrading, as new services or ports may be introduced in newer Harvester versions.

### API Changes

None.

## Design

### Implementation Overview

This proposal is documentation-only. The deliverables are:

1. **Port reference table** — A comprehensive table of all services and ports per node role (see Appendix §A and §B).
2. **Sample iptables commands** — Ready-to-use shell commands for each node role (see Appendix §E).
3. **Operational guidance** — Instructions on how to apply, verify, persist, and remove the rules, including cleanup commands, logging for debugging, and reboot persistence via the `CloudInit` CRD.
4. **Adding missing rules** — Users can identify what traffic is being dropped via the iptables log and add the corresponding rules back as needed.

No changes to any Harvester repository codebase are required. To verify which ports are currently listening, use `sudo lsof -i -P -n` or `sudo ss -tulpn`.

### Test Plan

#### Logging dropped packets (for debugging/testing)

To track which packets are being blocked, insert a LOG rule immediately before the DROP rule. This should only be enabled temporarily during testing or troubleshooting, as excessive logging can impact performance and fill up system logs. During the test, there should not be any logs.

```bash
# --log-level 4 is the warning level
iptables -I HARVESTER_INPUT <line-number-before-drop> \
  -m limit --limit 10/min \
  -j LOG --log-prefix "HARVESTER_DROP: " --log-level 4

# View dropped packets in real-time
journalctl -k -f | grep HARVESTER_DROP

# Or check recent logs
journalctl -k --since "5 minutes ago" | grep HARVESTER_DROP
```

The `--limit 10/min` option prevents log flooding by limiting log entries to 10 per minute. Adjust as needed during testing.

**Example log entry:**

When a VXLAN packet from another node is blocked (e.g., after removing the UDP 8472 rule during testing), the kernel log shows:

```
Mar 30 04:59:41 harvester1 kernel: HARVESTER_DROP: IN=mgmt-br OUT= MAC=52:54:00:ab:cd:ef:52:54:00:ab:cd:ea:08:00 SRC=192.168.122.62 DST=192.168.122.61 LEN=110 TOS=0x00 PREC=0x00 TTL=64 ID=9470 PROTO=UDP SPT=33521 DPT=8472 LEN=90
```

Key fields:
- `IN=mgmt-br` — packet arrived on the management bridge interface
- `SRC=192.168.122.62` — source IP (likely another Harvester node)
- `DST=192.168.122.61` — destination IP (this node)
- `PROTO=UDP` — protocol
- `DPT=8472` — destination port (VXLAN overlay)
- `SPT=33521` — source port (ephemeral)

This log indicates that VXLAN traffic from node `.62` to node `.61` was blocked, which would break inter-node Pod communication.

#### Interpreting drop log entries

Not every entry in the drop log indicates a missing rule. Some entries are **benign false positives** caused by conntrack state loss — for example, when iptables rules are applied manually via SSH after connections are already established. In that case, the conntrack table has no record of the existing connection, so return traffic fails the `ESTABLISHED,RELATED` check and hits the DROP rule.

Use the following fields to distinguish a benign false positive from a genuine missing rule:

| Field | Benign (conntrack state loss) | Genuine missing rule |
|-------|-------------------------------|----------------------|
| TCP flags | `ACK`, `ACK FIN`, or `ACK RST` | `SYN` (new connection) |
| `SPT` | A known service port (e.g., 9345, 6443) | Unknown or unexpected port |
| `DPT` | An ephemeral port (> 1024, random) | A protected service port |
| `SRC` | A Harvester cluster-internal IP | An external or unknown IP |

**Example of a benign drop** — return traffic from the RKE2 supervisor API on `.61` back to an ephemeral port on the witness node `.63`, triggered because the connection was established before the rules were applied:

```
Apr 22 10:00:01 harvester3 kernel: HARVESTER_DROP_IN=mgmt-br OUT= MAC=... SRC=192.168.122.61 DST=192.168.122.63 LEN=52 PROTO=TCP SPT=9345 DPT=46002 WINDOW=521 RES=0x00 ACK FIN URGP=0
```

Key indicators: `SPT=9345` (server-side port), `DPT=46002` (client ephemeral port), `ACK FIN` (connection teardown, not a new SYN). This is not a misconfiguration — it resolves on its own as connections are re-established under conntrack tracking.

When rules are applied at boot via the `CloudInit` CRD (`stages.boot`), connections are tracked from the start and this situation does not occur.

#### Features to Be Tested

- Basic E2E tests
- [Storage Network](https://docs.harvesterhci.io/v1.7/advanced/storagenetwork/)
- Guest cluster
  - At least a two-node guest cluster
- Kube-OVN
  - [Virtual Private Cloud](https://docs.harvesterhci.io/v1.7/networking/kubeovn-vpc)
- [Managed DHCP](https://docs.harvesterhci.io/v1.7/advanced/addons/managed-dhcp/)
- VM communication across the cluster
  - Ensure Flannel VXLAN and Kube-OVN VXLAN work correctly

None of the traffic generated by the above tests should appear in the iptables drop log.

### Upgrade Strategy

Before performing the upgrade, we suggest removing the iptables temporarily for the better upgrade flow.

## Note

This section lists all ports scanned by nmap before adding any iptables.

Enabled Addons:

- kubeovn-operator (Experimental)
- pcidevices-controller
- rancher-logging
- rancher-monitoring
- vm-import-controller

Some notes may be inaccurate, but they are preserved here as reported by nmap.

### Observed Port on a Two-Node Harvester cluster (Default Role)

#### External scan (`192.168.122.61` — from a separate host)

| Port | State | Service | Notes |
|------|-------|---------|-------|
| 22/tcp | open | SSH (OpenSSH 9.6) | Management access |
| 80/tcp | open | HTTP (nginx reverse proxy) | Harvester UI redirect |
| 443/tcp | open | HTTPS (nginx reverse proxy) | Harvester UI |
| 2112/tcp | open | tcpwrapped | Prometheus metrics (RKE2 component) |
| 2379/tcp | open | SSL (etcd client API) | etcd client |
| 2380/tcp | open | SSL (etcd peer) | etcd peer replication |
| 6443/tcp | open | SSL/HTTP (Golang) | Kubernetes API server |
| 6641/tcp | open | unknown | OVN Northbound DB |
| 6642/tcp | open | unknown | OVN Southbound DB |
| 6643/tcp | open | unknown | OVN (JSON-RPC echo) |
| 6644/tcp | open | unknown | OVN (JSON-RPC echo) |
| 9091/tcp | open | HTTP (Golang) | Prometheus metrics exporter |
| 9345/tcp | open | SSL/HTTP (Golang) | RKE2 supervisor API |
| 9796/tcp | open | HTTP (Prometheus Node Exporter 1.9.0) | Monitoring metrics |
| 10250/tcp | open | SSL/HTTP (Golang) | kubelet API |
| 10661/tcp | open | HTTP (Golang) | Internal component |
| 10665/tcp | open | HTTP (Golang) | Internal component |
| 31145/tcp | open | HTTP (nginx 1.18.0 Ubuntu) | NodePort — dynamically assigned |
| 31878/tcp | open | SSL/HTTP (nginx) | NodePort — dynamically assigned |
| 31933/tcp | open | HTTP (nginx reverse proxy) | NodePort — dynamically assigned |

#### External scan (`192.168.122.62` — from a separate host)

| Port | State | Service | Notes |
|------|-------|---------|-------|
| 22/tcp | open | SSH (OpenSSH 9.6) | Management access |
| 80/tcp | open | HTTP (nginx reverse proxy) | Harvester UI redirect |
| 443/tcp | open | HTTPS (nginx reverse proxy) | Harvester UI |
| 8080/tcp | open | HTTP (Golang) | Internal component |
| 8443/tcp | open | SSL/HTTP (Golang) | Harvester webhook server |
| 9091/tcp | open | HTTP (Golang) | Prometheus metrics exporter |
| 9796/tcp | open | HTTP (Prometheus Node Exporter 1.9.0) | Monitoring metrics |
| 10250/tcp | open | SSL/HTTP (Golang) | kubelet API |
| 10660/tcp | open | HTTP (Golang) | Internal component |
| 10665/tcp | open | HTTP (Golang) | Internal component |
| 31145/tcp | open | HTTP (nginx 1.18.0 Ubuntu) | NodePort — dynamically assigned |
| 31878/tcp | open | SSL/HTTP (nginx) | NodePort — dynamically assigned |
| 31933/tcp | open | HTTP (nginx reverse proxy) | NodePort — dynamically assigned |

#### Internal scan (`127.0.0.1` — from within `192.168.122.61`)

| Port | State | Service | Scope | Notes |
|------|-------|---------|-------|-------|
| 22/tcp | open | SSH (OpenSSH 9.6) | external + internal | Management access |
| 2112/tcp | open | tcpwrapped | internal only | Prometheus metrics (RKE2 component) |
| 2379/tcp | open | SSL (etcd client API) | internal only | etcd client |
| 2380/tcp | open | SSL (etcd peer) | internal only | etcd peer replication |
| 2381/tcp | open | HTTP (Golang) | internal only | etcd metrics / health |
| 2382/tcp | open | SSL | internal only | etcd additional client listener |
| 6443/tcp | open | SSL/HTTP (Golang) | external + internal | Kubernetes API server |
| 9091/tcp | open | HTTP (Golang) | internal only | Prometheus metrics exporter |
| 9099/tcp | open | HTTP (Golang) | internal only | Calico/CNI health check |
| 9345/tcp | open | SSL/HTTP (Golang) | internal only | RKE2 supervisor API |
| 9796/tcp | open | HTTP (Prometheus Node Exporter 1.9.0) | inter-node | Monitoring metrics |
| 10010/tcp | open | HTTP (Golang) | internal only | containerd / CNI plugin |
| 10248/tcp | open | HTTP (Golang) | internal only | kubelet healthz |
| 10249/tcp | open | HTTP (Golang) | internal only | kube-proxy metrics |
| 10250/tcp | open | SSL/HTTP (Golang) | internal only | kubelet API |
| 10256/tcp | open | HTTP (Golang) | internal only | kube-proxy health |
| 10257/tcp | open | SSL/HTTP (Golang) | internal only | kube-controller-manager |
| 10258/tcp | open | SSL/HTTP (Golang) | internal only | cloud-controller-manager |
| 10259/tcp | open | SSL/HTTP (Golang) | internal only | kube-scheduler |

#### Internal scan (`127.0.0.1` — from within `192.168.122.62`)

| Port | State | Service | Scope | Notes |
|------|-------|---------|-------|-------|
| 22/tcp | open | SSH (OpenSSH 9.6) | external + internal | Management access |
| 6443/tcp | open | SSL/HTTP (Golang) | internal only | Kubernetes API server (agent proxy) |
| 6444/tcp | open | SSL/HTTP (Golang) | internal only | RKE2 agent API proxy |
| 8443/tcp | open | SSL/HTTP (Golang) | external + internal | Harvester webhook server |
| 9091/tcp | open | HTTP (Golang) | external + internal | Prometheus metrics exporter |
| 9099/tcp | open | HTTP (Golang) | internal only | Calico/CNI health check |
| 9796/tcp | open | HTTP (Prometheus Node Exporter 1.9.0) | inter-node | Monitoring metrics |
| 10010/tcp | open | HTTP (Golang) | internal only | containerd / CNI plugin |
| 10248/tcp | open | HTTP (Golang) | internal only | kubelet healthz |
| 10249/tcp | open | HTTP (Golang) | internal only | kube-proxy metrics |
| 10250/tcp | open | SSL/HTTP (Golang) | external + internal | kubelet API |
| 10256/tcp | open | HTTP (Golang) | internal only | kube-proxy health |


### Observed Port on a two management nodes and one worker node Harvester cluster

#### External scan (`192.168.122.63` — from a separate host, witness)

| Port | State | Service | Notes |
|------|-------|---------|-------|
| 22/tcp | open | SSH (OpenSSH 9.6) | Management access |
| 80/tcp | open | HTTP (nginx reverse proxy) | Harvester UI redirect |
| 443/tcp | open | HTTPS (nginx reverse proxy) | Harvester UI |
| 9091/tcp | open | HTTP (Golang) | Prometheus metrics exporter |
| 9796/tcp | open | HTTP (Prometheus Node Exporter 1.9.0) | Monitoring metrics |
| 10250/tcp | open | SSL/HTTP (Golang) | kubelet API |
| 10665/tcp | open | HTTP (Golang) | kube-ovn-daemon |
| 31915/tcp | open | HTTP (nginx reverse proxy) | NodePort — dynamically assigned |
| 32224/tcp | open | SSL/HTTP (nginx) | NodePort — dynamically assigned |

#### Internal scan (`127.0.0.1` — from within `192.168.122.63`, witness)

| Port | State | Service | Scope | Notes |
|------|-------|---------|-------|-------|
| 22/tcp | open | SSH (OpenSSH 9.6) | external + internal | Management access |
| 6443/tcp | open | SSL/HTTP (Golang) | internal only | Kubernetes API server (rke2 agent proxy) |
| 6444/tcp | open | SSL/HTTP (Golang) | internal only | RKE2 agent API proxy |
| 9091/tcp | open | HTTP (Golang) | external + internal | Prometheus metrics exporter |
| 9099/tcp | open | HTTP (Golang) | internal only | Calico/CNI health check |
| 9796/tcp | open | HTTP (Prometheus Node Exporter 1.9.0) | inter-node | Monitoring metrics |
| 10010/tcp | open | HTTP (Golang) | internal only | containerd / CNI plugin |
| 10248/tcp | open | HTTP (Golang) | internal only | kubelet healthz |
| 10249/tcp | open | HTTP (Golang) | internal only | kube-proxy metrics |
| 10250/tcp | open | SSL/HTTP (Golang) | external + internal | kubelet API |
| 10256/tcp | open | HTTP (Golang) | internal only | kube-proxy health |

### Observed Port on a two management nodes and one witness node Harvester cluster

#### External scan (`192.168.122.63` — from a separate host)

| Port | State | Service | Notes |
|------|-------|---------|-------|
| 22/tcp | open | SSH (OpenSSH 9.6) | Management access |
| 2379/tcp | open | SSL (etcd client API) | etcd client |
| 2380/tcp | open | SSL (etcd peer) | etcd peer replication |
| 9091/tcp | open | HTTP (Golang) | Prometheus metrics exporter |
| 9345/tcp | open | SSL/HTTP (Golang) | RKE2 supervisor API |
| 9796/tcp | open | HTTP (Prometheus Node Exporter 1.9.0) | Monitoring metrics |
| 10250/tcp | open | SSL/HTTP (Golang) | kubelet API |
| 10665/tcp | open | HTTP (Golang) | kube-ovn-daemon |
| 31915/tcp | open | HTTP (nginx reverse proxy) | NodePort — dynamically assigned |
| 32224/tcp | open | SSL/HTTP (nginx) | NodePort — dynamically assigned |

#### Internal scan (`127.0.0.1` — from within `192.168.122.63`)

| Port | State | Service | Scope | Notes |
|------|-------|---------|-------|-------|
| 22/tcp | open | SSH (OpenSSH 9.6) | external + internal | Management access |
| 2379/tcp | open | SSL (etcd client API) | external + internal | etcd client |
| 2380/tcp | open | SSL (etcd peer) | external + internal | etcd peer replication |
| 2381/tcp | open | HTTP (Golang) | internal only | etcd metrics / health |
| 2382/tcp | open | SSL | internal only | etcd additional client listener |
| 6443/tcp | open | SSL/HTTP (Golang) | internal only | Kubernetes API server (rke2 agent proxy) |
| 6444/tcp | open | SSL/HTTP (Golang) | internal only | RKE2 agent API proxy |
| 9091/tcp | open | HTTP (Golang) | external + internal | Prometheus metrics exporter |
| 9099/tcp | open | HTTP (Golang) | internal only | Calico/CNI health check |
| 9345/tcp | open | SSL/HTTP (Golang) | external + internal | RKE2 supervisor API |
| 9796/tcp | open | HTTP (Prometheus Node Exporter 1.9.0) | inter-node | Monitoring metrics |
| 10010/tcp | open | HTTP (Golang) | internal only | containerd / CNI plugin |
| 10248/tcp | open | HTTP (Golang) | internal only | kubelet healthz |
| 10249/tcp | open | HTTP (Golang) | internal only | kube-proxy metrics |
| 10250/tcp | open | SSL/HTTP (Golang) | external + internal | kubelet API |
| 10256/tcp | open | HTTP (Golang) | internal only | kube-proxy health |
| 10258/tcp | open | SSL/HTTP (Golang) | internal only | cloud-controller-manager |

## Different roles for different iptables

#### Control-plane node

```yaml
apiVersion: node.harvesterhci.io/v1beta1
kind: CloudInit
metadata:
  name: iptables-control-plane
spec:
  matchSelector:
    node-role.kubernetes.io/control-plane: "true"
    node-role.kubernetes.io/etcd: "true"
  filename: 99-iptables-control-plane
  contents: |
    stages:
      boot:
        - name: "Apply iptables rules for control-plane node"
          commands:
            - iptables -D INPUT -i mgmt-br -j HARVESTER_INPUT 2>/dev/null || true
            - iptables -F HARVESTER_INPUT 2>/dev/null || true
            - iptables -X HARVESTER_INPUT 2>/dev/null || true
            - iptables -N HARVESTER_INPUT
            - iptables -I INPUT -i mgmt-br -j HARVESTER_INPUT
            - iptables -A HARVESTER_INPUT -m conntrack --ctstate ESTABLISHED,RELATED -m comment --comment "Accept return traffic from established connections" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p icmp -m comment --comment "ICMP (ping, MTU discovery, etc.)" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --sport 67 --dport 68 -m comment --comment "DHCP server reply" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --sport 68 --dport 67 -m comment --comment "DHCP client request" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 22 -m comment --comment "SSH" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 80 -m comment --comment "Harvester UI HTTP" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 443 -m comment --comment "Harvester UI HTTPS" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 2112 -m comment --comment "RKE2 kube-vip Prometheus metrics" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 2379:2380 -m comment --comment "etcd client/peer" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 6443 -m comment --comment "Kubernetes API server" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 6641:6644 -m comment --comment "OVN NB/SB DB and JSON-RPC" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 8080 -m comment --comment "kube-ovn-webhook HTTP" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 8443 -m comment --comment "kube-ovn-webhook HTTPS" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 9091 -m comment --comment "calico-node metrics endpoint (Prometheus format)" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 9345 -m comment --comment "RKE2 supervisor API" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 9796 -m comment --comment "Prometheus node-exporter metrics" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 10250 -m comment --comment "kubelet API" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 10661 -m comment --comment "kube-ovn-monitor metrics" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 10665 -m comment --comment "kube-ovn-daemon" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 30000:32767 -m comment --comment "NodePort TCP range" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --dport 30000:32767 -m comment --comment "NodePort UDP range" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --dport 8472 -m comment --comment "VXLAN Flannel/Canal" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --dport 4789 -m comment --comment "VXLAN Kube-OVN" -j ACCEPT
            - iptables -A HARVESTER_INPUT -m limit --limit 10/min -m comment --comment "Rate-limit logging of dropped packets" -j LOG --log-prefix HARVESTER_DROP_ --log-level 4
            - iptables -A HARVESTER_INPUT -m comment --comment "Drop all other traffic" -j DROP
  paused: false
```

#### Worker node

```yaml
apiVersion: node.harvesterhci.io/v1beta1
kind: CloudInit
metadata:
  name: iptables-worker
spec:
  matchSelector:
    node-role.harvesterhci.io/worker: "true"
  filename: 99-iptables-worker
  contents: |
    stages:
      boot:
        - name: "Apply iptables rules for worker node"
          commands:
            - iptables -D INPUT -i mgmt-br -j HARVESTER_INPUT 2>/dev/null || true
            - iptables -F HARVESTER_INPUT 2>/dev/null || true
            - iptables -X HARVESTER_INPUT 2>/dev/null || true
            - iptables -N HARVESTER_INPUT
            - iptables -I INPUT -i mgmt-br -j HARVESTER_INPUT
            - iptables -A HARVESTER_INPUT -m conntrack --ctstate ESTABLISHED,RELATED -m comment --comment "Accept return traffic from established connections" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p icmp -m comment --comment "ICMP (ping, MTU discovery, etc.)" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --sport 67 --dport 68 -m comment --comment "DHCP server reply" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --sport 68 --dport 67 -m comment --comment "DHCP client request" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 22 -m comment --comment "SSH" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 80 -m comment --comment "Harvester UI HTTP" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 443 -m comment --comment "Harvester UI HTTPS" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 8080 -m comment --comment "kube-ovn-webhook HTTP" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 8443 -m comment --comment "kube-ovn-webhook HTTPS" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 9091 -m comment --comment "calico-node metrics endpoint (Prometheus format)" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 9796 -m comment --comment "Prometheus node-exporter metrics" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 10250 -m comment --comment "kubelet API" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 10660 -m comment --comment "kube-ovn-controller" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 10665 -m comment --comment "kube-ovn-daemon" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 30000:32767 -m comment --comment "NodePort TCP range" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --dport 30000:32767 -m comment --comment "NodePort UDP range" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --dport 8472 -m comment --comment "VXLAN Flannel/Canal" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --dport 4789 -m comment --comment "VXLAN Kube-OVN" -j ACCEPT
            - iptables -A HARVESTER_INPUT -m limit --limit 10/min -m comment --comment "Rate-limit logging of dropped packets" -j LOG --log-prefix HARVESTER_DROP_ --log-level 4
            - iptables -A HARVESTER_INPUT -m comment --comment "Drop all other traffic" -j DROP
  paused: false
```

#### Witness node

```yaml
apiVersion: node.harvesterhci.io/v1beta1
kind: CloudInit
metadata:
  name: iptables-witness
spec:
  matchSelector:
    node-role.harvesterhci.io/witness: "true"
  filename: 99-iptables-witness
  contents: |
    stages:
      boot:
        - name: "Apply iptables rules for witness node"
          commands:
            - iptables -D INPUT -i mgmt-br -j HARVESTER_INPUT 2>/dev/null || true
            - iptables -F HARVESTER_INPUT 2>/dev/null || true
            - iptables -X HARVESTER_INPUT 2>/dev/null || true
            - iptables -N HARVESTER_INPUT
            - iptables -I INPUT -i mgmt-br -j HARVESTER_INPUT
            - iptables -A HARVESTER_INPUT -m conntrack --ctstate ESTABLISHED,RELATED -m comment --comment "Accept return traffic from established connections" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p icmp -m comment --comment "ICMP (ping, MTU discovery, etc.)" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --sport 67 --dport 68 -m comment --comment "DHCP server reply" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --sport 68 --dport 67 -m comment --comment "DHCP client request" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 22 -m comment --comment "SSH" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 2379:2380 -m comment --comment "etcd client/peer" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 6641:6644 -m comment --comment "OVN NB/SB DB and JSON-RPC" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 9091 -m comment --comment "calico-node metrics endpoint (Prometheus format)" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 9345 -m comment --comment "RKE2 supervisor API" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 9796 -m comment --comment "Prometheus node-exporter metrics" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 10250 -m comment --comment "kubelet API" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p tcp --dport 10665 -m comment --comment "kube-ovn-daemon" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --dport 8472 -m comment --comment "VXLAN Flannel/Canal" -j ACCEPT
            - iptables -A HARVESTER_INPUT -p udp --dport 4789 -m comment --comment "VXLAN Kube-OVN" -j ACCEPT
            - iptables -A HARVESTER_INPUT -m limit --limit 10/min -m comment --comment "Rate-limit logging of dropped packets" -j LOG --log-prefix HARVESTER_DROP_ --log-level 4
            - iptables -A HARVESTER_INPUT -m comment --comment "Drop all other traffic" -j DROP
  paused: false
```

## Appendix — Port Reference Tables

### Control-plane Node

| Protocol | Port | Bind Address | Source | Description |
|----------|------|--------------|--------|-------------|
| TCP | 22 | `0.0.0.0` | All | SSH |
| TCP | 80 | `0.0.0.0` | All | Harvester UI HTTP (nginx proxy) |
| TCP | 443 | `0.0.0.0` | All | Harvester UI HTTPS (nginx proxy) |
| TCP | 2112 | `0.0.0.0` | All | kube-vip Prometheus metrics |
| TCP | 2379 | `127.0.0.1` + node IP | Harvester management nodes | etcd client port |
| TCP | 2380 | `127.0.0.1` + node IP | Harvester management nodes | etcd peer port |
| TCP | 2381 | `127.0.0.1` only | localhost | etcd metrics/health — no iptables rule needed |
| TCP | 2382 | `127.0.0.1` only | localhost | etcd learner client (HTTP) — no iptables rule needed |
| TCP | 6443 | `0.0.0.0` | All | Kubernetes API server |
| TCP | 6641 | node IP | Harvester nodes | OVN Northbound DB |
| TCP | 6642 | node IP | Harvester nodes | OVN Southbound DB |
| TCP | 6643 | node IP | Harvester nodes | OVN JSON-RPC |
| TCP | 6644 | node IP | Harvester nodes | OVN JSON-RPC |
| TCP | 8080 | node IP | Harvester nodes | kube-ovn-webhook HTTP |
| TCP | 8443 | `0.0.0.0` | All | kube-ovn-webhook HTTPS |
| TCP | 9091 | `0.0.0.0` | All | calico-node metrics endpoint (Prometheus format) |
| TCP | 9099 | `127.0.0.1` only | localhost | Calico/CNI health check — no iptables rule needed |
| TCP | 9345 | `0.0.0.0` | Harvester nodes | RKE2 supervisor API |
| TCP | 9796 | `0.0.0.0` | All | Prometheus node-exporter |
| TCP | 10010 | `127.0.0.1` only | localhost | containerd gRPC — no iptables rule needed |
| TCP | 10248 | `127.0.0.1` only | localhost | kubelet healthz — no iptables rule needed |
| TCP | 10249 | `127.0.0.1` only | localhost | kube-proxy metrics — no iptables rule needed |
| TCP | 10250 | `0.0.0.0` | Kubernetes components | kubelet API |
| TCP | 10256 | `127.0.0.1` only | localhost | kube-proxy health — no iptables rule needed |
| TCP | 10257 | `127.0.0.1` only | localhost | kube-controller-manager — no iptables rule needed |
| TCP | 10258 | `127.0.0.1` only | localhost | cloud-controller-manager — no iptables rule needed |
| TCP | 10259 | `127.0.0.1` only | localhost | kube-scheduler — no iptables rule needed |
| TCP | 10661 | node IP | Harvester nodes | kube-ovn-monitor metrics |
| TCP | 10665 | node IP | Harvester nodes | kube-ovn-daemon |
| TCP | 30000-32767 | `0.0.0.0` | All | NodePort services (TCP) |
| UDP | 4789 | `0.0.0.0` | Harvester nodes | VXLAN (Kube-OVN) |
| UDP | 8472 | `0.0.0.0` | Harvester nodes | VXLAN (Flannel/Canal) |
| UDP | 30000-32767 | `0.0.0.0` | All | NodePort services (UDP) |

### Worker Node

| Protocol | Port | Bind Address | Source | Description |
|----------|------|--------------|--------|-------------|
| TCP | 22 | `0.0.0.0` | All | SSH |
| TCP | 80 | `0.0.0.0` | All | Harvester UI HTTP (nginx proxy) |
| TCP | 443 | `0.0.0.0` | All | Harvester UI HTTPS (nginx proxy) |
| TCP | 6443 | `127.0.0.1` + `[::1]` only | localhost | Kubernetes API server (rke2 agent proxy) — no iptables rule needed |
| TCP | 6444 | `127.0.0.1` + `[::1]` only | localhost | RKE2 agent API proxy — no iptables rule needed |
| TCP | 8080 | node IP | Harvester nodes | kube-ovn-webhook HTTP |
| TCP | 8443 | `0.0.0.0` | All | kube-ovn-webhook HTTPS |
| TCP | 9091 | `0.0.0.0` | All | calico-node metrics endpoint (Prometheus format) |
| TCP | 9099 | `127.0.0.1` only | localhost | Calico/CNI health check — no iptables rule needed |
| TCP | 9796 | `0.0.0.0` | All | Prometheus node-exporter |
| TCP | 10010 | `127.0.0.1` only | localhost | containerd gRPC — no iptables rule needed |
| TCP | 10248 | `127.0.0.1` only | localhost | kubelet healthz — no iptables rule needed |
| TCP | 10249 | `127.0.0.1` only | localhost | kube-proxy metrics — no iptables rule needed |
| TCP | 10250 | `0.0.0.0` | Kubernetes components | kubelet API |
| TCP | 10256 | `127.0.0.1` only | localhost | kube-proxy health — no iptables rule needed |
| TCP | 10660 | node IP | Harvester nodes | kube-ovn-controller |
| TCP | 10665 | node IP | Harvester nodes | kube-ovn-daemon |
| TCP | 30000-32767 | `0.0.0.0` | All | NodePort services (TCP) |
| UDP | 4789 | `0.0.0.0` | Harvester nodes | VXLAN (Kube-OVN) |
| UDP | 8472 | `0.0.0.0` | Harvester nodes | VXLAN (Flannel/Canal) |
| UDP | 30000-32767 | `0.0.0.0` | All | NodePort services (UDP) |

### Witness Node

| Protocol | Port | Bind Address | Source | Description |
|----------|------|--------------|--------|-------------|
| TCP | 22 | `0.0.0.0` | All | SSH |
| TCP | 2379 | `127.0.0.1` + node IP | Harvester management nodes | etcd client port |
| TCP | 2380 | `127.0.0.1` + node IP | Harvester management nodes | etcd peer port |
| TCP | 2381 | `127.0.0.1` only | localhost | etcd metrics/health — no iptables rule needed |
| TCP | 2382 | `127.0.0.1` only | localhost | etcd learner client (HTTP) — no iptables rule needed |
| TCP | 6443 | `127.0.0.1` only | localhost | Kubernetes API server (rke2 agent proxy) — no iptables rule needed |
| TCP | 6444 | `127.0.0.1` only | localhost | RKE2 agent API proxy — no iptables rule needed |
| TCP | 6641 | node IP | Harvester nodes | OVN Northbound DB |
| TCP | 6642 | node IP | Harvester nodes | OVN Southbound DB |
| TCP | 6643 | node IP | Harvester nodes | OVN JSON-RPC |
| TCP | 6644 | node IP | Harvester nodes | OVN JSON-RPC |
| TCP | 9091 | `0.0.0.0` | All | calico-node metrics endpoint (Prometheus format) |
| TCP | 9099 | `127.0.0.1` only | localhost | Calico/CNI health check — no iptables rule needed |
| TCP | 9345 | `0.0.0.0` | Harvester nodes | RKE2 supervisor API |
| TCP | 9796 | `0.0.0.0` | All | Prometheus node-exporter |
| TCP | 10010 | `127.0.0.1` only | localhost | containerd gRPC — no iptables rule needed |
| TCP | 10248 | `127.0.0.1` only | localhost | kubelet healthz — no iptables rule needed |
| TCP | 10249 | `127.0.0.1` only | localhost | kube-proxy metrics — no iptables rule needed |
| TCP | 10250 | `0.0.0.0` | Kubernetes components | kubelet API |
| TCP | 10256 | `127.0.0.1` only | localhost | kube-proxy health — no iptables rule needed |
| TCP | 10258 | `127.0.0.1` only | localhost | cloud-controller-manager — no iptables rule needed |
| TCP | 10665 | node IP | Harvester nodes | kube-ovn-daemon |
| UDP | 4789 | `0.0.0.0` | Harvester nodes | VXLAN (Kube-OVN) |
| UDP | 8472 | `0.0.0.0` | Harvester nodes | VXLAN (Flannel/Canal) |

## Addon Port Listening Reference

The following tables show the **new ports** introduced by each addon, compared against the baseline (no addons enabled). Data collected via `sudo ss -tulpn` on a two management nodes + one witness node cluster.

### kubeovn-operator (Experimental)

#### Management Node (control-plane)

| Protocol | Address | Port | Process | Description |
|----------|---------|------|---------|-------------|
| UDP | 0.0.0.0 | 4789 | — | VXLAN tunnel (Kube-OVN overlay) |
| UDP | [::] | 4789 | — | VXLAN tunnel (Kube-OVN overlay, IPv6) |
| TCP | node IP | 10665 | kube-ovn-daemon | Kube-OVN daemon metrics/API |
| TCP | node IP | 10660 | kube-ovn-controller | Kube-OVN controller metrics |
| TCP | node IP | 6641 | ovsdb-server | OVN Northbound DB |
| TCP | node IP | 6642 | ovsdb-server | OVN Southbound DB |
| TCP | node IP | 6643 | ovsdb-server | OVN JSON-RPC |
| TCP | node IP | 6644 | ovsdb-server | OVN JSON-RPC |

#### Witness Node

| Protocol | Address | Port | Process | Description |
|----------|---------|------|---------|-------------|
| UDP | 0.0.0.0 | 4789 | — | VXLAN tunnel (Kube-OVN overlay) |
| UDP | [::] | 4789 | — | VXLAN tunnel (Kube-OVN overlay, IPv6) |
| TCP | node IP | 10665 | kube-ovn-daemon | Kube-OVN daemon metrics/API |

### rancher-monitoring

#### Management Node (control-plane)

| Protocol | Address | Port | Process | Description |
|----------|---------|------|---------|-------------|
| TCP | * | 9796 | node_exporter | Prometheus Node Exporter metrics |

#### Witness Node

| Protocol | Address | Port | Process | Description |
|----------|---------|------|---------|-------------|
| TCP | * | 9796 | node_exporter | Prometheus Node Exporter metrics |

### rancher-logging

None.