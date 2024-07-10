# CPU Pinning

## Summary
Support virtual machine CPU pinning to pin guest's vCPUs to the host's pCPUs to provide predictable latency and
enhanced performance.

### Related Issues
https://github.com/harvester/harvester/issues/2305

## Motivation
Enabling CPU pinning could bring the following benefits:
- Performance Isolation: CPU pinning allows you to dedicate CPU cores or threads to particular virtual machines.
This isolation can prevent performance interference between different VMs running on the same physical hardware.
- Predictable Performance: By assigning dedicated CPU resources to VMs, you can achieve more predictable performance levels.
This is particularly important for applications with stringent performance requirements, such as real-time applications
or high-performance computing workloads.
- Reduced Latency: CPU pinning can help reduce latency by ensuring that critical tasks consistently run on the same set
of CPU cores or threads. This can be beneficial for applications where low latency is crucial.

### Goals
- Allowing virtual machines to exclusively use the CPU resources.
- After live migration, the virtual machine retains CPU pinning settings.

### Non-goals
This HEP does not cover:
- Allow user to specify specific CPUs in virtual machine.
- Enable CPU pinning without restarting the VM.

## Proposal
Enabling CPU pinning for KubeVirt requires setting the kubelet argument `--cpu-manager-policy` to [static](https://kubernetes.io/docs/tasks/administer-cluster/cpu-management-policies/#static-policy).
However, this change affects the way CPU resources are utilized by all pods that meet the condition as follows, not just VMs. 
- Both requests and limits must be configured in pods (Guaranteed QoS) and their values must be the same integer.
And that means if a node use static cpu manager policy, there will have two scenarios, one is pods that meet the criteria as above, and the other one is not.
For the former, CPUs inside the pods are not isolated and are all shared with each other, and the later will have isolated CPUs in each pod.
Note that if a node use none cpu manager policy, all CPUs in the pods are not isolated and could be shared with each other.

To address this, the proposal is to allow users to set the CPU manager policy for each node individually, rather than forcing all nodes to use the same policy. This flexibility is crucial because switching the CPU manager policy from static to none causes CPUs in Guaranteed QoS pods to lose isolation. To prevent issues, an error should be thrown if any VMs with CPU pinning enabled exist on a node where the user wants to switch from static to none. The user must then either migrate these VMs to another node or stop them. Forcing a uniform policy across all nodes would be problematic, as turning the CPU manager policy to none would require migrating all CPU pinned VMs to other nodes. If no nodes are using the static policy, migration is impossible, leaving users with no option but to stop all VMs, which is inconvenient and reduces availability.

So we cannot set `--cpu-manager-policy` to static on all nodes initially. Instead, we allow users to decide which nodes they want to set `--cpu-manager-policy=static`.
To enable CPU pinning, we also need to specify `dedicatedCpuPlacement=true` to VirtualMachine CR, this will add 1-1 CPU mapping to libvirt xml.

### User Stories

#### Story 1: Set cpu-manager-policy from none to static
Assuming we have 3 Harvester nodes (node1, node2, node3), each containing CPU sets ranging from 0 to 47, total 48 CPUs,
and there is a VM named `test` with 1 CPU deployed on node1. Initially, the CPU set in test is `0-47` since CPU pinning is not yet enabled.
We can examine VM CPU set by running
```sh
kubectl exec -n default -it virt-launcher-test-9nmwp -- taskset -cp 1
```
the output is
```txt
pid 1's current affinity list: 0-47
```

After setting up `cpu-manager-policy` to `static` on node1 and node2, we create another VM named test2 with 16 CPUs and enable CPU pinning.
The CPU set in `test2` is `1-8,25-32` (16 CPUs). To examine the CPUs are pinned in `test2`, we can run cmd as below. 
```sh
kubectl exec virt-launcher-test2-7wgvk -- virsh dumpxml default_test2 | awk "/<cputune>/,/<\/cputune>/"`
```
the output is
```xml
  <cputune>
    <vcpupin vcpu='0' cpuset='1'/>
    <vcpupin vcpu='1' cpuset='25'/>
    <vcpupin vcpu='2' cpuset='2'/>
    <vcpupin vcpu='3' cpuset='26'/>
    <vcpupin vcpu='4' cpuset='3'/>
    <vcpupin vcpu='5' cpuset='27'/>
    <vcpupin vcpu='6' cpuset='4'/>
    <vcpupin vcpu='7' cpuset='28'/>
    <vcpupin vcpu='8' cpuset='5'/>
    <vcpupin vcpu='9' cpuset='29'/>
    <vcpupin vcpu='10' cpuset='6'/>
    <vcpupin vcpu='11' cpuset='30'/>
    <vcpupin vcpu='12' cpuset='7'/>
    <vcpupin vcpu='13' cpuset='31'/>
    <vcpupin vcpu='14' cpuset='8'/>
    <vcpupin vcpu='15' cpuset='32'/>
  </cputune>
```

Subsequently, when we check VM `test`, we observe that the CPU set in it changes to `0,9-24,33-47`. The CPUs occupied by `test2`
are excluded from the CPU shared pool.

Now, let's examine other pre-existing pod, such as the `harvester-node-manager` pod deployed on node1. The CPU set in it is also `0,9-24,33-47`.

#### Story 2: Set cpu-manager-policy from static to none
Assume that we have 3 nodes (node1, node2, node3), each node contains CPU set `0-47`, and the `cpu-manager-policy` is set to `static` in node1, node2.
Initially, we have a VM named `test` with CPU pinning enabled, utilizing 16 CPUs(`1-8,25-32`), deployed to node1.

Now, if user want to change the `cpu-manager-policy` to `none` in node1, an error raises. Changing the CPU manager policy to 'none' will cause CPUs to
no longer be isolated in those VMs. Therefore, an error is returned if any VM on the node has CPU pinning enabled.

Users need to shut down the VM `test` or migrate it to node2 or node3 to set the cpu-manager-policy to none in node1.

#### Story 3: users who don't want to deploy workload to nodes that set cpu-manager-policy to static
If users are aware of which nodes apply the static CPU manager policy, they can utilize [node affinity](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#node-affinity).
Another approach is to use [node selector](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector),
KubeVirt will label `cpumanager=false` to nodes that use cpu manager policy `none` and label nodes with `cpumanager=true`
when the policy is set to `static`.

#### Story 4: Restart VM
Assume a VM with CPU pinning enabled and is utilizing 3 CPUs (`4,5,6`). After restarting the VM,
the CPU set might change to something like `5,6,8`. This happens because restarting a VM essentially
deletes the old pod and creates a new one, with no guarantee that the VM retain the same CPU set
or remain on the same node.

#### Story 5: Live Migration
Assume that we have 2 nodes (node0, node1), each node contains CPU set `0-11`, and the `cpu-manager-policy` is set to `static` in all nodes.
Initially, we have the following VMs enable CPU Pinning, and all utilizing 2 CPUs.
- vm `test` in node1, CPU set `4, 5`
- vm `test2` in node1, CPU set `6, 7`
- vm `test3` in node0, CPU set `4, 5`

Then start migrating `test` to node0. After migration done, when we check CPU set in vm `test`, the CPU set
is `6, 7`, as you can see no longer `4, 5` as it was in node1.

If we create more VMs and the CPU resources on all nodes are nearly exhausted,
attempting to migrate `test` to node1 fails after several minutes and falls back to node0.

#### Story 6-1: Upgrade Cluster
Assume that we have 2 nodes (node0, node1), each node contains CPU set `0-11`, and the `cpu-manager-policy` is set to `static` in all nodes.
Initially, we have one VMs enable CPU Pinning  utilizing 2 CPUs.
- vm `test` in node1, CPU set `4,5`

Then start upgrade. After upgrade done, when we check CPU set in vm `test`, the CPU set is `4, 5` while now it is in node0.
There is no guarantee that vm retain the same CPU set or remain on the same node after upgrade.

#### Story 6-2: Upgrade Cluster
Assume that we have 2 nodes (node0, node1), each node contains CPU set `0-11`, and the `cpu-manager-policy` is set to `static` in **node0**.
Initially, we have one VMs enable CPU Pinning  utilizing 2 CPUs.
- vm `test` in node0, CPU set `4,5`

Then start upgrade. While the upgrade stuck in drain node stage since node1 doesn't enable cpu pinning, that means there is no way to migrate vm `test` to another node. User have to stop the vm `test` to continue upgrade harvester.

### User Experience In Detail
1. User decide which nodes they want to enable `static` cpu-manager-policy.
2. Then user can create vm with cpu-pinning and harvester will deploy the vm to the nodes that enable `static` cpu-manager-policy.

### API changes
Add two new actions to nodes endpoint.
```
POST /v1/harvester/nodes/{node_name}?action=enableCPUManager
```
```
POST /v1/harvester/nodes/{node_name}?action=disableCPUManager
```
- enableCPUManager means `cpu-manager-policy=static`
- disableCPUManager means `cpu-manager-policy=none`

## Design

### Implementation Overview
To implement this proposal, there are several steps need to be taken.
1. Enable [CPU Manager Static Policy](https://kubernetes.io/docs/tasks/administer-cluster/cpu-management-policies/#static-policy).
  Currently Harvester use `none` policy which is the default one. And we also need to set kubelet
  `--kube-reserved` or `--system-reserved` options. This step needs to delete cpu_manager_state file and restart rke2-server/rke2-worker (or you can say kubelet).
2. Enable KubeVirt `CPUManager` feature gate. Then KubeVirt could add [cputune](https://libvirt.org/formatdomain.html#cpu-tuning)
  part to libvirt domain xml to pin guest's vCPUs in virtual machine to the host's pCPUs.
3. Allow virtual machine to apply [Guaranteed QoS](https://kubernetes.io/docs/concepts/workloads/pods/pod-qos/#guaranteed).
  Currently Harvester force all virtual machines apply overcommitting resources which make all virtual machine pods under
  [Burstable QoS](https://kubernetes.io/docs/concepts/workloads/pods/pod-qos/#burstable). While the CPU Manager static
  policy only reserved cpus when both cpu requests and limits values must be the same integer.

In step1, we have to set up the reserved resource due to the k8s cpu manager requirement.
And we should ensure kube-reserved and system-reserved are both defined as rke2 is using static pods and we should ensure they do not get impacted in anyway.
Here we utilize the formula as follows to set `system-reserved` and `kube-reserved` CPU resources during install harvester node and upgrade path. This is inspired by [GKE CPU reservations](https://cloud.google.com/kubernetes-engine/docs/concepts/plan-node-sizes).

```text
6% of the first core +
1% of the next core (up to 2 cores) +
0.5% of the next 2 cores (up to 4 cores) +
0.25% of any cores above 4 cores

if the maximum number of Pods per node beyond the default of 110 (Currently harvester max pods is 200),
reserves an extra 400 mCPU in addition to the preceding reservations.
```

After calculating the cpu reserved resources, add the following content to `/etc/rancher/rke2/config.yaml/d/99-z00-harvester-reserved-resources.yaml` during harvester installation.

```yaml
kubelet-arg+:
- "system-reserved=cpu=490m"
- "kube-reserved=cpu=490m"
```

and add another config `/etc/rancher/rke2/config.yaml/d/99-z01-harvester-cpu-manager.yaml` which includes the cpu-manager-policy setting.

```yaml
kubelet-arg+:
- "cpu-manager-policy=none"
```

> [!NOTE]
> After upgrading harvester to a newer version, a new file `99-max-pods.yaml` shows up which override all the kubelet-args, that's why I have to 
> create another two files `/etc/rancher/rke2/config.yaml/d/99-z00-harvester-reserved-resources.yaml`, `/etc/rancher/rke2/config.yaml/d/99-z01-harvester-cpu-manager.yaml`
> instead of adding these configs to `90-harvester-server.yaml` or `90-harvester-worker.yaml`. Another benefit is that if we want to modify cpu manager policy setting,
> we only need to update one file `99-z01-harvester-cpu-manager.yaml`.

Add two new actions to nodes endpoint to change cpu manager policy to either static or none.
```
/v1/harvester/nodes/{node_name}?action=enableCPUManager
/v1/harvester/nodes/{node_name}?action=disableCPUManager
```

Introduce a new annotation `harvesterhci.io/cpu-manager-policy-update-status` here and it will be used in the following steps,
the content inside it is in json format.
```json
{"status":"complete","policy":"none"}
```
- status: current update status, should be either `complete`, `failed`, `requested`, `running`.
- policy: cpu manager policy, should be either `none` or `static`.

Take action disableCPUManager as example, the steps are:
1. Check if the node is already under none cpu-manager-policy, if yes, return error. if no, go to next step.
  - node label `cpumanager: "false"` means node is under none cpu-manager-policy. The label is under 
    kubevirt control which continues checking the cpu manager policy. true means static policy, false 
    means none policy.
2. Check if there is any vm that enable cpu pinning, if yes, return error, if no go to next step.
  - Changing the CPU manager policy to 'none' will cause CPUs to no longer be isolated in those VMs. Therefore, an error is returned if any VM on the node has CPU pinning enabled.
3. Check if the node do have `harvesterhci.io/cpu-manager-policy-update-status` annotation, if yes, check if the status in it is running or requested, if yes, means the update is ongoing, return error. if no go to next step.
4. Check if the current node is in master role, if yes and there is other master node is updating the cpu manager policy, throw error. If no, go to next step.
  - During the updating policy process, we need to restart rke2-server if the node is in master role, and if all rke2-servers are offline, which means the whole cluster are unavailable, and we should avoid this kind of scenario.
5. Add annotation to the node
  ```yaml
  harvesterhci.io/cpu-manager-policy-update-status='{"status":"requested","policy":"none"}'
  ```
6. CPUManagerNodeController#OnChange
  - check the harvesterhci.io/cpu-manager-policy-update-status status is requested, if yes, submit
    k8s job to the node to update cpu manager policy and update the status in annotation to running.
  - the k8s job do the following things
    - update `cpu-manager-policy=none` in `/etc/rancher/rke2/config.yaml.d/99-z01-harvester-cpu-manager.yaml`
    - remove /var/lib/kubelet/cpu_manager_state.
    - if the node is worker, restart rke2-agent, if not, restart rke2-server.
7. CPUManagerJobController#OnChange
  - If job complete, update the annotation status to complete
    ```yaml
    harvesterhci.io/cpu-manager-policy-update-status='{"status":"complete","policy":"none"}'
    ```
  - If job failed, update the annotation status to failed
    ```yaml
    harvesterhci.io/cpu-manager-policy-update-status='{"status":"failed","policy":"none"}'
    ```

Once the above implementations are completed, and specify the cpu-manager-policy to static to the node, we can activate VM CPU pinning by including
`dedicatedCpuPlacement=true` in `.spec.template.cpu`.
```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
spec:
  template:
    spec:
      domain:
        cpu:
          cores: 2
          sockets: 1
          threads: 1
          dedicatedCpuPlacement: true
        resources:
          limits:
            cpu: 2
          requests:
            cpu: 2
[...]
```

#### Web UI
- On the hosts page, click the hamburger button on the right. If the current node has the label `cpumanager: "false"`, the pop-up will have a button labeled `Enable CPU Manager`.
- On the hosts page, click the hamburger button on the right. If the current node has the label `cpumanager: "true"`, the pop-up will have a button labeled `Disable CPU Manager`.
- If the status in the node annotation `harvesterhci.io/cpu-manager-policy-update-status` is `failed`, an error popup should warn the user that `Enable CPU Manager` or `Disable CPU Manager` has failed.
- On the hosts page, click on the node name. In the Basics tab, a new attribute called `cpumanager` will be displayed, whose value (either true or false) reflects the node label `cpumanager`.
- On the create VM page, there is a checkbox labeled `cpu-pinning`. Enabling this checkbox will add the following setting to `.spec.template.spec.domain.cpu` in the VirtualMachine CR. Users can only enable or disable the `cpu-pinning` checkbox when creating a VM or when the VM is stopped.
  ```yaml
  dedicatedCpuPlacement: true
  ```

### Test plan
1. Prepare harvester cluster and check if vm enabled cpu pinning do have the following libvirt xml settings. (assume the vm has 2 dedicated cpus)
```xml
  <cputune>
    <vcpupin vcpu='0' cpuset='1'/>
    <vcpupin vcpu='1' cpuset='2'/>
  </cputune>
```
2. verify cpu pinning functions works after upgrade from a version that doesn't have cpu pinning implementation to one that has cpu pinning (under one node and multiple harvester nodes)
3. check vm live migration works if cpu pinning is enabled, note that this should be tested under two+ harvester nodes, and at least two nodes swithc cpu-manager-policy to `static`, otherwise vm live migration won't work due to insufficient resources.

### Notes
- Users should also be able to enable VM CPU pinning through Terraform.

## Discussion
- In [Story 2: Set cpu-manager-policy from static to none](#Story-2-Set-cpu-manager-policy-from-static-to-none), after changing cpu manager policy from none to static and then change it back to none. All existing pods still use the same cpu sets as settings in static policy. Currently, I'm not sure if this will affect the pod performance or not.
