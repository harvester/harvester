# Title

System Robustness Enhancement

## Summary

Harvester will enhance from different layers of the whole tech stack to make sure the cluster is more robust in such scenarios:

- Frequrent power-off and power-on the cluster.
- Node is accidently down.
- Node is accidently restarted.

### Related Issues

The original requirement is from issue [FEATURE Optimize for Frequent Power-off/Power-On operating procedures](https://github.com/harvester/harvester/issues/3261).

The issue is converted to an EPIC, and a list of sub-stories are created.

## Motivation

When the cluster/node is rebooted without plan, the cluster/node should be able to recover and the related k8s pods, VMs, user workloads should all be able to recover.

The cluster needs to have a very high quality of robustness. It is not achieved by nature, it comes from perfect system design and large amount of tests and enhancements.

### Goals

Define the roubustness goals.

Define the roubustness enhancements.

### Non-goals [optional]

The following features are not covered in this HEP.

- Enhance UI with cluster shutdown menu and status display
- Enhance NODE level stop/maintain and shutdown
- Enhance 3-rd party CSI stopping
- Enhance k8s (etcd) backup and restore
- Enhance VM backup and restore
- Enhance managed dhcp
- Enhance loadbalancer harvester

## Proposal

A Harvester cluster is deployed in the rough sequence of:

>OS -> rancherd -> rke2 -> k8s ->  fleet -> harvester -> longhorn | monitoring | logging..., -> virtual-machine ...

The robustness will be analyzed in each layer.

### User Stories

Running Harvester in edge or remote environments with either intermittent power or with devices needing to be turned off and moved to a new location frequently (sometimes daily).

Operators turning the cluster off and on are not highly technical with Kubernetes and thus can't be expected to troubleshoot stuck containers that don't come back online after startup.

It was observed that, when run 5+ VMs on the cluster (1~3 Nodes), stop all nodes at same time, then restart the cluster at certain time. Some VMs may stuck / take quite a long time to restart.

#### 1. Cluster can boot up after frequent power-off and power-on

Target:
- The Linux OS can boot up, the node IP, cluster VIP are kept same and recovered.

On edge scenarios, users are suggested to use static IP if there is no DHCP server available.

The Linux OS in Harvester is read-only, it is immune to unplaned changes.

#### 2. Cluster can recover after frequent power-off and power-on

Target:
- Rancher, RKE2 services can recover
- Etcd DB can recover
- k8s clusters can recover

The embedded rancherd and RKE2 server/client are sured by systemd, there can recover.

ETCD has it's internal mechanism to sure the data consistency. By default, the ETCD snapshot is taken each day at 12:00.

When abover are recovered, the kubernetes cluster will be brought up by RKE2.

Harvester keeps updating the verson of Rancher and RKE2.

In Harvester v1.3.0, it has:
```
RKE2_VERSION="v1.27.10+rke2r1"
RANCHER_VERSION="v2.8.1"
```

#### 3. PVCs/Volumes can recover after frequent power-off and power-on

Target:
- Longhorn can recover all PVCs/Volumes.

In Harvester the VMs are using PVC/Longhorn volumes as their root and data disks by default.
There are some managedcharts/addons in Harveter, which also use PVC/Longhorn volumes, like `rancher-monitoring`, `rancher-vcluster`.

The [Longhorn:Cloud native distributed block storage for Kubernetes](https://longhorn.io/) is keeping developing and enhancing, and Harvester bumps the Longhorn to it's latest stable release in each Harvester release. Features and robustness are increased stably.

In Harvester v1.3.0, the Longhorn version is: v1.6.0.

#### 4. VMs can recover after frequent power-off and power-on

Target:
- All VMs can recover with Longhorn PVC/Volumes as their root disks.

From the history data from QA tests and production deployment, this is the weak point as of Harvester v1.2.1.

In Harvester v1.3.0, those known issues and enhancements are solved/added.

##### Bug 1: VM volume fails to mount/unmount

Before the root cause is identified, there are some issues like:
- The VM can't reboot after cluster restating
- The VM stucks in terminating
- The VM gets IO error
- ...

Those various phenomena cause the impression that the Harvester/Longhorn is not robust enough.

With big efforts on troubleshooting and reproducing, those issue are finally confirmed to be caused by same root cause, a few of them are listed.

[BUG VM got IO error after host restart](https://github.com/harvester/harvester/issues/4633)

		VM can't restart after a host accidentally restarts. VMI changes frequently between ready and unready and it has VMI was paused, IO error error message. At first, related Volume's robustness is fault and engine is stopped. After around 30 minutes, the volume is attached and engine is running, but VMI still can't get ready. 

		VM status flipping between Paused and Running.

		Longhorn fix: from v1.4.x
		Fix engine migration crash (backport #2275) https://github.com/longhorn/longhorn-manager/pull/2287

		K8s fix from v1.26
		Automated cherry pick of #122211: Fix device uncertain errors on reboot https://github.com/kubernetes/kubernetes/pull/122398

		RKE2 v1.27.10+rke2r1 has fix from upstream k8s
		https://github.com/rancher/rke2/releases/tag/v1.27.10%2Brke2r1

		Harvester: bumped REK2 to v1.27.10 in v1.3.0

[BUG After power off Harvester nodes then power on for days, all VMs display Stopping state and can't launch](https://github.com/harvester/harvester/issues/4033)

		From Harvester's view, those VM's were pendng deleting/stopping, the vmi deletionTimestamp: "2023-06-05T06:11:33Z" was set.

		Kubelet has warning that umounting PVC failed:

		```
		Warning: Unmap skipped because symlink does not exist on the path: /var/lib/kubelet/pods/5c80253b-13fe-4def-8288-c7433c08a3df/volumeDevices/kubernetes.io~csi/pvc-40df082e-2ac6-4beb-97a2-9a58de527fcc
		```

[BUG VM stuck in stopping state. Then after delete stuck in terminating phase](https://github.com/harvester/harvester/issues/3539)

[Is this a typical problem with this harvester that the rke2 clusters above the harvester no longer start (remains in pending state) after rebooting the harvester nodes following a power outage for example? version 1.2](https://github.com/harvester/harvester/issues/4789)

##### Bug 2: VM fails to migrate due to LVM devices are accidently enabled

[BUG Do not activate the LVM device on the harvester node](https://github.com/harvester/harvester/issues/4674)

[BUG Migrating (or simply stopping) VMs with LVM volumes causes Buffer I/O errors in kernel log of source host](https://github.com/harvester/harvester/issues/3843)

##### Bug 3: VM stucks in loop toggle between "pause"/"unpause" due to volume is used up

[BUG vm stuck in loop toggle between "pause"/"unpause"](https://github.com/harvester/harvester/issues/4828)

This issue is realted disk used up, need to add the workaround to Harvester document.

##### Bug 4: VM start button is not visible

[Doc Add workaround for VM start button is not visible](https://github.com/harvester/harvester/issues/4659)

##### Enhancement 1: Rebuild the last healthy replica during node drain

[FEATURE rebuild last healthy replica during node drain #3378](https://github.com/harvester/harvester/issues/3378)

Planned in Harvester v1.4.0

##### Enhancement 2: Harvester supports clear VMI objects automatically when VM is power-off from inside VM

[ENHANCEMENT Harvester supports clear VMI objects automatically when VM is power-off from inside VM #5081](https://github.com/harvester/harvester/issues/5081)

Planned in Harvester v1.4.0

##### Enhancement 3: Managed dhcp is more robust

As an in-cluster service which is related to vm-lifecycle-managedment, this service will be enhanced for such scenarios.

Planned in Harvester v1.4.0

##### Enhancement 4: Loadbalancer harvester is more robust

As an in-cluster service which is related to vm-lifecycle-managedment, this service will be enhanced for such scenarios.

Planned in Harvester v1.4.0

### User Experience In Detail

Detail what user need to do to use this enhancement. Include as much detail as possible so that people can understand the "how" of the system. The goal here is to make this feel real for users without getting bogged down.

### API changes

## Design

### Implementation Overview

Overview on how the enhancement will be implemented.

### Test plan

Integration test plan.

1. Set up a Harvester cluster with 1 ~ N nodes.
1. Set up storage network (optional).
1. Creat 5+ VMs.
1. Set up workloads on those VMs.
1. Observe all Longhorn volumes are in healthy/degraded states.
1. Power off the cluster.
1. Power on the cluster.
1. Observe cluster is recovered, VIP can be accessed.
1. Observe all Longhorn volumes are recovered.
1. Observe all VMs are recovered in a few minutes.

### Upgrade strategy

Anything that requires if user want to upgrade to this enhancement

## Note [optional]

Additional nodes.
