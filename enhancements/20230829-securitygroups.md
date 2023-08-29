# Harvester SecurityGroups

## Summary

This proposal introduces the new SecurityGroup(SG) CRD to Harvester. The SG can be used to define ingress rules to control access to VM workloads from specific source IP's to all or specific destination ports on the VM.

### Related Issues

[GH issue 2260 ](https://github.com/harvester/harvester/issues/2260)

## Motivation

### Goals

* Allow filtering of traffic to VM's based on a combination of following:
    * source address
    * destination ports
    * protocol (tcp/udp/icmp)


## Proposal

### User Stories

#### Restrict network access to VMs

### User Experience In Detail

End users would like to restrict traffic to a particular VM based on a combination or Source IP, destination ports and network layer transports like tcp/udp/icmp.

### API changes
A new SecurityGroup CRD is introduced to allow users to define source addresses allowed to connect to destination ports on VM.

```go
type SecurityGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              []IngressRules `json:"allowIngressSpec,omitempty"`
}

type IngressRules struct {
	SourceAddress   string   `json:"sourceAddress"`
	SourcePortRange []uint32 `json:"ports,omitempty"`
	// +kubebuilder:validation:Enum={"tcp","udp","icmp","icmpv6"}
	IPProtocol string `json:"ipProtocol"`
}
```
## Design

### Implementation Overview

The implementation involves the following changes to harvester:

* **SecurityGroup CRD**: users can define `ingressRules` to control access to a VM workload. A sample would be as follows:

```yaml

apiVersion: harvesterhci.io/v1beta1
kind: SecurityGroup
metadata:
  name: sample-ingress
  namespace: default
allowIngressSpec:
- ipProtocol: tcp
  ports:
  - 22
  - 80
  sourceAddress: 172.19.107.9
- ipProtocol: icmp
  sourceAddress: 172.19.107.9  
```

Users can attach a `SecurityGroup` to a VM using annotations

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  annotations:
    harvesterhci.io/attachedSecurityGroup: sample-ingress
  labels:
    harvesterhci.io/creator: harvester
    harvesterhci.io/os: ubuntu
  name: ubuntu
  namespace: default
spec: ...VMSPEC
```
* **vm-network-policy sidecar**: The vm-network-policy sidecar, watches the `SecurityGroup` attached to the VM and applies `iptables` forwarding rules to the tap interfaces associated with VM bridge interfaces.

  Any changes to the `SecurityGroup` are reconciled to corresponding `iptables` rules. This allows for dynamic configuration of the rules without rebooting the VMs.

* **pod mutation webhook**: Additional logic in the harvester pod mutation webhook. The logic, checks the following:
  * the pod being created is a virt-launcher pod
  * the VM associated with the virt-launcher pod has a `SecurityGroup` attached
  
  When these conditions are met, the mutator patches the virt-launcher pod to run the additional `vm-network-policy` sidecar. The sidecar definition also contains information about the VM the launcher pod is associated with.

* **namespace controller**: The namespace controller watches non-system namespaces and creates a new rolebinding with the `default` service account to allow account to view `SecurityGroup` definitions

```yaml
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: harvester-sidecar
rules:
  - apiGroups: ["harvesterhci.io"]
    resources: ["securitygroups"]
    verbs: ["get", "watch", "list"]
  - apiGroups: ["kubevirt.io"]
    resources: ["virtualmachines"]
    verbs: ["get", "watch", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: default-vmnetworkpolicy
  namespace: default
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: harvester-sidecar
subjects:
  - kind: ServiceAccount
    name: default
    namespace: default
```
  
### Test plan

* **VMs without a securitygroup**: end users should be able to access all services/ports from any source address. This would be the current behaviour.
* **VMs with securitygroup**: end users should only be able to access a predefined list of ports/services from a list of whitelisted source addresses. This will work the same irrespective of the guest OS.

### Upgrade strategy

No further action is needed, as the `harvester` crd chart will include the CRD definition for `SecurityGroup`

