# Support for Network Policies in Harvester

Support Network Policies in Harvester using KubeOVN for microsegmentation of VM workloads

## Summary

- Traffic Isolation:
  - Network policies help to isolate VMs and allow access only between certain VMs bases on rules.
  - Policies allow only specific VMs to communicate with each other, reducing the attack surface. This is especially critical in multi-tenant environments or 
    cloud infrastructures.
  - Network policies allow you to define and enforce rules for controlling inbound and outbound traffic, ensuring that only legitimate traffic is allowed, 
    and malicious or unnecessary traffic is blocked.

- Micro-Segmentation:
  - Network policies allow for granular control over which VMs can communicate with each other, even within the same subnet
  - Containment of Breaches: If a VM is compromised, micro-segmentation limits the attacker's ability to move laterally within the network. Each VM or group 
    of VMs can be isolated, reducing the potential spread of a security breach.
  - Granular Access Control: Security policies can be applied at a finer level, ensuring that VMs only communicate with other VMs that they need to interact 
    with, minimizing the risk of unauthorized access.

### Related Issues

[The URL For the related enhancement issues in Harvester repo.](https://github.com/harvester/harvester/issues/7381)

## Motivation
Need for microsegmentation between VMs

VM isolation could be achieved using using VLANs traditionally or using virtual switches in KubeOVN.
But implementing network policies allow micro segmentation of VMs by isolating VMs within the same L2/Virtual Switch.

### Goals

Users will be able to restrict access between VMs within same logical network
- match [direction,srcip,dstip,ip protocol,application port numbers]
- action [ drop,allow]
- Isolate subnets/Layer2 from other subnets

### Non-goals

## Proposal

- Use KubeOVN's subnet ACL to apply ACL rules per subnet
- Use K8s Network Policy to allow/deny traffic to a particular VM 

### User Stories

#### Isolate traffic between VMs within same subnet/vswitch using subnet ACLs
- Deny traffic between VMs using srcip,dstip
- Deny traffic between VMs using direction to-lport,from-lport
- Deny traffic between VMs using higher priority rule
- Deny traffic between VMs using srcport,dstport
- Deny traffic between VMs using ip protocol type

#### Isolate traffic between subnets used by VMs
- Deny traffic to a subnet by toggle private to true in that subnet
- Allow specific subnet to communicate by adding subnet ranges to allowSubnets

#### Allow traffic between only certain VMs within the same subnet
- Create VMs with labels
- Create K8s Network Policy matching cidr using ip-block and using labels as pod selectors
- The policy is applied to VMs only matching the selectors

#### Isolate traffic between VMs by vxlan network
- Deny/Allow traffic from specific subnets
- Deny/Allow traffic from specific tunnel endpoints
- Deny/Allow traffic matching vxlan headers
- Deny/Allow traffic matching vxlan udp port number

### User Experience In Detail
Users will be able to isolate VMs within the same Layer2 network by specifying various packet matching rules per subnet/logical switch
or create K8s Network Policy with Pod selectors to apply network policies only to certain pods/VMs

### API changes

## Design

### Implementation Overview

Harvester Network controller to integrate subnets apis/clients from kubeovn
https://github.com/kubeovn/kube-ovn/blob/master/pkg/apis/kubeovn/v1/subnet.go
https://github.com/kubeovn/kube-ovn/blob/master/pkg/client/clientset/versioned/typed/kubeovn/v1/subnet.go

- GUI to configure policy options with match [direction,srcip,dstip,ip protocol,application port numbers]
  and action [drop,allow] on a specific subnet configuration
- subnet controller in Harvester network controller should update the subnet spec acls list with user configured match/action

```yaml
apiVersion: kubeovn.io/v1
kind: Subnet
metadata:
  name: acl
spec:
  allowEWTraffic: false
  acls:
    - action: drop
      direction: to-lport
      match: ip4.dst == 10.10.0.2 && ip
      priority: 1002
    - action: allow-related
      direction: from-lport
      match: ip4.src == 10.10.0.2 && ip
      priority: 1002
  cidrBlock: 10.10.0.0/24
```
- GUI to configure toggle true/false under subnet private option and list allow CIDR
- subnet controller in Harvester network controller should update the subnet spec with user configured values
  for private and allow CIDR

  ```yaml
  apiVersion: kubeovn.io/v1
  kind: Subnet
  metadata:
    name: private
  spec:
    protocol: IPv4
    default: false
    namespaces:
    - ns1
    - ns2
    cidrBlock: 10.69.0.0/16
    private: true
    allowSubnets:
    - 10.16.0.0/16
    - 10.18.0.0/16
  ```
- GUI to configure K8s network policy
  [Fields - match Pod selector,direction,cidr]
- K8s NetworkPolicy is implicit deny and explicit rules have to be added for allowing traffic.
- Using a Pod selector with Network Policy and matching rules will allow only those matching traffic to the pod, other traffic will be dropped

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: ip-block
spec:
  ingress:
    - from:
        - ipBlock:
            cidr: 172.20.2.0/30
        - ipBlock:
            cidr: 10.52.0.0/16
  egress:
    - to:
        - ipBlock:
            cidr: 172.20.2.0/30
        - ipBlock:
            cidr: 10.52.0.0/16
```
- Pod Selector: The set of pods the policy applies to.
- Ingress & Egress: Defines the inbound (Ingress) and outbound (Egress) traffic rules.
- Policy Types: There are two main policy types:
  Ingress: Controls the incoming traffic to the selected pods.
  Egress: Controls the outgoing traffic from the selected pods.
- Namespaces: Network policies to restrict traffic between pods in different namespaces.

- Network Policy CRDs are available from networkpolicies.crd.projectcalico.org in Harvester
- Integrate network policy apis from projectcalico into harvester network controller
  https://github.com/projectcalico/calico/blob/master/api/pkg/apis/projectcalico/v3/networkpolicy.go

### Test plan

### Upgrade strategy

## Note
