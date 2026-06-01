# Make use of status conditions when activating maintenance mode

## Summary

Currently, when a user enables maintenance mode on a node in Harvester, the system stores maintenance mode state using the `harvesterhci.io/maintain-status` annotation on the Node resource. This enhancement replaces the `harvesterhci.io/maintain-status` annotation with a standard Kubernetes `status.conditions` entry, providing a cleaner architecture where status information is stored in the proper location. This provides better visibility into the process, exposing states like `Initializing`, `Running`, `Completed`, or `Error` through status conditions.

### Related Issues

https://github.com/harvester/harvester/issues/9022

## Motivation

Right now there are only several annotations set on the node resource when the maintenance mode is activated. There is not much information available for users and the UI to see what is going on. By utilizing standard node status conditions, we provide a Kubernetes-native way to expose the maintenance lifecycle, making it easier for administrators and the Harvester UI to diagnose situations where things go wrong.

### Goals

- Replace the `harvesterhci.io/maintain-status` annotation with a standard node status condition of type `MaintenanceMode`.
- Establish status conditions as the single source of truth for node maintenance state.
- Provide `Initializing`, `Running`, `Completed`, and `Error` states via the condition reason.
- Ensure error messages during maintenance mode enablement (e.g., non-migratable VMs) are visible in the Harvester UI via the condition.
- Retain the `MaintenanceMode` condition while maintenance mode is active, and remove it immediately when maintenance mode is deactivated.
- Provide an action menu to clear the `MaintenanceMode` error status condition without needing to successfully re-trigger maintenance mode.

### Non-goals

- Altering the underlying mechanism or rules for node draining and virtual machine migrations.
- Automatically retrying failed maintenance mode transitions indefinitely without user intervention.

## Proposal

This enhancement replaces the `harvesterhci.io/maintain-status` annotation with a new node status condition of type `MaintenanceMode`. When a user enables maintenance mode on a node, the Harvester controllers will set this condition to reflect the current state (`Initializing`, `Running`, `Completed`, `Error`). If an error occurs (e.g., VMs cannot be migrated and force drain is not selected), the condition will be set to `Error` and the detailed error message will be propagated to the condition message. When maintenance mode is disabled, the `MaintenanceMode` condition is removed from the node.

By giving the error condition reason the exact name `Error`, the underlying [Wrangler summary framework](https://github.com/rancher/wrangler/blob/73fdb33d6a7529e14e2f769b66f717468a09201d/pkg/summary/summarizers.go#L320) will recognize it, and the error will automatically be displayed in the Harvester `Hosts` page below the node.

Additionally, to allow users to dismiss the error state without retrying maintenance mode, a new custom action `clearMaintenanceMode` will be added to the Node resource API.

#### State Transition Diagram

The `MaintenanceMode` condition follows this state machine:

```
[No Condition]
       |
       | enable maintenance mode requested
       v
  [Initializing] <---- Drain annotation detected, pre-checks + drain in progress
       |
       +-- pre-check fails OR drain fails --> [Error]
       |                          |
       |                          +-- clearMaintenanceMode --> [No Condition]
       |                          |
       |                          +-- retry maintenance mode (set drain-requested again)
       |                             |
       |                             v
       |                       [Initializing] (restart cycle)
       |
       | pre-checks pass & drain completes successfully
       v
   [Running] <---- drain-requested & drain-forced removed
       |           node is already cordoned/drained
       |           waiting for all VM migrations / restart handling to finish
       |
       | all VMs migrated away + ShutdownAndRestartAfterEnable VMs restarted
       v
  [Completed] <---- node fully in maintenance mode
       |
       +-- disable maintenance mode --> [No Condition] <---- all annotations removed, cordon removed
       |
       v
  (node remains cordoned in Completed state until disabled)
```

**Condition Transitions (State per State):**

- **No Condition → Initializing:** Triggered immediately when `harvesterhci.io/drain-requested` annotation is detected. First node update by `nodedrain-controller`. Both pre-checks and the drain attempt are executed in this state.

- **Initializing → Error:** Triggered when either a pre-check fails (`DrainPossible` fails OR non-migratable VMs found without `force` flag) or the subsequent cordon/drain step fails after the pre-checks have already passed. Condition is set to `reason: Error` with detailed message. User must retry or clear the error.

- **Initializing → Running:** Triggered only after both the pre-checks pass and the drain operation completes successfully. The `nodedrain-controller` updates the condition to `reason: Running`. At this point, the node has already been cordoned/drained and Harvester is waiting for all VM migrations and restart handling to finish.

- **Running → Completed:** Triggered by `maintain-controller` once:
  - All VirtualMachineInstances have been migrated off the node (no running VMs remain)
  - VMs with `harvesterhci.io/maintain-mode-strategy=ShutdownAndRestartAfterEnable` label have been restarted on other nodes
  - The controller updates `MaintenanceMode` condition to `reason: Completed`

- **Error → No Condition:** Triggered by:
  - User invokes `clearMaintenanceMode` action (removes only the condition without retrying; node returns to clean state)

- **Error → Initializing:** Triggered by:
  - User retries maintenance mode (by clicking "Enable Maintenance Mode" again in the UI, optionally with different settings like `force` flag)
  - The `drain-requested` annotation is set again, restarting the maintenance mode cycle from the beginning

- **Running/Completed → No Condition:** Triggered when user disables maintenance mode (removes condition and all annotations, uncordons the node).

### User Stories

#### Story 1: Clear visibility of maintenance mode progress
As a Harvester administrator, when I put a node into maintenance mode, I want to clearly see whether the node is still in preparation and drain execution (`Initializing`), already past the drain step and waiting for workload completion (`Running`), or has successfully been put into maintenance (`Completed`). Before this enhancement, I only saw `Cordoned` or `Maintenance` badges and had to manually check annotations to infer the exact progress.

#### Story 2: Error visibility when maintenance mode fails
As a Harvester administrator, if I attempt to put a node into maintenance mode but there are non-migratable VMs blocking the drain process, I want to see a clear error message indicating why the maintenance mode failed so I can take corrective action (such as using force drain or fixing the VM). Before this enhancement, the operation would silently fail or require inspecting controller logs.

#### Story 3: Clearing maintenance mode errors
As a Harvester administrator, when an error occurs during maintenance mode enablement, I want to clear the error state from the node without having to successfully complete the maintenance mode. I want an action to `Clear Maintenance Mode Error` to reset the node's status condition back to a clean state.

### User Experience In Detail

1. The user goes to the `Hosts` page in the Harvester UI.
2. The user selects `Enable Maintenance Mode` from the action menu on a specific node.
3. As soon as `drain-requested` is detected, a `MaintenanceMode` condition with reason `Initializing` and status `True` is added to the node.
4. Pre-checks run (validation of `DrainPossible`, non-migratable VM detection) and, if they pass, Harvester immediately starts the cordon/drain step. The condition remains `Initializing` during this entire preparation-and-drain phase.
5. If either the pre-check phase or the drain step fails, the condition reason is updated to `Error` immediately. The error message is included in the condition and surfaced in the UI. This error remains displayed until the user triggers another maintenance mode attempt or uses the `Clear Maintenance Mode Error` action menu.
6. After pre-checks pass and the drain operation completes successfully, the condition reason is updated to `Running`. At this point, the node is already cordoned/drained and Harvester is waiting for VM migrations and restart handling to finish on other nodes.
7. The condition **remains in `Running` state for the duration** that migrations and any required restart handling are still in flight (typically seconds to minutes, depending on workload size and migration speed).
8. Once all workloads have migrated off the node and any labeled VMs have been restarted on other nodes, the `maintain-controller` updates the condition reason to `Completed`.
9. When the user selects `Disable Maintenance Mode`, the `MaintenanceMode` condition is removed entirely and the cordon is removed.
10. If the node is in an `Error` state regarding maintenance mode, the user can select a `Clear Maintenance Mode Error` action from the node's action menu to manually remove the error condition, clearing the warning from the UI.

### API changes

There are no new Custom Resource Definitions (CRDs). The standard Kubernetes `Node` object's `status.conditions` list will contain a new condition.

The following Go constants will be introduced to standardize the condition:
```go
const (
	NodeConditionTypeMaintenanceMode corev1.NodeConditionType = "MaintenanceMode"
	NodeConditionReasonInitializing  string                   = "Initializing"
	NodeConditionReasonRunning       string                   = "Running"
	NodeConditionReasonCompleted     string                   = "Completed"
	NodeConditionReasonError         string                   = "Error"
)
```

A new API action endpoint `clearMaintenanceMode` will be added to the Node resource formatter to clear the error condition.

#### Node Resource Formatter Actions

**New Action: `clearMaintenanceMode`**
- **Availability:** Only shown in the UI when the `MaintenanceMode` condition has `reason: Error`
- **Permission:** Requires same permissions as `disableMaintenanceMode`
- **Behavior:** Removes the `MaintenanceMode` condition from the node without affecting maintenance mode status or annotations
- **Idempotent:** Safe to call multiple times; returns success if condition is not present
- **UI Label:** `Clear Maintenance Mode Error`

#### Example Error Messages

Common error scenarios and their corresponding condition messages:

```yaml
# Example 1: Non-migratable VM blocking drain
status:
  conditions:
  - type: MaintenanceMode
    reason: Error
    status: "True"
    message: |
      enabling maintenance mode is impossible. Non-migratable VMs found: 
      default/ubuntu-vm cannot be migrated due to host affinity; 
      default/postgres-vm cannot be migrated due to pod disruption budget. 
      Use 'force drain' to perform a collective shutdown
    lastTransitionTime: 2024-06-01T10:30:00Z
    lastHeartbeatTime: 2024-06-01T10:30:05Z

# Example 2: Insufficient HA control plane nodes
status:
  conditions:
  - type: MaintenanceMode
    reason: Error
    status: "True"
    message: "enabling maintenance mode is impossible: unable to maintain HA with current node state"
    lastTransitionTime: 2024-06-01T10:30:00Z

# Example 3: Successful completion of drain
status:
  conditions:
  - type: MaintenanceMode
    reason: Completed
    status: "True"
    message: "Maintenance mode enabled"
    lastTransitionTime: 2024-06-01T10:35:00Z

# Example 4: Initialization / draining in progress
status:
  conditions:
  - type: MaintenanceMode
    reason: Initializing
    status: "True"
    message: "validating drain pre-checks and executing node drain"
    lastTransitionTime: 2024-06-01T10:29:00Z

# Example 5: Drain completed, waiting for workloads to finish moving
status:
  conditions:
  - type: MaintenanceMode
    reason: Running
    status: "True"
    message: "drain completed, waiting for VM migration and restart handling"
    lastTransitionTime: 2024-06-01T10:30:00Z
```

## Design

### Implementation Overview

The `nodedrain-controller` will be updated to manage the `MaintenanceMode` node status condition and migrate away from the `harvesterhci.io/maintain-status` annotation. The `maintain-controller` will manage the `Completed` state and condition cleanup on disable.

**Detailed flow in `nodedrain-controller.OnNodeChange`:**

1. **Detect Drain Annotation**: When `harvesterhci.io/drain-requested` annotation is detected on a node:
   - Update the node with `MaintenanceMode` condition set to `reason: Initializing`
   - Log the maintenance mode initiation

2. **Pre-checks and drain (still in Initializing state)**:
   - Execute `DrainPossible` validation
   - List and validate non-migratable VMs (respecting `force` flag)
   - Shut down VMs that require shutdown (non-migratable on forced drain, or labeled strategy VMs)
   - Execute `DrainNode` to cordon and drain the node
   - If either a pre-check or the `DrainNode` call fails:
     - Update the node with `MaintenanceMode` condition set to `reason: Error` (with detailed error message)
     - Remove the `harvesterhci.io/drain-requested` and `harvesterhci.io/drain-forced` annotations
     - Return error (operation stops here; user must retry or clear error)

3. **Transition to Running**: Once pre-checks pass and the drain operation succeeds:
   - Remove the `harvesterhci.io/drain-requested` and `harvesterhci.io/drain-forced` annotations
   - Update the node with `MaintenanceMode` condition set to `reason: Running`

**Detailed flow in `maintain-controller.OnNodeChanged`:**

4. **Completion Phase - Reconciliation Loop**:
   - The controller watches nodes in the `Running` condition state
   - Checks if any VirtualMachineInstances are still running on the node
   - If VMs exist: No action, return and wait for next reconciliation (the drain step is already done, but VM migration/restart handling is still in flight)
   - If no VMs exist (post-drain waiting is complete):
     - Update the node's `MaintenanceMode` condition from `reason: Running` to `reason: Completed`

5. **Disabling** (handled by UI/API handler):
   - When user disables maintenance mode, API handler or controller removes all maintenance mode annotations and deletes the `MaintenanceMode` condition from the node

6. **Error Clearing**: A new API action `clearMaintenanceMode` will be introduced in the node formatter. When triggered via the API:
   - Verify `MaintenanceMode` condition exists with `reason: Error`
   - Remove the `MaintenanceMode` condition from the node

**Key points:**
- `Initializing` includes both the pre-checks and the active cordon/drain step
- If pre-checks succeed but the drain step fails, the node still transitions from `Initializing` to `Error`
- `Running` is set by `nodedrain-controller` **only after the drain step completes successfully**
- `Running` persists **for the entire duration** that Harvester waits for VM migration / restart handling to finish after the drain step
- `maintain-controller` actively **reconciles and waits** in `Running` state for all VMs to disappear
- `Completed` transition happens **only after**:
  - All VMs have migrated away from the node
  - VMs with `ShutdownAndRestartAfterEnable` strategy have been restarted on other nodes

### Test plan

Validation note for all tests:
- `Initializing` / `Running` / `Completed` / `Error` are validated via `Node.status.conditions` (via `Edit YAML` in the UI or `kubectl` CLI).
- Quick CLI check for conditions:

```bash
kubectl get node <NAME> -o jsonpath='{.status.conditions[*]}' | jq
```

- Optional focused check for only `MaintenanceMode` condition:

```bash
kubectl get node <NAME> -o json | jq '.status.conditions[] | select(.type == "MaintenanceMode")'
```

- The Harvester UI does **not** render these reason values as separate badges.
- UI expectations are:
  - Display the `Maintenance` badge on the node on success.
  - Show the error message below the node in `Hosts` (Wrangler summary) in case of an error.

Test cases:
- **Successful enablement (conditions):** Enable maintenance mode and verify condition transitions in `status.conditions`: `Initializing` -> `Running` -> `Completed`.
- **Successful enablement (UI):** Verify UI shows `Maintenance` badge (no dedicated `Initializing`/`Running`/`Completed` badges).
- **Initialization visibility:** Verify `Initializing` appears while pre-checks run and remains present through the drain attempt.
- **Drain failure after successful pre-checks:** Simulate a `DrainNode` failure after pre-checks pass and verify `Initializing` -> `Error`.
- **Running duration:** Verify condition remains `Running` until all VMs are migrated from the node and restart handling is complete.
- **Completion behavior:** Verify `Completed` is set only after VM migration is done and restart logic for `ShutdownAndRestartAfterEnable` VMs has completed.
- **DrainPossible failure:** Simulate failure and verify `Initializing` -> `Error`, plus annotations' cleanup.
- **Non-migratable VM failure:** Create non-migratable VM(s), enable without force, verify `Error` in condition and error message in UI below node.
- **Disablement:** Disable maintenance mode from `Completed`; verify `MaintenanceMode` condition is removed.
- **Clear Error action:** From `Error`, invoke `Clear Maintenance Mode Error` action menu; verify condition is removed and warning disappears from UI.
- **Retry after error:** From `Error`, retry enable (optionally with force); verify a new cycle starts at `Initializing` and can progress to `Completed`.
- **Upgrade migration of completed maintenance state:** Upgrade a node that still has `harvesterhci.io/maintain-status=completed` and verify the upgrade script creates the equivalent `MaintenanceMode` condition with `reason: Completed` and removes the legacy completed annotation.

### Upgrade strategy

No general migration is required.

During a Harvester upgrade, nodes are expected to be out of maintenance mode. In normal operation, there is therefore no `harvesterhci.io/maintain-status` value to migrate.

**Exceptional case handling:**
- If a node is unexpectedly still marked with `harvesterhci.io/maintain-status=completed` when the upgrade runs, the upgrade script detects this legacy completed state, writes the equivalent `MaintenanceMode` status condition with `reason: Completed`, and removes the legacy completed annotation.
- After the upgrade finishes, the user must be able to disable maintenance mode from the UI exactly as with a node that entered `Completed` state natively under the new condition-based implementation.
- A node in a running maintenance transition (with `harvesterhci.io/maintain-status=running` annotation) is considered an invalid pre-upgrade state.

**Zero-downtime upgrade path:**
- Rolling update of Harvester components remains unchanged.
- Nodes not in maintenance mode are unaffected by this enhancement.
- Only the exceptional `Completed` annotation case is migrated by the upgrade script from the legacy annotation format to the new status condition format.

## Note

In the event of an error, the annotations `harvesterhci.io/drain-requested` and `harvesterhci.io/drain-forced` are removed. Retaining them would cause the system to constantly attempt to activate maintenance mode. Consequently, the user must explicitly re-trigger maintenance mode (e.g., with the force flag) after addressing the underlying issue or they can dismiss the message with the new `Clear Maintenance Mode Error` action.

### Additional Considerations

#### UI Display Details

**Condition Display Location:**
- In the `Hosts` page, error conditions are shown in the node's summary section (below the node status)
- In the node's YAML view (via "Edit YAML" action), the full condition object is visible in `status.conditions` including type, reason, message, and transition times (lastTransitionTime, lastHeartbeatTime)
- The `Initializing`, `Running`, and `Completed` states are NOT shown in the node's status indicators. There is only the already existing `Maintenance` badge.

**Action Menu Visibility:**
- `Disable Maintenance Mode` action remains always visible when node is in any maintenance mode state
- `Clear Maintenance Mode Error` action is only visible when `MaintenanceMode` status condition exists AND `reason == "Error"`

#### Backward Compatibility Notes

- The `harvesterhci.io/maintain-status` annotation is **removed** as part of this migration, but this is a safe change because:
  - It is used **only internally** within Harvester controllers and is not part of the public API contract
  - No external tools, operators, or custom scripts consume this annotation
  - The status condition provides the same information in a more standard Kubernetes format
- The `harvesterhci.io/drain-requested` and `harvesterhci.io/drain-forced` annotations continue to be used as internal lifecycle markers and are not affected by this migration
