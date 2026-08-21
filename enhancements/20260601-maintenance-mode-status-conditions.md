# Make use of status conditions when activating maintenance mode

## Summary

When a node is put into maintenance mode in Harvester today, the lifecycle state is tracked solely through the `harvesterhci.io/maintain-status` annotation on the `Node` object (with the values `running` and `completed`). Annotations are opaque key/value strings: they carry no notion of transition time, no human-readable message, and, most importantly, no standard place to express *why* maintenance mode failed. Users and the UI therefore have very little insight into what is happening, especially when something goes wrong.

This enhancement makes the standard Kubernetes `Node.status.conditions` list the **single source of truth** for the maintenance lifecycle. A new condition of type `MaintenanceMode` exposes the current phase (`Validating` → `Draining` → `Evacuating` → `Completed`) and, on failure, an `Error` state whose **message** explains exactly what went wrong. The legacy `harvesterhci.io/maintain-status` annotation is removed and every internal consumer is migrated to read the condition instead.

### Related Issues

- https://github.com/harvester/harvester/issues/9022
- Reference PR (initial, annotation-mirroring approach): https://github.com/harvester/harvester/pull/9041
- Related: https://github.com/harvester/harvester/issues/8985, https://github.com/harvester/harvester/issues/8966

## Motivation

Right now only a small set of annotations is set on the node resource when maintenance mode is activated. There is not much information available for users and the UI to understand what is going on, and there is no Kubernetes-native way to surface an error. By moving the maintenance lifecycle to standard node status conditions, we:

- give administrators and the UI a first-class, observable lifecycle (with `lastTransitionTime` / `lastHeartbeatTime`)
- attach a human-readable message that is surfaced in the `Hosts` view when maintenance mode fails
- stop overloading annotations with state that properly belongs in `status`

### Goals

- Replace the `harvesterhci.io/maintain-status` annotation with a standard node status condition of type `MaintenanceMode`, and make the condition the single source of truth for node maintenance state.
- Define a formal, documented set of reasons: the happy-path lifecycle (`Validating`, `Draining`, `Evacuating`, `Completed`) plus a single failure reason (`Error`). All failure detail is conveyed through the condition `message`.
- Ensure failure messages are visible in the Harvester `Hosts` page, using the existing Wrangler summary behaviour (no UI/dashboard change required).
- Keep the condition present while maintenance mode is active or has failed, and remove it when maintenance mode is deactivated.
- Provide an explicit `clearMaintenanceMode` action so a user can dismiss a failed-state condition without having to successfully (re-)run maintenance mode.

### Non-goals

- Altering the underlying mechanism or rules for node draining and virtual machine migration.
- Automatically retrying failed maintenance mode transitions indefinitely without user intervention.
- Changing the user-facing trigger annotations `harvesterhci.io/drain-requested` and `harvesterhci.io/drain-forced`, these remain the way the API expresses user intent and are unaffected by this enhancement.
- Introducing multiple machine-readable failure reasons (see "Why a single `Error` reason").

## Proposal

A new node status condition of type `MaintenanceMode` is introduced and becomes the authoritative record of the maintenance lifecycle. The Harvester controllers set the condition as the node moves through the lifecycle; on failure they set it to an `Error` state carrying a descriptive message. When maintenance mode is disabled, the condition is removed.

### Condition model and reasons

The condition uses `status` to answer the question *"is the node engaged in maintenance mode?"* and `reason` to express the phase. The four lifecycle reasons map one-to-one to a distinct, controller-observable step, so each controller can tell from the current reason exactly where in the state machine it is.

| `status` | `reason` | Meaning | Set by |
|----------|----------|---------|--------|
| `True` | `Validating` | Request accepted; read-only pre-checks running (control-plane quorum + VM migratability). No side effects yet. | `nodedrain-controller` |
| `True` | `Draining` | Pre-checks passed; VMs that must be stopped are shut down, then the node is cordoned and drained (`DrainNode`). | `nodedrain-controller` |
| `True` | `Evacuating` | The drain returned successfully (handoff); waiting for the last `VirtualMachineInstance`s to leave the node and restarting shut-down VMs. | set by `nodedrain-controller`, acted on by `maintain-controller` |
| `True` | `Completed` | The node is fully drained and in maintenance mode. | `maintain-controller` |
| `False` | `Error` | Maintenance mode could not be enabled. The `message` describes the cause (e.g. control-plane quorum, non-migratable VMs, or a drain failure). | `nodedrain-controller` |

`status == True` therefore means "this node is (being) taken into maintenance" and is the signal that other controllers (e.g. the HA quorum check) treat the node as unavailable. `status == False` with `reason: Error` means maintenance mode is **not** engaged and the attempt failed.

### Why a single `Error` reason

The review discussion on #9022 suggested a formal set of *failure* reasons (e.g. `FailedPreCheck`, `ConstraintViolation`, `DrainFailed`) so that automation could branch on them. We investigated this against the Rancher/Wrangler summary framework that Harvester relies on to render resource errors in the UI ([`pkg/summary/summarizers.go`](https://github.com/rancher/wrangler/blob/73fdb33d6a7529e14e2f769b66f717468a09201d/pkg/summary/summarizers.go#L320)).

For a **Node**, that framework flags a condition as an error only when:

- the condition `reason` is **exactly** the string `Error`, or
- the condition `type` is one of `OutOfDisk` / `MemoryPressure` / `DiskPressure` / `NetworkUnavailable` (the hard-coded `Node` entry in `GVKConditionErrorMapping`).

The generic `Failed` / `Stalled` fallback does **not** apply to Nodes, because the Node GVK is matched explicitly. Therefore, a custom `MaintenanceMode` condition with any reason other than `Error` (such as `MigrationBlocked`) would **not** be surfaced in the `Hosts` page at all.

Modifying Wrangler is out of scope and not realistic for this enhancement. We accept the framework's behavior as-is and adapt to it:

- All failures use **`reason: Error`** so the error reliably appears in the UI.
- The **specifics are conveyed through the `message`**. The existing controller errors already produce clear, distinct messages for the quorum, non-migratable-VM, and drain-failure cases.

Multiple machine-readable failure reasons are therefore explicitly out of scope: they could not be displayed and would add complexity without user-visible benefit. Note that the granular *lifecycle* reasons (`Validating`/`Draining`/`Evacuating`) are unaffected by this constraint, which only governs how *errors* are surfaced.

### `clearMaintenanceMode` action

Because a failed condition (`status: False`, `reason: Error`) is intentionally retained so the error stays visible, a user needs a way to dismiss it without successfully completing maintenance mode. A new custom node action `clearMaintenanceMode` removes the `MaintenanceMode` condition when it is in the failed state.

### State Transition Diagram

The `MaintenanceMode` condition follows this state machine. (`status` is shown in parentheses.)

```
                              [No Condition]
                                    │
                                    │ user enables maintenance mode
                                    │ (sets drain-requested [+ drain-forced])
                                    ▼
                          ┌─────────────────────┐
                          │  Validating (True)  │  read-only pre-checks
                          │                     │  (quorum + VM migratability)
                          └─────┬─────────┬─────┘
              pre-check fails ──┘         │ pre-checks pass
                  │                       ▼
                  │         ┌─────────────────────┐
                  │         │   Draining (True)   │  stop required VMs +
                  │         │                     │  cordon + DrainNode
                  │         └──────┬───────┬──────┘
                  │   drain fails ─┘       │ drain succeeds (handoff)
                  │       │                ▼
                  │       │   ┌─────────────────────┐
                  │       │   │  Evacuating (True)  │  wait for last VMIs +
                  │       │   │                     │  restart shut-down VMs
                  │       │   └──────────┬──────────┘
                  │       │              │ node empty AND restarts done
                  │       │              ▼
                  │       │     ┌─────────────────────┐
                  │       │     │   Completed (True)  │  node under maintenance
                  │       │     └──────────┬──────────┘
                  ▼       ▼                │
          ┌──────────────────────┐         │ disable maintenance mode
          │     Error (False)    │         │ (remove condition, uncordon)
          │  message describes   │         ▼
          │  the cause           │      [No Condition]
          └────┬────────────┬────┘
               │            │
  clearMaintenanceMode      │ retry enable
  (remove condition)        │ (drain-requested set again, optionally force)
               │            │
               ▼            ▼
       [No Condition]   [Validating] (restart cycle)
```

**Transition notes:**

- **No Condition → Validating (True):** Triggered when `nodedrain-controller` observes the `harvesterhci.io/drain-requested` annotation. The condition is created (or an existing `Error` condition is overwritten).
- **Validating → Error (False):** A pre-check fails: control-plane quorum would be violated, or non-migratable VMs are present and `force` was not requested. The `drain-requested`/`drain-forced` annotations are removed to stop the reconcile loop, and the condition is set to `status: False`, `reason: Error` with a message describing the cause.
- **Validating → Draining (True):** All pre-checks passed. The controller advances the condition; VMs that must be stopped are shut down and the cordon/drain step begins.
- **Draining → Error (False):** The cordon/drain step (`DrainNode`) failed or timed out. The drain annotations are removed and the condition is set to `Error` with the drain-failure message.
- **Draining → Evacuating (True):** `DrainNode` returned successfully. The `drain-requested`/`drain-forced` annotations are removed and the condition is set to `Evacuating`. This reason is the explicit handoff signal that `maintain-controller` acts on.
- **Evacuating → Completed (True):** `maintain-controller` confirms no `VirtualMachineInstance` remains on the node and has restarted VMs labelled `harvesterhci.io/maintain-mode-strategy=ShutdownAndRestartAfterEnable`.
- **Error → No Condition (clear):** User invokes `clearMaintenanceMode`; the condition is removed and the node returns to a clean state.
- **Error → Validating (retry):** User re-enables maintenance mode (optionally with `force`). `drain-requested` is set again and a new cycle starts; the `Error` condition is overwritten.
- **Any engaged state → No Condition (disable):** User disables maintenance mode; the condition is removed and the node is uncordoned.

### User Stories

#### Story 1: Clear visibility of maintenance mode progress
As a Harvester administrator, when I put a node into maintenance mode I want to see which phase it is in: still checking whether it is allowed (`Validating`), actively draining (`Draining`), waiting for the last workloads to leave (`Evacuating`), or fully in maintenance (`Completed`). Today I only see a `Cordoned`/`Maintenance` badge and have to inspect annotations to infer the actual progress.

#### Story 2: A clear error message when maintenance mode fails
As a Harvester administrator, if maintenance mode cannot be enabled I want a clear, human-readable explanation in the `Hosts` page, for example that a control-plane node is already in maintenance, that non-migratable VMs are blocking the drain (and that `force` would shut them down), or that the drain timed out. Today the operation fails with little feedback and often requires reading controller logs. The message must be specific enough that I know what to do next (wait, fix/force, or retry).

#### Story 3: Clearing a maintenance mode error
As a Harvester administrator, when maintenance mode has failed and I decide not to proceed right now, I want a `Clear Maintenance Mode Error` action to reset the node's condition back to a clean state without having to successfully complete or otherwise toggle maintenance mode.

### User Experience In Detail

1. The user opens the `Hosts` page and selects `Enable Maintenance Mode` for a node (optionally ticking `Force`).
2. As soon as `drain-requested` is observed, a `MaintenanceMode` condition with `status: True`, `reason: Validating` is created while the read-only pre-checks run.
3. When the pre-checks pass, the condition advances to `status: True`, `reason: Draining`: VMs that must be stopped are shut down and the node is cordoned and drained. This is the long-running phase.
4. If a pre-check (in `Validating`) or the drain step (in `Draining`) fails, the condition flips to `status: False`, `reason: Error` with a descriptive message. Because the reason is `Error`, the Wrangler summary surfaces the message below the node in the `Hosts` page, where it remains until the user retries or clears it.
5. When the drain returns successfully, the condition becomes `status: True`, `reason: Evacuating`. The node is already cordoned; Harvester waits for the last VM migrations and restart handling.
6. Once all workloads have left the node and any labelled VMs have been restarted elsewhere, the condition becomes `status: True`, `reason: Completed`.
7. `Disable Maintenance Mode` removes the condition entirely and uncordons the node.
8. When the condition is in the `Error` state, a `Clear Maintenance Mode Error` action is offered; selecting it removes the condition and clears the warning from the UI.

The `Validating`, `Draining`, `Evacuating`, and `Completed` reasons are not rendered as separate UI badges (Wrangler does not summarise them); they are visible via the node YAML / `kubectl`. The existing `Maintenance` badge behaviour is retained.

### API changes

There are no new CRDs. The standard `Node` object's `status.conditions` list gains a `MaintenanceMode` condition.

New Go constants:

```go
const (
	NodeConditionTypeMaintenanceMode corev1.NodeConditionType = "MaintenanceMode"

	// Lifecycle reasons (status == True).
	NodeConditionReasonValidating string = "Validating"
	NodeConditionReasonDraining   string = "Draining"
	NodeConditionReasonEvacuating string = "Evacuating"
	NodeConditionReasonCompleted  string = "Completed"

	// Failure reason (status == False). Must be exactly "Error" so that the
	// Rancher/Wrangler summary surfaces the message in the Hosts page.
	NodeConditionReasonError string = "Error"
)
```

#### Node resource formatter actions

**New action: `clearMaintenanceMode`**
- **Availability:** shown only when a `MaintenanceMode` condition exists with `status: False` / `reason: Error`.
- **Permission:** same permissions as `disableMaintenanceMode`.
- **Behavior:** removes the `MaintenanceMode` condition; does not touch annotations, cordon state, or VMs.
- **Idempotent:** safe to call repeatedly; succeeds even if the condition is already gone.
- **UI label:** `Clear Maintenance Mode Error`.

Existing actions `enableMaintenanceMode` / `disableMaintenanceMode` keep their semantics; their *visibility* logic moves from "annotation present?" to "is a `MaintenanceMode` condition present / engaged?" (see Design).

#### Example conditions

```yaml
# Validating: read-only pre-checks running
- type: MaintenanceMode
  status: "True"
  reason: Validating
  message: "Checking whether the node can enter maintenance mode"
  lastTransitionTime: "2026-06-01T10:29:00Z"
  lastHeartbeatTime:  "2026-06-01T10:29:00Z"

# Draining: stopping required VMs, cordoning and draining the node
- type: MaintenanceMode
  status: "True"
  reason: Draining
  message: "Draining the node"
  lastTransitionTime: "2026-06-01T10:29:05Z"

# Evacuating: drained, waiting for the last workloads to move off
- type: MaintenanceMode
  status: "True"
  reason: Evacuating
  message: "Waiting for VM migration and restart handling to complete"
  lastTransitionTime: "2026-06-01T10:30:00Z"

# Completed: node fully in maintenance mode
- type: MaintenanceMode
  status: "True"
  reason: Completed
  message: "Maintenance mode enabled"
  lastTransitionTime: "2026-06-01T10:35:00Z"

# Error caused by non-migratable VMs (force not set) — raised during Validating
- type: MaintenanceMode
  status: "False"
  reason: Error
  message: >
    Enabling maintenance mode is impossible. Non-migratable VMs found:
    default/ubuntu-vm cannot be migrated due to host affinity.
    Use 'force drain' to perform a collective shutdown.
  lastTransitionTime: "2026-06-01T10:29:02Z"

# Error caused by control-plane HA quorum — raised during Validating
- type: MaintenanceMode
  status: "False"
  reason: Error
  message: "enabling maintenance mode is impossible: another controlplane is already in maintenance mode, cannot place current node in maintenance mode"
  lastTransitionTime: "2026-06-01T10:29:02Z"
```

## Design

### Implementation Overview

`MaintenanceMode` is owned jointly by the two existing controllers; the `harvesterhci.io/maintain-status` annotation is removed. Each phase is selected by the *current* condition reason, so reconciles are resumable and no external trigger (such as annotation presence/absence) is needed for the handoff between controllers.

**`nodedrain-controller.OnNodeChange`:**

1. **Detect intent (→ `Validating`):** when `harvesterhci.io/drain-requested` is present and the condition is absent or `Error`, set the condition to `status: True`, `reason: Validating` and return.
2. **Pre-checks (reason `Validating`):**
   - Run `DrainPossible`. On `ErrNodeDrainNotPossible`, set `status: False`, `reason: Error` with the quorum message, remove the drain annotations, and stop.
   - Detect non-migratable VMs. If found and `force` is not set, set `status: False`, `reason: Error` with the non-migratable-VM message, remove the drain annotations, and stop.
   - If all pre-checks pass, advance the condition to `status: True`, `reason: Draining`.
3. **Drain (reason `Draining`):**
   - Shut down VMs that must be stopped (non-migratable under force, or `ShutdownAndRestartAfterEnable` strategy).
   - Run `DrainNode` (cordon + drain). On failure, set `status: False`, `reason: Error` with the drain-failure message, remove the drain annotations, and stop.
   - On success, remove the `drain-requested`/`drain-forced` annotations and set the condition to `status: True`, `reason: Evacuating` (handoff).

**`maintain-controller.OnNodeChanged`:**

4. Reconcile nodes whose `MaintenanceMode` condition reason is `Evacuating`:
   - If any `VirtualMachineInstance` is still on the node, requeue and wait.
   - Otherwise restart VMs labelled `ShutdownAndRestartAfterEnable` that were shut down for this node, then set the condition to `status: True`, `reason: Completed`.

**Disable (API handler):** remove the `MaintenanceMode` condition, uncordon the node, remove the drain taint, and remove any remaining drain annotations.

**Clear error (API handler / `clearMaintenanceMode`):** if the `MaintenanceMode` condition exists with `status: False` / `reason: Error`, remove it.

All failure messages are produced by the existing controller error values (`err.Error()`), so they remain distinct and descriptive even though they share the single `Error` reason.

### Consumers migrated off the annotation

The `harvesterhci.io/maintain-status` annotation is read in several places today; all are migrated to consult the `MaintenanceMode` condition instead. A node is considered "engaged in maintenance mode" when it has a `MaintenanceMode` condition with `status: True` (reasons `Validating`/`Draining`/`Evacuating`/`Completed`). A failed (`Error`) or absent condition is treated as not engaged.

| Location | Today (annotation) | After (condition) |
|----------|--------------------|-------------------|
| `pkg/api/node/formatter.go` | shows enable vs. disable based on annotation presence | based on `MaintenanceMode` condition engagement; also gates `clearMaintenanceMode` on `status: False` |
| `pkg/api/vm/handler.go` (`isDrained`) | annotation present → drained | condition `status: True` → drained (still also honours `drain-requested` and `Unschedulable`) |
| `pkg/webhook/resources/node/validator.go` | cordon/maintenance admission via annotation | via condition engagement |
| `pkg/util/drainhelper/helper.go` (`DrainPossible`) | counts CP nodes without the annotation as available | counts CP nodes without an engaged `MaintenanceMode` condition as available |
| `pkg/controller/master/node/maintain_controller.go` | reads/writes annotation | reads/writes condition |
| `pkg/controller/master/nodedrain/nodedrain_controller.go` | writes `running`, deletes annotation | writes condition |

The trigger annotations `harvesterhci.io/drain-requested` and `harvesterhci.io/drain-forced` are unchanged and continue to express user intent.

### UI integration

No dashboard change is required. The error message surfaces through the existing Wrangler summary because the failure condition uses `reason: Error` (the only reason Wrangler recognises for a Node). The lifecycle reasons (`Validating`/`Draining`/`Evacuating`/`Completed`) are not summarised by Wrangler and are simply visible in the node's conditions; the existing `Maintenance` badge is retained.

### Test plan

Conditions are validated through `Node.status.conditions` (UI `Edit YAML`, or CLI):

```bash
kubectl get node <NAME> -o jsonpath='{.status.conditions[*]}' | jq
kubectl get node <NAME> -o json | jq '.status.conditions[] | select(.type == "MaintenanceMode")'
```

Unit tests (controllers + `pkg/util` helpers, with fake clients):
- `Validating` is set on `drain-requested`.
- `Validating` → `Draining` when all pre-checks pass.
- `Validating` → `Error` when `DrainPossible` fails, with the quorum message.
- `Validating` → `Error` for non-migratable VMs without force, with the non-migratable-VM message.
- `Draining` → `Error` when `DrainNode` fails, with the drain-failure message.
- `Draining` → `Evacuating` when `DrainNode` succeeds, and the drain annotations are removed.
- `Evacuating` → `Completed` only after all VMIs are gone and `ShutdownAndRestartAfterEnable` VMs are restarted.
- `clearMaintenanceMode` removes the condition only when `status: False` / `reason: Error`.
- Condition helper functions (`SetNodeStatusCondition`, `FindNodeStatusCondition`, `RemoveNodeStatusCondition`).

Integration tests (`tests/integration/api`):
- **Successful enablement:** enable on a node, observe `Validating` → `Draining` → `Evacuating` → `Completed`.
- **Disablement:** disable, observe the condition removed and the node uncordoned.
- **Non-migratable VMs:** create a non-migratable VM, enable without force, observe `status: False` / `reason: Error` and the message in the UI; clear it with the action; re-enable with force and reach `Completed`.
- **Control-plane quorum:** trigger an HA-quorum violation, observe `reason: Error` with the quorum message, and confirm force does not bypass it.
- **Drain failure:** simulate a drain timeout, observe `reason: Error` with the drain message, confirm retry can succeed.
- **Migration target selection:** confirm `isDrained` based on the condition keeps an engaged node out of migration target selection.

### Upgrade strategy

No general migration is required. During a Harvester upgrade nodes are expected to be out of maintenance mode, so in normal operation there is no `harvesterhci.io/maintain-status` value to migrate.

**Exceptional case:** if a node still carries `harvesterhci.io/maintain-status=completed` when the upgrade runs, the upgrade logic writes the equivalent `MaintenanceMode` condition (`status: True`, `reason: Completed`) and removes the legacy annotation, so the user can disable maintenance mode normally afterwards. A node carrying `harvesterhci.io/maintain-status=running` (an in-flight transition) is treated as an invalid pre-upgrade state.

Rolling component updates are unaffected, and nodes not in maintenance mode are not touched by this enhancement.

## Note

On failure the `harvesterhci.io/drain-requested` and `harvesterhci.io/drain-forced` annotations are removed; retaining them would make the controller continuously re-attempt maintenance mode. The failed `MaintenanceMode` condition is intentionally kept so the error stays visible. The user therefore either re-triggers maintenance mode (e.g. with `force`) after addressing the cause, or dismisses the error with the `clearMaintenanceMode` action.

### Backward compatibility

- `harvesterhci.io/maintain-status` is **removed**. It is an internal Harvester implementation detail, is not part of any public API contract, and is not known to be consumed by external tooling; the `MaintenanceMode` condition supersedes it.
- `harvesterhci.io/drain-requested` and `harvesterhci.io/drain-forced` remain as internal lifecycle/intent markers and are unchanged.
