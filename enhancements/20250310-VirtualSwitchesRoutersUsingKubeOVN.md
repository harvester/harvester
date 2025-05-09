# Support Virtual Switches and Routers in Harvester

Support Virtual Switches and Routers in Harvester using KubeOVN https://kubeovn.github.io/docs/v1.12.x/en/

## Summary

- Harvester VMs are isolated using vlans which requires changes at hardware level at the external switches for the correct tagging of vlans.
- Harvester uses DHCP server externally to allocate IP addresses to VM interfaces per vlan.
- Harvester does not provide Layer3 connectivity between VMs at the software level.
- Harvester uses Linux bridges which are limited in functionality compared to OVS bridges for connecting VMs internally.

Since isolation of VMs using above design is more tied to the physical infrastructure and does not scale well, it is important to bring in support for logical switches and routers for isolating the VMs in Harvester.

### Related Issues

[The URL For the related enhancement issues in Harvester repo.](https://github.com/harvester/harvester/issues/7332)

## Motivation

### Goals

- Add KubeOVN as addon and integrate with Harvester
- Use Subnets from KubeOVN to achieve configuration of virtual switches
- Use VPCs from KubeOVN to achieve configuration of virtual Routers
- Eliminate the need for DHCP servers at the external switches to allocate IP addresses to the VMs
- Use OVS bridges in the data plane for communication between the VMs

### Non-goals [optional]

## Proposal

- Bring in the support for Software Defined Networking using KubeOVN which separates control plane and data plane enabling greater automation and programmability 
  in the network.
- Providing logical network abstraction on top of physical infrastructure
- Using OVN(Open Virtual Networking) at the control plane level for programming virtual switches and routers and using OVS(Open Virtual Switch) at the data plane for 
  traffic forwarding

### User Stories
Detail the things that people will be able to do if this enhancement is implemented. A good practise is including a comparsion of what user cannot do before the enhancement implemented, why user would want an enhancement and what user need to do after, to make it clear why the enhancement beneficial to the user.

The experience details should be in the `User Experience In Detail` later.

#### KubeOVN Components

- OVN Central: Control plane component running in the cluster contains ovn-northd, responsible for managing the config from Cloud Management System.

- OVN-Controller: Runs per node. Adds virtual interfaces to the OVS bridge.Adds openFlow entries for packet forwarding.

- OVN-Monitor, OVN-Pinger: Monitoring tools

- OVN-CNI:  Takes care of container networking in ovn.

- OVS-OVN: Data plane component responsible for packet forwarding through openFlow entries.

#### Subnets
- Custom subnets can be created by users to create virtual switches in the network for Layer2 Connectivity

#### VPCs
- Custom VPCs can be created by users to create virtual routers in the network for Layer3 connectivity

#### Steps:
- Create a Network Attachment Defintion as type KubeOVN (UI to introduce NAD creation without vlan-id and cluster network config and use only type kube-ovn)
- Create a vpc if Layer3 connectivity is required between two or more subnets
- Apply the vpc configuration
- Create a subnet specifying the ip subnet range and using the NAD's name and namespace as provider and provide a vpc name if Layer3 connectivity is required
- Apply the subnet configuration
- Create VMs using the NAD created in step 1, IPs allocated from subnet using the NAD
- Add Routes to the VMs if using subnets connected to a VPC
- Test Ping between VMs within same subnet
- Test Ping between VMs within different subnets connected via VPC

#### Verify succesful Layer2 connectivity between VMs using same subnet running on the same node

#### Verify succesful Layer3 connectivity between VMs using different subnets running on the same node

#### Verify succesful Layer2 connectivity between VMs using same subnet  running on different nodes

#### Verify succesful Layer3 connectivity between VMs using different subnets running on different nodes

#### Verify connectivity fails between VMs isolated via different subnets within same/different nodes
- Subnets used by VMs are not connected to any VPCs

#### Verify connectivity between VMs through VPC Peering
- Create subnet1,subnet2,vpc1 and vpc2
- Attach subnet1 to vpc1
- Attach subnet2 to vpc2
- Add static route in VPC1 to reach VPC2
- Add static route in VPC2 to reach VPC1
- Spin up VM1 using subnet1 NAD and VM2 using subnet2 NAD
- Verify ping succeeds between VM1 and VM2

#### Verify VM isolation using same CIDR Block in different namespaces
- Create subnet1 in vpc1 in namespace1 with cidr block 10.0.1.0/24
- Create subnet2 in vpc2 in namespace2 with same cidr 10.0.1.0/24
- Spin up VMs using subnet1 in namespace1
- Sping up VMs using subnet2 in namespace2
- Verify connectivity succeeds between VMs in same namespace
- Verify connectivity fails between VMs in same namespace

#### Verify external connectivity from VM (TBD)

### API changes

## Design

### Implementation Overview

#### Creation of Network Attachment Definition with type kube-ovn

- Since Vlans are eliminated in this model, NADs should be created with type "kube-ovn".
- Harvester network controller changes:
  - nad webhook must be modified to accept type "kube-ovn" and without any bridge name.
  - nad controller updates the nad cache with new type as kube-ovn
  - nad controller logic must skip processing related to vlan and checking L3 network connectivity from DHCP.

  ```yaml
  apiVersion: k8s.cni.cncf.io/v1
  kind: NetworkAttachmentDefinition
  metadata:
    annotations:
      network.harvesterhci.io/route: '{"mode":"auto","serverIPAddr":"","cidr":"","gateway":""}'
    name: vswitch1
    namespace: default
    resourceVersion: '905524'
    uid: 097c3d0f-e3e2-43a9-a4c9-933ff89c505b
  spec:
  config: >-
    {"cniVersion":"0.3.1","name":"vswitch1","type":"kube-ovn","server_socket":
    "/run/openvswitch/kube-ovn-daemon.sock", "provider": "vswitch1.default.ovn"}
  ```
#### Creation of Virtual Switches/Subnets

- KubeOVN provides CRD called subnet for creation of virtual switches.
- Harvester GUI must add changes to facilitate the configuration of virtual switches from user.
  [ Fields required - name,namespace,protocol,provider,vpc,subnet range,gateway ip, exclude range]
- Harvester Network Controller changes:
  - introduce new webhook for ovn subnet to validate the existence of NAD,VPC and validate the CIDR ip block used by Subnet spec
  - introduce new controller for ovn subnet to include kubeovn apis and clients and add/update/delete the cache with user configured values
    https://github.com/kubeovn/kube-ovn/blob/master/pkg/apis/kubeovn/v1/subnet.go
    https://github.com/kubeovn/kube-ovn/blob/master/pkg/client/clientset/versioned/typed/kubeovn/v1/subnet.go

  ```yaml
  apiVersion: kubeovn.io/v1
  kind: Subnet
  metadata:
    name: vswitch1
  spec:
    protocol: IPv4
    provider: vswitch1.default.ovn
    vpc: commonvpc
    cidrBlock: 172.20.0.0/16
    gateway: 172.20.0.1
  ```

#### Creation of Virtual Routers/VPCs

- KubeOVN provides CRD called VPCs for creation of virtual routers.
- Harvester GUI must add changes to facilitate the configuration of virtual routers from user.
  [Fields - name is mandatory, user could also specify static routes and policy routes but optional]
- Harvester network controller changes
  - introduce new controller for ovn vpc to include kubeovn apis and clients and add/update/delete the cache with user configured values
    https://github.com/kubeovn/kube-ovn/blob/master/pkg/apis/kubeovn/v1/vpc.go
    https://github.com/kubeovn/kube-ovn/blob/master/pkg/client/clientset/versioned/typed/kubeovn/v1/vpc.go

  ```yaml
  kind: Vpc
  apiVersion: kubeovn.io/v1
  metadata:
    name: commonvpc
  ```
#### Creation of Virtual Machines:

- Creation of Virtual Machines can be done in the usual way selecting the NADs created using type kube-ovn.
- Secondary interfaces of VMs are now part of particular logical network.
- Routes must be configured on the VMs by the user manually or using cloud init for which Layer3 connectivity is required.

Example: VM1 using subnet vswitch1 (172.20.0.0/16) and VM2 using subnet vswitch2(172.30.0.0/16), both connected via router commonvpc.

VM1 guest OS Routes:

172.30.0.0/16 dev enp1s0 proto kernel scope link src 172.20.0.1

VM2 guest OS Routes:

172.20.0.0/16 dev enp1s0 proto kernel scope link src 172.30.0.1

### Test plan

Integration test plan.

### Upgrade strategy


## Note

Currently Harvester has IP addresses configured only on mgmt interfaces, since underlay Layer3 connectivity is required for communication between VMs running on different nodes with this model, same physical nic part of mgmt-bo is shared for VM traffic as well.This will impose traffic overhead as there is no segregation of VM traffic on a separate physical link.

In future, the above limitation should be taken care by facilitating the configuration of IP Addresses on user configured cluster networks and utilizing them as underlay for VMs part of virtual switches and routers.

