# Enhanced Maintenance Mode

## Summary

Extend the existing maintenance mode with a configurable option that allows the user to explicitly define the behavior of individual VMs in maintenance mode. There are situations in which the user does not want VMs to be migrated to another node or all VMs to be forcibly shut down.

### Related Issues

[#5069](https://github.com/harvester/harvester/issues/5069)

## Motivation

When setting a Harvester node into maintenance mode it will (try to) migrate all VMs on that node to other nodes in the cluster or shut down all of them. This behavior is sometimes not desired. Therefore, the user must be able to define the behavior of the VM more precisely, as there are usage scenarios in which a VM should behave different to the current implementation.

### Goals

Allows users to explicitly define the behavior of VMs in maintenance mode more precisely.

### Non-goals [optional]

## Proposal

Introduce the label `harvesterhci.io/maintain-mode-strategy` which can be added to a `VirtualMachine` resource to specify the behaviour of the VM in case of a node maintenance event. The following modes are supported:

- `Migrate` (default): The VM will be live-migrated to another node in the cluster. This is the behavior of the maintenance feature as it is at the moment and will be default with this enhancement.
- `ShutdownAndRestartAfterEnable`: Shut down and restart the VM after maintenance mode is enabled. The VM will be scheduled on a different node.
- `ShutdownAndRestartAfterDisable`: Shut down when maintenance mode is enabled and restart the VM after maintenance mode is disabled. The VM will stay on the same node.
- `Shutdown`: Shut down when maintenance mode is enabled. Do NOT restart the VM, it remains switched off.

If the `Force` checkbox is selected in the maintenance dialog, a configured maintenance mode strategy of a VM is overridden and it is shut down and remains off regardless of the configured strategy.
For all other VMs that do not have the label `harvesterhci.io/maintain-mode-strategy`, the current behavior of the forced maintenance mode is applied.

### User Stories

Before this enhancement it was not possible to specify the behavior of an individual VM in maintenance mode. There were only two options; the VMs are live-migrated or forcibly shut down. With this enhancement, users can now explicitly specify the behavior of a VM in maintenance mode.

#### Story 1
I want to shut down only certain VMs when I put a node into maintenance mode. These VMs must remain switched off after maintenance mode has been disabled.

#### Story 2
I want to shut down and restart certain VMs when maintenance mode is enabled on a node. This is necessary because these VMs need to perform certain actions when they are moved to another node. This can be ensured by executing scripts when shutting down and then restarting on the new node.

#### Story 3
I want to shut down certain VMs when the maintenance mode is enabled on a node. These VMs need to be restarted on the same node after the maintenance mode is disabled.

### User Experience In Detail

The user can add the label `harvesterhci.io/maintain-mode-strategy` for each VM of his choice in the Harvester UI or do this using the `kubectl` command line tool with the help of filters for many VMs at once. 

### API changes

No

## Design

### Implementation Overview

The names of the labels and annotations introduced here are based on the existing label `harvesterhci.io/maintain-status` and have been adopted to ensure consistency of the names.  

- Introduce the label `harvesterhci.io/maintain-mode-strategy` which can be added to a `VirtualMachine` resource to specify the behaviour of the VM in case of a node maintenance event.
- Introduce the annotation `harvesterhci.io/maintain-mode-strategy-node-name` which is used to identify the VMs that need to be restarted when the maintenance mode of a node is deactivated. This annotation is automatically added and removed by the maintenance mode code.
- Enhance the code part that is responsible for synchronizing the instance labels from the `VirtualMachine` resource to the `VirtualMachineInstance` resource. Add a list of extra labels that have to be synchronized in addition to the instance labels. This list can be enhanced in future by other features if necessary.
- For this feature improvement the label `harvesterhci.io/maintain-mode-strategy` is sync-ed to the `VirtualMachineInstance` resource because it is needed by a kubectl drain helper filter that is required to ignore those VMs that do not have to be live-migrated.

### Test plan

#### Case 1
1. Add the `harvesterhci.io/maintain-mode-strategy` label to a VM of your choice. Press `Edit YAML` and add `harvesterhci.io/maintain-mode-strategy: ShutdownAndRestartAfterDisable` to `metadata.labels`. Save the settings.
2. Make sure a second VM without the `harvesterhci.io/maintain-mode-strategy` label is running on the same node. Migrate it to the node on which the other VM is running if necessary.
3. Note the node on which the VMs are running.
4. Go to the `Hosts` page and select the `Enable Maintenance Mode` action in the actions dropdown menu of the node your VMs are running on.
5. After some time the `Cordon` state should be displayed for the host we are putting into maintenance mode.
6. Now go to the `Virtual Machines` page. The VM with the `harvesterhci.io/maintain-mode-strategy` label should show the `Stopping` status badge. The other VM should show the `Migrate` status badge.
7. After some time the VMs should be shut down or migrated to another node. Choose the `Edit YAML` action menu on the powered off VM and make sure the `harvesterhci.io/maintain-mode-strategy-node-name: <NODENAME>` is in the annotations.
8. Go back to the `Hosts` page. The host of choice should now (or after some time) display the `Maintenance` status badge.
9. Disable the maintenance mode via the action menu of the node. After some time the `Maintenance Mode` status badge should be gone away.
10. Go to the `Virtual Machines` page. The previously shut down VM with the `harvesterhci.io/maintain-mode-strategy: ShutdownAndRestartAfterDisable` label should be started again.

In the logs of the `harvester-xxx-xxx` pods you should find something like this:
```
time="2024-03-19T12:45:24Z" level=info msg="attempting to place node in maintenance mode" node_name=harvester-node-2
time="2024-03-19T12:45:24Z" level=info msg="force stopping VM" namespace=default virtualmachine_name=test01
time="2024-03-19T12:45:24Z" level=info msg="migration of pod owned by VM test01 is skipped because of the label harvesterhci.io/maintain-mode-strategy" namespace=default pod_name=virt-launcher-test01-gfgjp
time="2024-03-19T12:55:37Z" level=info msg="restarting the VM that was shut down in maintenance mode" namespace=default virtualmachine_name=test01
```

#### Case 2
1. Add the `harvesterhci.io/maintain-mode-strategy` label to a VM of your choice. Press `Edit YAML` and add `harvesterhci.io/maintain-mode-strategy: ShutdownAndRestartAfterEnable` to `metadata.labels`. Save the settings.
2. Make sure a second VM without the `harvesterhci.io/maintain-mode-strategy` label is running on the same node. Migrate it to the node on which the other VM is running if necessary.
3. Note the node on which the VMs are running.
4. Go to the `Hosts` page and select the `Enable Maintenance Mode` action in the actions dropdown menu of the node your VMs are running on.
5. After some time the `Cordon` state should be displayed for the host we are putting into maintenance mode.
6. Now go to the `Virtual Machines` page. The VM with the `harvesterhci.io/maintain-mode-strategy` label should show the `Stopping` status badge. The other VM should show the `Migrate` status badge.
7. Choose the `Edit YAML` action menu on the powered off VM and make sure the `harvesterhci.io/maintain-mode-strategy-node-name: <NODENAME>` is in the annotations.
8. Go back to the `Hosts`page. Wait until the `Maintenance`status badge is shown.
9. On the `Virtual Machines` page, the VM that was shut down should be restarted and running on a different node than before. Note, this VM was NOT migrated; it was shutdown and restarted.
10. On the `Hosts` page, disable the maintenance mode via the action menu of the node. After some time the `Maintenance Mode` status badge should be gone away.
11. Go to the `Virtual Machines` page. The VMs should still run and the node on which they run should not have changed again.

In the logs of the `harvester-xxx-xxx` pods you should find something like this:
```
time="2024-03-19T13:00:30Z" level=info msg="attempting to place node in maintenance mode" node_name=harvester-node-1
time="2024-03-19T13:00:30Z" level=info msg="force stopping VM" namespace=default virtualmachine_name=test01
time="2024-03-19T13:00:31Z" level=info msg="migration of pod owned by VM test01 is skipped because of the label harvesterhci.io/maintain-mode-strategy" namespace=default pod_name=virt-launcher-test01-rsvwf
time="2024-03-19T13:01:19Z" level=info msg="restarting the VM that was temporary shut down for maintenance mode" namespace=default virtualmachine_name=test01
```

#### Case 3
1. Add the `harvesterhci.io/maintain-mode-strategy` label to a VM of your choice. Press `Edit YAML` and add `harvesterhci.io/maintain-mode-strategy: Shutdown` to `metadata.labels`. Save the settings.
2. Make sure a second VM without the `harvesterhci.io/maintain-mode-strategy` label is running on the same node. Migrate it to the node on which the other VM is running if necessary.
3. Note the node on which the VMs are running.
4. Go to the `Hosts` page and select the `Enable Maintenance Mode` action in the actions dropdown menu of the node your VMs are running on.
5. After some time the `Cordon` state should be displayed for the host we are putting into maintenance mode.
6. Now go to the `Virtual Machines` page. The VM with the `harvesterhci.io/maintain-mode-strategy` label should show the `Stopping` status badge. The other VM should show the `Migrate` status badge.
7. Choose the `Edit YAML` action menu on the powered off VM and make sure the `harvesterhci.io/maintain-mode-strategy-node-name: <NODENAME>` is NOT in the annotations.
8. Go back to the `Hosts` page. Wait until the `Maintenance` status badge is shown.
9. On the `Virtual Machines` page, the VM with the `harvesterhci.io/maintain-mode-strategy` label should be off. The other VM should be migrated to another node.
10. On the `Hosts` page, disable the maintenance mode via the action menu of the node. After some time the `Maintenance Mode` status badge should be gone away.
11. On the `Virtual Machines` page, the VM with the `harvesterhci.io/maintain-mode-strategy` label should be still off.

In the logs of the `harvester-xxx-xxx` pods you should NOT find something like this:
```
time="2024-03-19T12:55:37Z" level=info msg="restarting the VM that was ... shut down in maintenance mode" namespace=default virtualmachine_name=test01
```

#### Case 4
1. Add the `harvesterhci.io/maintain-mode-strategy` label to a VM of your choice. Press `Edit YAML` and add `harvesterhci.io/maintain-mode-strategy: ShutdownAndRestartAfterDisable` to `metadata.labels`. Save the settings.
2. Make sure a second VM without the `harvesterhci.io/maintain-mode-strategy` label is running on the same node. Migrate it to the node on which the other VM is running if necessary.
3. Note the node on which the VMs are running.
4. Go to the `Volume` page in the embedded Longhorn UI and reduce the number of replicas to 1 for the volume that belongs to the VM that has no maintenance mode strategy. Make sure that you delete all replicas that are not located on the node that is to be put into maintenance mode.
5. Go to the `Hosts` page in the Harvester UI and select the `Enable Maintenance Mode` action in the actions dropdown menu of the node your VMs are running on. Check the `Force` checkbox. Note that this overrides the maintenance mode strategy of the VMs (which is to be tested here).
6. After some time the `Cordon` state should be displayed for the host we are putting into maintenance mode.
7. Now go to the `Virtual Machines` page. Both VMs should show the `Stopping` status badge and should be both `Off` after some time.
8. Disable the maintenance mode on the `Hosts` page via the action menu of the node. After some time the `Maintenance Mode` status badge should be gone away.
9. Go to the `Virtual Machines` page. Both VMs should be still off.
10. Choose the `Edit YAML` action menu on the VM that was labeled with `harvesterhci.io/maintain-mode-strategy: ShutdownAndRestartAfterDisable` and make sure the `harvesterhci.io/maintain-mode-strategy-node-name: <NODENAME>` is NOT in the annotations.
11. Note that in this test case, the node cannot switch to `Maintenance Mode` because several pods cannot be drained due to the deleted replicas of the Longhorn volume.

In the logs of the `harvester-xxx-xxx` pods you should NOT find something like this:
```
time="2024-03-19T12:55:37Z" level=info msg="restarting the VM that was ... shut down in maintenance mode" namespace=default virtualmachine_name=test01
```

#### Case 5
1. Add the `harvesterhci.io/maintain-mode-strategy` label to a VM of your choice. Press `Edit YAML` and add `harvesterhci.io/maintain-mode-strategy: ShutdownAndRestartAfterDisable` to `metadata.labels`. Save the settings.
2. Note the node on which the VM are running.
3. Go to the `Hosts` page and select the `Enable Maintenance Mode` action in the actions dropdown menu of the node your VM is running on.
4. After some time the `Cordon` state should be displayed for the host we are putting into maintenance mode.
5. Now go to the `Virtual Machines` page. The VM with the `harvesterhci.io/maintain-mode-strategy` label should show the `Stopping` status badge.
6. After some time the VM should be shut down. Choose the `Edit YAML` action menu on the powered off VM and make sure the `harvesterhci.io/maintain-mode-strategy-node-name: <NODENAME>` is in the annotations.
7. Go back to the `Hosts` page. The host of choice should now (or after some time) display the `Maintenance` status badge.
8. Delete this node. This might take some time. When it is done, go to the `Virtual Machines` page.
9. Choose the `Edit YAML` menu on the shut-down VM. The `harvesterhci.io/maintain-mode-strategy-node-name: <NODENAME>` annotation should not exist anymore.

### Upgrade strategy

No

## Note [optional]
