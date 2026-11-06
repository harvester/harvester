# Refactor auto-rotate-rke2-certs and Expose Certificate Management

## Summary

Harvester provides an `auto-rotate-rke2-certs` setting to automatically renew underlying RKE2 cluster certificates before expiration. Currently, this capability relies on dialing TCP port 6443 on `default/kubernetes` Service endpoints and delegating the rotation orchestration to embedded Rancher's CAPR planner (`clusters.provisioning.cattle.io`).

This enhancement proposal refactors the RKE2 certificate management and auto-rotation mechanism in Harvester to:
1. **Decouple from embedded Rancher**, aligning with Harvester's broader embedded Rancher decoupling initiative ([#10712](https://github.com/harvester/harvester/issues/10712), [#11792](https://github.com/harvester/harvester/issues/11792), [#11929](https://github.com/harvester/harvester/issues/11929)).
2. **Provide full certificate observability** across all cluster nodes (control-plane and worker nodes) by introducing two dedicated Custom Resource Definitions (CRDs):
   * `ClusterRKE2Certificate` — a cluster singleton and the **single user-facing entry point** for all certificate operations (cluster-level and node-level rotation requests).
   * `RKE2Certificate` — a per-node, **read-only observation** resource reporting the certificate inventory of each node.
3. **Execute sequential, non-disruptive rotations** coordinated by Harvester's master controller and executed by `harvester-node-manager`.
4. **Refactor the setting schema** from `expiringInHours` to `autoRenewalWindowHours` (with a maximum threshold of 30 days / 720 hours) while ensuring backward compatibility during upgrades.

### Related Issues

* https://github.com/harvester/harvester/issues/10407
* https://github.com/harvester/harvester/issues/11444
* https://github.com/harvester/harvester/issues/11792

## Motivation

### Current Limitations
1. **Incomplete Expiration Detection**:
   Dialing TCP port 6443 only inspects the `kube-apiserver` serving certificate. It cannot detect expiration dates for:
   * Client certificates (`admin`, `kube-controller-manager`, `kube-scheduler`, `kube-proxy`, `kubelet`, `supervisor`, `auth-proxy`).
   * `etcd` peer, server, and client certificates.
   * `kube-controller-manager` and `kube-scheduler` self-signed serving certificates.
   * Any certificate on worker nodes (`client-kubelet.crt`, `serving-kubelet.crt`, `client-kube-proxy.crt`).
2. **Zero Kubernetes API Observability**:
   Certificate expiration dates are logged as debug/info strings but never stored in any Kubernetes resource status. Users and the UI have no structured way to inspect certificate health.
3. **Coupling with Embedded Rancher & Race Conditions**:
   The current implementation bumps `spec.rkeConfig.rotateCertificates.generation` on `clusters.provisioning.cattle.io`. In production clusters, this has resulted in race conditions with the upgrade controller and left `clusters.cluster.x-k8s.io` stuck in `paused: true` state ([#11792](https://github.com/harvester/harvester/issues/11792)).

### Goals

* Reimplement `auto-rotate-rke2-certs` without leveraging embedded Rancher.
* Expose per-node certificate inventory and expiration dates as structured Kubernetes Custom Resources.
* Provide a single user-facing resource (`ClusterRKE2Certificate`) for all rotation operations, so RBAC, validation, and concurrency control live in one place.
* Implement an in-tree master controller and node agent in Harvester and `harvester-node-manager` that coordinates and executes certificate rotation without embedded Rancher.
* Rotate nodes sequentially (one node at a time in role order: `witness node` $\rightarrow$ `management nodes` $\rightarrow$ `worker nodes`) to maintain cluster stability; cluster-level rotation synchronizes all nodes' future expiration dates to the same 365-day cycle.
* Support automatic rotation (policy-driven) and manual cluster-level or node-level rotation.
* Rename `expiringInHours` to `autoRenewalWindowHours` (capped at 30 days) with full upgrade backward compatibility.

### Non-goals

* Automatic rotation of RKE2 Root/Intermediate Certificate Authority (CA) certificates. CA certs are valid for 10 years and require manual disruptive procedures (`rke2 certificate rotate-ca`). They will be monitored in `.status` for observability, but leaf auto-rotation will not rotate CAs.

## Proposal

### User Stories

#### Story 1: Comprehensive Certificate Observability
As an administrator, I want to inspect the expiration dates and status of all RKE2 certificates across all nodes (control plane and workers) via `kubectl` and the Harvester Web UI, so that I can monitor certificate health and prevent unexpected cluster outages.

#### Story 2: Reliable, Non-Disruptive Auto-Rotation
As an administrator, I want Harvester to automatically renew expiring RKE2 certificates within a configured window (e.g. 10 days before expiry) without taking down the cluster.

#### Story 3: Manual On-Demand Rotation (Cluster or Node)
As an administrator, I want the ability to manually trigger a certificate rotation across the entire cluster or on specific nodes without waiting for certificates to enter the auto-renewal expiration window.


### User Experience In Detail

* **Viewing Certificate Status via CLI**:
  ```bash
  $ kubectl get clusterrke2certificate
  NAME      PHASE   SCOPE   REQUEST-ID   CURRENT-NODE   AGE
  cluster   Idle                                        10d

  $ kubectl get rke2certificates
  NAME               EARLIEST-EXPIRY        CA-EXPIRY              READY   ROTATING   AGE
  harvester-node-0   2027-10-22T08:05:00Z   2036-10-18T08:00:00Z   True    False      30d
  harvester-node-1   2027-10-22T08:05:00Z   2036-10-18T08:00:00Z   True    False      30d
  harvester-node-2   2027-10-22T08:05:00Z   2036-10-18T08:00:00Z   True    False      30d
  ```
  Cluster-wide aggregates (e.g. the earliest expiration across nodes) are derived by the UI and controllers from `RKE2Certificate` CRs; they are not duplicated on `ClusterRKE2Certificate`.

* **Manually Triggering Cluster-Level Rotation**:
  ```bash
  kubectl patch clusterrke2certificate cluster --type merge \
    -p '{"spec":{"rotation":{"requestID":"'"$(uuidgen)"'","scope":"Cluster","nodeNames":null}}}'
  ```
  The phase transitions to `Rotating` with `scope: Cluster` and processes all nodes one by one in role order.

* **Manually Triggering Node-Level Rotation**:
  ```bash
  kubectl patch clusterrke2certificate cluster --type merge \
    -p '{"spec":{"rotation":{"requestID":"'"$(uuidgen)"'","scope":"Node","nodeNames":["harvester-node-1","harvester-node-2"]}}}'
  ```
  The phase transitions to `Rotating` with `scope: Node`. Only the listed nodes are rotated, sequentially in role order.

* **Retrying After a Failure**:
  When a rotation fails, the phase becomes `Failed` and `status.lastRotation` records the failing node and reason. The administrator retries by submitting a new `spec.rotation` with a new `requestID`.

* **Cancelling a Rotation**:
  ```bash
  kubectl patch clusterrke2certificate cluster --type merge -p '{"spec":{"rotation":{"cancel":true}}}'
  ```
  A pending request is discarded without touching any node. A running batch lets the node currently rotating finish, then stops before the next node. `status.lastRotation.result` becomes `Cancelled`. Auto-rotation stays suspended until an administrator submits a new request (see [Cancellation](#cancellation)).

* **`RKE2Certificate` is read-only**: there is no `spec`. Any attempt by a user to create, update, or delete it is rejected by the admission webhook.

### API Changes

Two CRDs are introduced under the `node.harvesterhci.io/v1beta1` group.

#### 1. `ClusterRKE2Certificate` (Cluster-Scoped Singleton, user-facing)

```yaml
apiVersion: node.harvesterhci.io/v1beta1
kind: ClusterRKE2Certificate
metadata:
  name: cluster
spec:
  # The single rotation request. Submitting a new requestID starts a rotation.
  # Written by users (manual) or by the master controller (auto-rotation).
  rotation:
    requestID: 5b0e4d7a-3c1f-4f7e-9a51-2f6a1c9d8e10
    scope: Node                    # Cluster | Node
    nodeNames:                     # Required iff scope == Node; must be empty for Cluster
      - harvester-node-1
      - harvester-node-2
    cancel: false                  # Set to true to cancel this request; the only change allowed while locked

status:
  phase: Rotating                  # Idle, Rotating, Failed

  # Last spec.rotation.requestID accepted into a rotation batch.
  observedRequestID: 5b0e4d7a-3c1f-4f7e-9a51-2f6a1c9d8e10

  # The in-flight rotation batch. Empty when phase is Idle or Failed.
  # This is also the command channel consumed by harvester-node-manager.
  currentRotation:
    requestID: 5b0e4d7a-3c1f-4f7e-9a51-2f6a1c9d8e10
    scope: Node
    targetNodes:                   # Membership snapshot taken when the batch starts (name + UID)
      - name: harvester-node-1
        uid: 3f0c...
      - name: harvester-node-2
        uid: 8a91...
    currentNode:
      name: harvester-node-2
      uid: 8a91...
    skippedNodes: []               # Targets removed (or replaced) before they were rotated
    startTime: "2026-10-30T08:00:00Z"

  # Result of the most recently finished batch.
  lastRotation:
    requestID: auto-20261022T080400Z
    scope: Cluster
    result: Succeeded              # Succeeded | Failed | Cancelled
    failedNode: ""
    skippedNodes:
      - harvester-node-3
    cancelledNodes: []             # Targets not rotated because the batch was cancelled
    message: ""
    startTime: "2026-10-22T08:04:00Z"
    completionTime: "2026-10-22T08:16:00Z"

  conditions:
    - type: Ready
      status: "False"
    - type: Rotating
      status: "True"
    - type: Paused                 # True while rollout is held (reason: UpgradeInProgress | PromotionInProgress)
      status: "False"
    - type: AutoRotationSuspended  # True after a Failed or Cancelled batch; see "Auto-Rotation Suspended"
      status: "False"
```

Go types (abridged):

```go
type ClusterRKE2CertificateSpec struct {
    // Rotation is the current rotation request. Setting a new RequestID starts a rotation.
    // +optional
    Rotation *RotationRequest `json:"rotation,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self.scope == 'Node' ? size(self.nodeNames) > 0 : !has(self.nodeNames) || size(self.nodeNames) == 0",message="nodeNames is required for scope Node and must be empty for scope Cluster"
type RotationRequest struct {
    // Opaque, client-generated identifier; a UUID is recommended.
    // The "auto-" prefix is reserved for the master controller.
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=64
    // +kubebuilder:validation:Pattern=`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`
    RequestID string `json:"requestID"`

    // +kubebuilder:validation:Enum=Cluster;Node
    Scope RotationScope `json:"scope"`

    // +optional
    // +listType=set
    NodeNames []string `json:"nodeNames,omitempty"`

    // Cancel discards a pending request, or stops a running batch after the
    // current node finishes. It can only change from false to true.
    // +optional
    Cancel bool `json:"cancel,omitempty"`
}
```

**Request semantics**:
* `spec.rotation` is a single imperative request. A request is *pending* when `spec.rotation` is set and `spec.rotation.requestID != status.observedRequestID`.
* `requestID` is opaque and carries no ordering. Clients do not need to read the current value before writing a new one.
* When the master controller starts a batch, it sets `status.observedRequestID` and `status.currentRotation` in one status update. A request is consumed once, whether the batch succeeds or fails. Retrying means submitting a new `requestID`.
* `spec.rotation` is **locked** while a request is pending or a batch is running: any modification, including removal, is rejected, **except** setting `cancel: true` on the current request. `spec.rotation` therefore always describes the request being processed, or the last one processed.
* When the cluster is unlocked (`phase` is `Idle` or `Failed` and no request is pending), the only meaningful change is submitting a new request. Removing `spec.rotation` is allowed but has no effect.
* At most one batch exists at any time. See [Serializing Rotations](#serializing-rotations).
* **Auto-rotation uses the same path**: the master controller writes `spec.rotation = {requestID: "auto-<UTC timestamp>", scope: Cluster}`. Manual and automatic rotations are therefore indistinguishable downstream except by the reserved `auto-` prefix, which also tells the UI what triggered a batch.
* **Revert caveat**: any new non-empty `requestID` is a new request. Restoring an older manifest (GitOps revert, `kubectl apply` of an old file, backup restore) triggers a rotation if the restored ID is accepted by the webhook. `spec.rotation` should not be kept in declarative manifests; the UI and docs should say so. The request ID uniqueness check (see webhook) rejects most such replays.

#### 2. `RKE2Certificate` (Per-Node CR, observation only)

```yaml
apiVersion: node.harvesterhci.io/v1beta1
kind: RKE2Certificate
metadata:
  name: harvester-node-0
  ownerReferences:
    - apiVersion: v1
      kind: Node
      name: harvester-node-0
      uid: <node-uid>
# No spec: this resource is written exclusively by harvester-node-manager.
status:
  earliestExpirationTime: "2027-10-22T08:05:00Z"    # Earliest leaf cert NotAfter (drives auto-rotate)
  caEarliestExpirationTime: "2036-10-18T08:00:00Z"  # Earliest CA cert NotAfter (observability only)
  lastScanTime: "2026-10-30T08:30:00Z"

  rotation:
    requestID: auto-20261022T080400Z   # Request this node last acted on
    state: Succeeded                   # Rotating | Succeeded | Failed
    message: ""
    lastRotatedTime: "2026-10-22T08:05:00Z"

  conditions:
    - type: Ready
      status: "True"
    - type: Rotating
      status: "False"

  certificates:
    - service: api-server
      filename: serving-kube-apiserver.crt
      subject: kube-apiserver
      issuer: rke2-server-ca@1749726675
      isCA: false
      usages: ["ServerAuth"]
      notBefore: "2026-10-22T08:05:00Z"
      notAfter: "2027-10-22T08:05:00Z"
      status: OK
```

#### 3. Setting Schema Refactor (`auto-rotate-rke2-certs`)
The setting `auto-rotate-rke2-certs` retains its role as **declarative policy**:
```json
{
  "enable": false,
  "autoRenewalWindowHours": 240
}
```
* **Field Rename**: `expiringInHours` $\rightarrow$ `autoRenewalWindowHours`. The legacy key is still accepted on read and write for compatibility (e.g. scripts or Terraform), and is treated as `autoRenewalWindowHours`. Specifying both keys is rejected.
* **Validation Bounds**: Minimum `72` hour (3 days); Maximum `720` hours (30 days). They apply to new or changed values only; see [Upgrade Strategy](#upgrade-strategy) for existing values.

## Design

### Implementation Overview

#### Data Flow

```
 user / UI ─────┐
                ├─writes spec.rotation─▶ ClusterRKE2Certificate ◀──reads status── harvester-node-manager
 master ctrl ───┘  (auto-rotation)          │      ▲                                      │
                                            │      └── master ctrl writes status          │
                                            ▼          (currentRotation)                  │
                         master ctrl reads RKE2Certificate.status ◀──writes status────────┘
```

* `spec.rotation` is written by users (manual requests) and by the master controller (auto-rotation requests only).
* `ClusterRKE2Certificate.status` is written only by the master controller.
* `RKE2Certificate.status` is written only by the `harvester-node-manager` on the corresponding node.
* No status field on `ClusterRKE2Certificate` duplicates data derivable from `RKE2Certificate` (per-node expiry, per-node last rotation time, rotation progress).

#### Component Responsibilities

1. **Master Controller (`harvester` pod)**:
   * Reconciles a single object, `ClusterRKE2Certificate/cluster`. It **watches** (read-only, never writes) the `auto-rotate-rke2-certs` Setting, `Node`s, `Upgrade`s, and `RKE2Certificate` CRs, and enqueues `cluster` when any of them changes. `RKE2Certificate` is an input for three decisions:
     * **Auto-rotation**: whether any node's `status.earliestExpirationTime` is inside the renewal window.
     * **Rollout progress**: whether the current node reports `status.rotation` `Succeeded` or `Failed` for the current request, which is how the controller knows when to move to the next node.
     * **Reuse guard**: whether a target node already holds the new `requestID`.
   * Because entering the renewal window is time-driven rather than event-driven, `cluster` is also re-enqueued periodically (e.g. hourly).
   * **Auto-rotation**: When auto-rotation is enabled, `phase == Idle`, `AutoRotationSuspended` is not `True`, no request is pending, and any node has `earliestExpirationTime < Now() + autoRenewalWindowHours`, writes `spec.rotation = {requestID: "auto-<UTC timestamp>", scope: Cluster}`.
   * **Auto-rotation suspension**: The controller sets `AutoRotationSuspended=True` (reason `LastRotationFailed` or `LastRotationCancelled`) when a batch ends `Failed` or `Cancelled`. After a failure, this keeps a broken node from being rotated in a loop. After a cancel, it keeps auto-rotation from immediately re-submitting what the administrator just cancelled. It is cleared when either:
     * the controller starts a batch for a new request, or
     * no node is inside the renewal window. Suspension only needs to hold while auto-rotation would otherwise fire right away. Once nothing is due, clearing it means auto-rotation resumes by itself when the next window arrives, instead of being blocked indefinitely by an old failure or cancel.

     See [Auto-Rotation Suspended](#auto-rotation-suspended) for the user experience.
   * **Cancelling a pending request**: Checked before starting a batch. If the pending request has `cancel: true`, the controller sets `observedRequestID`, records `lastRotation` with `result: Cancelled` (all would-be targets in `cancelledNodes`), sets `phase: Idle`, and starts nothing. Because cancel is evaluated first, a request cancelled while the controller was down never runs when the controller comes back.
   * **Starting a batch**: When a request is pending, not cancelled, and `phase` is `Idle` or `Failed`. The controller never preempts a running batch: a request that becomes pending while `phase == Rotating` (only possible if the webhook is bypassed) stays pending until the current batch finishes. Then:
     * Resolves target nodes (all nodes for `Cluster`, `nodeNames` for `Node`) and records each as `{name, uid}` in `targetNodes`. Names that no longer exist are recorded in `skippedNodes`. If no target remains, the batch completes immediately with `result: Succeeded` and all names in `skippedNodes`; a removed node is not a failure and must not block auto-rotation.
     * **Reuse guard** (defense in depth, see [Request ID Uniqueness](#request-id-uniqueness)): if any target's `RKE2Certificate.status.rotation.requestID` already equals the new `requestID`, it records `lastRotation` with `result: Failed, message: "requestID already used"` and sets `phase: Failed` without rotating anything.
     * Otherwise writes `observedRequestID`, `currentRotation`, and `phase: Rotating` in one status update.
   * **Sequential rollout**:
     * **Pause**: Before each node, if a Harvester Upgrade is in progress or a node promotion is running, the controller does not select the next node. It sets `Paused=True` (reason `UpgradeInProgress` or `PromotionInProgress`), keeps `phase: Rotating`, and requeues. A node already rotating is allowed to finish. When the upgrade or promotion completes, it clears `Paused` and continues with the remaining targets. Remaining targets are re-validated (UID, role) at that point. The batch stays locked while paused; only `cancel` is accepted.
     * Before each node, checks cluster health (etcd quorum healthy, previously rotated node Ready). If a check fails, it requeues and does not advance.
     * **Pre-flight on the next node**: it must be `Ready=True` and its `harvester-node-manager` pod must be running. If not, the controller fails the batch immediately with `failedNode` and a reason (e.g. `node NotReady`) rather than waiting for the rotation timeout. Cordon (`spec.unschedulable`) and maintenance mode are **not** checked; such nodes are rotated normally (see [Cordoned and Maintenance-Mode Nodes](#node-lifecycle-handling)).
     * A target is **gone** when no Node with its `name` exists, or the Node's UID differs from the recorded `uid` (removed and re-joined under the same hostname). Gone targets are appended to `skippedNodes` and never rotated; a re-joined node already has fresh certificates from bootstrap.
     * Picks the next target that is not gone and whose `RKE2Certificate.status.rotation` is not `{requestID: currentRotation.requestID, state: Succeeded}`. Role order (`witness node` $\rightarrow$ `management nodes` $\rightarrow$ `worker nodes`) is **recomputed from current Node labels at every step** rather than fixed at batch start, so a worker promoted to a management node mid-batch (e.g. after a management node is removed) is rotated in its new role position. Sets it as `currentRotation.currentNode` (`{name, uid}`). Progress is derived this way rather than stored.
     * Once every target is either gone or satisfies that condition (and is `Ready=True`), moves `currentRotation` to `lastRotation` with `result: Succeeded` and sets `phase: Idle`.
     * **Per-node timeout**: Each node has a deadline (default 30 minutes) measured from when it becomes `currentNode`. It covers the whole step: the agent picking it up, the rotation, and the node returning to `Ready=True`. No wait in the rollout is unbounded.
     * If the current node reports `state: Failed` for this `requestID`, or misses its deadline while its Node still exists with the same UID, moves `currentRotation` to `lastRotation` with `result: Failed` and `failedNode`, and sets `phase: Failed`. Remaining nodes are not rotated. A node that is unreachable (`NotReady`) but not deleted is a failure, not a skip: only Node deletion removes a target from the batch.
     * If the current node becomes gone mid-rotation, the controller adds it to `skippedNodes`, clears `currentNode`, and re-runs the cluster health check (etcd quorum with the reduced member set) before continuing.
     * **Cancelling a running batch**: When `spec.rotation.cancel` becomes `true` for `currentRotation.requestID`, the controller stops selecting new nodes. If a node is currently rotating, it waits for that node to reach `Succeeded` or `Failed` (or time out); `rke2 certificate rotate` is never interrupted. It then moves `currentRotation` to `lastRotation` with `result: Cancelled`, lists the targets never started in `cancelledNodes`, and sets `phase: Idle`. If the in-progress node fails, `result: Failed` takes precedence and `phase` becomes `Failed`.

2. **Node Agent Controller (`harvester-node-manager` DaemonSet on each node)**:
   * **Inspection**: Periodically scans `/var/lib/rancher/rke2/{server,agent}/tls` (and static pod certs for KCM and Scheduler). Creates its node's `RKE2Certificate` if missing, and updates `.status.certificates`, `.status.earliestExpirationTime`, and `.status.caEarliestExpirationTime`.
   * **Rotation Execution**: Watches `ClusterRKE2Certificate/cluster`. Acts when `status.currentRotation.currentNode` matches both its own Node name **and UID**, and either `status.rotation.requestID != currentRotation.requestID` (new request) or `status.rotation.state == Rotating` (resuming after an agent restart):
     1. Sets `status.rotation = {requestID: currentRotation.requestID, state: Rotating}` and conditions `Rotating=True, Ready=False`.
     2. Stops local `rke2-server` (or `rke2-agent`). The RKE2 systemd units use `KillMode=process`, so workload containers and the agent's own pod keep running while the service is stopped.
     3. Runs `rke2 certificate rotate` and records its result.
     4. **Always** starts `rke2-server` (or `rke2-agent`), whether step 3 succeeded or not. A failed rotation must never leave the service stopped.
     5. Waits for local service and node readiness, bounded by the remaining per-node deadline.
     6. On success, re-scans certificates on disk and verifies the renewed `NotAfter`.
     7. Reports the outcome:
        * **Succeeded**: rotate succeeded, service started, node Ready, and certificates renewed. Sets `state: Succeeded`, updates `lastRotatedTime`, and sets `Rotating=False, Ready=True`.
        * **Failed, node recovered**: rotate failed but the service restarted and the node became Ready. Sets `state: Failed` with the `rke2 certificate rotate` error in `message`, and `Rotating=False, Ready=True`.
        * **Failed, node not recovered**: the service did not start, or the node did not become Ready. Sets `state: Failed` with a `message`, and `Rotating=False, Ready=False`. The batch fails (`phase: Failed`) and the node needs manual attention.
   * If the agent restarts mid-sequence (`state: Rotating` on restart), it first ensures the RKE2 service is started, then re-runs the sequence from step 2.
   * A `Failed` state for the current `requestID` is not retried by the agent.
   * In every failure case the agent reports `Failed` before the controller's per-node deadline, or the deadline fires. Either way the batch ends, `phase` leaves `Rotating`, and the cluster is never stuck in a rotation.

3. **Admission Webhook for `ClusterRKE2Certificate`**:
   * **Singleton**: Enforces `metadata.name == "cluster"` on Create and rejects Delete.
   * **Shape**: `scope: Node` requires a non-empty `nodeNames` of existing Nodes without duplicates; `scope: Cluster` requires empty `nodeNames` (also enforced by CRD CEL validation).
   * **Lock**: any change to `spec.rotation` (new request, edit, or removal) is rejected while `status.phase == Rotating` or a request is pending (`spec.rotation` set and `spec.rotation.requestID != status.observedRequestID`). This also arbitrates races between auto-rotation and a user: whichever write lands first wins, the other gets a clear error.
   * **Cancel exception**: while locked, the only accepted change is `cancel: false` $\rightarrow$ `true` with `requestID`, `scope`, and `nodeNames` unchanged. Any user allowed to update the object may cancel, including auto-rotation (`auto-`) requests. Unlike new requests, cancel is not blocked by an in-progress upgrade or promotion, so a batch that is `Paused` can still be cancelled.
   * **Cancel rules**: a new request must not set `cancel: true`; `cancel` can never change from `true` back to `false`; setting `cancel: true` while unlocked (nothing to cancel) is rejected.
   * **Immutability of a request**: when unlocked, `scope`, `nodeNames`, and `cancel` can change only together with a new `requestID`. Removing `spec.rotation` is allowed.
   * **Reserved prefix**: `requestID` values starting with `auto-` are accepted only from the `harvester` service account.
   * **Preconditions for a new request**: additionally rejected while a Harvester Upgrade is in progress or a node promotion is running. A batch that was already running when the upgrade started is paused rather than rejected (see [Interaction with Harvester Upgrade](#interaction-with-harvester-upgrade)).
   * **Uniqueness**: a new `requestID` is rejected if it equals `status.observedRequestID`, `status.lastRotation.requestID`, or any `RKE2Certificate.status.rotation.requestID`. See below.
   * **Status protection**: The webhook is also registered for the `clusterrke2certificates/status` subresource and rejects status writes from anyone except the `harvester` service account, so users cannot fake `phase: Idle` or `observedRequestID` to bypass the lock. Default Harvester RBAC roles grant no `update`/`patch` on the `status` subresource.
   * **Failure policy**: `failurePolicy: Fail`. If the webhook is unavailable, rotation requests are rejected rather than admitted unchecked.
   * **Bootstrap**: Created in `pkg/data/add.go` on cluster startup (with no `spec.rotation`) if not present.

4. **Admission Webhook for `RKE2Certificate`**:
   * Create/Update/Delete are allowed only from the `harvester-node-manager` and `harvester` service accounts (plus Kubernetes garbage collection). The node-manager may only write the CR matching its own node. Any other request is rejected with a message pointing to `ClusterRKE2Certificate`.
   * Default Harvester RBAC roles grant `get/list/watch` only.

### Serializing Rotations

A new rotation must never start before the current one is done. This is enforced in three independent layers.

1. **Admission (fast feedback)**: The lock rejects any change to `spec.rotation` other than cancellation while a batch is running or a request is pending. The lock is race-free because the lock state (`status.phase`, `status.observedRequestID`) lives on the **same object** as the request (`spec.rotation`):
   * Every write to `ClusterRKE2Certificate`, including status updates, bumps its `resourceVersion`.
   * The API server evaluates validating webhooks against the stored object (`oldObject`) and commits only if that version is still current. If the controller updated status in between, the write conflicts and is retried with fresh `oldObject` (for patches/apply) or returned as `409 Conflict` (for updates carrying a stale `resourceVersion`).
   * So the webhook never approves a request based on an outdated `phase`. This would not hold if the lock lived on a different object, which is another reason the request and the batch state share the singleton.
   * The window between a request being accepted and the controller starting the batch is covered by the *pending* check (`spec.rotation.requestID != status.observedRequestID`), not by `phase`.
2. **Controller (authoritative)**: Only the master controller (single active replica via leader election) starts batches, and only when `phase` is `Idle` or `Failed`. Starting a batch is a single status update (`observedRequestID` + `currentRotation` + `phase: Rotating`) guarded by `resourceVersion`, so two batches cannot be started concurrently. A request admitted despite the webhook is queued, never preemptive.
3. **Node agent (execution)**: `harvester-node-manager` never reads `spec`. It acts only on `status.currentRotation.currentNode`, which only the controller sets, one node at a time. Changing `spec.rotation` therefore cannot directly reach any node.

Layer 2 alone guarantees correctness. Layer 1 provides immediate, explicit errors instead of silently queued requests. Layer 3 ensures that the only path to a node is through the controller's serialized batch.

### Cancellation

`spec.rotation.cancel` is the escape hatch for the lock. It is part of the API rather than an out-of-band bypass, so every cancellation is audited, visible on the object, and recorded in status.

| State when `cancel: true` is set | Controller behavior | Result |
|---|---|---|
| Request pending (not yet started) | Discards it; no node is touched. | `lastRotation.result: Cancelled`, `phase: Idle` |
| Request pending, controller down | Cancel waits on the object; on restart the controller evaluates `cancel` before starting any batch, so the request never runs. | Same as above |
| Batch running, between nodes | Stops immediately. | `Cancelled`; remaining targets in `cancelledNodes` |
| Batch running, a node mid-rotation | Lets that node finish (rotation is never interrupted), then stops. | `Cancelled` if the node succeeds; `Failed` if it fails |
| Unlocked (nothing pending or running) | Rejected by the webhook. | — |

Notes:
* **The node agent is unaware of cancellation.** It never reads `spec`, and the controller simply stops assigning `currentNode`. Cancellation therefore cannot leave a node half-rotated.
* **Mixed expiry dates**: a cancelled cluster batch leaves rotated and non-rotated nodes on different 365-day cycles. The next cluster-scope request re-synchronizes them.
* **Auto-rotation suspension**: after `Cancelled`, `AutoRotationSuspended=True`, so a cancelled auto-rotation is not immediately re-submitted. See [Auto-Rotation Suspended](#auto-rotation-suspended).
* **Request ID**: a cancelled request's `requestID` is recorded in `observedRequestID` and `lastRotation`, so the [uniqueness check](#request-id-uniqueness) prevents it from being replayed.

### Auto-Rotation Suspended

`AutoRotationSuspended=True` means that auto-rotation is enabled but intentionally holding back after the last batch ended `Failed` or `Cancelled`, while at least one node is inside the renewal window. Certificates will expire if nobody acts, so the user experience is built around making the next step obvious.

**Where it is shown**
* **Harvester UI**: a dismissible-per-session banner on the dashboard and the Certificates page, plus a status badge on `ClusterRKE2Certificate`.
* **Kubernetes Event**: a `Warning` Event `AutoRotationSuspended` on `ClusterRKE2Certificate/cluster` when the condition becomes true, repeated daily while it stays true.
* **CLI**: the condition's `reason` and `message` on `kubectl get clusterrke2certificate cluster -o yaml`.

**Severity escalation** (based on the earliest `earliestExpirationTime` across nodes)
* *Warning*: earliest expiry is inside the renewal window.
* *Critical*: earliest expiry is within 24 hours, or a certificate has already expired. The banner can no longer be dismissed.

**Banner content**
* What happened: "Automatic certificate rotation is suspended because the last rotation `<failed on node X | was cancelled>` at `<time>`."
* What is at stake: "Certificates on `<N>` node(s) expire within `<window>`; earliest: `<date>` on `<node>`."
* Actions: **View details** (opens `lastRotation` and per-node certificate status) and **Rotate now**.

**What the administrator should do**

| Reason | Steps |
|---|---|
| `LastRotationFailed` | 1. Open **View details** and read `lastRotation.failedNode` and `message`. 2. Check the node: `harvester-node-manager` logs, `journalctl -u rke2-server` (or `rke2-agent`), disk space, time sync. 3. Fix the cause, or remove the node if it is permanently gone. 4. Click **Rotate now**, which submits a `scope: Cluster` request so every node returns to the same 365-day cycle. Alternatively, submit a `scope: Node` request for the failed node and the nodes in `cancelledNodes`/not yet rotated. |
| `LastRotationCancelled` | 1. Decide when rotation can run (e.g. after a maintenance window). 2. Click **Rotate now** at that time. Waiting is safe only until the escalation reaches *Critical*. |

Either action clears the suspension once the new batch starts. If the administrator does nothing and nodes are later no longer inside the window (e.g. after a manual node replacement), the suspension clears by itself and auto-rotation resumes normally.

### Interaction with Harvester Upgrade

A rotation batch and a Harvester upgrade both restart RKE2 on nodes, so they must not touch nodes at the same time:

* **Upgrade starts while a batch is running**: the upgrade is allowed. The rotation controller lets the node currently rotating finish, then pauses the batch (`Paused=True`, reason `UpgradeInProgress`). The upgrade controller waits until no `RKE2Certificate` reports `status.rotation.state: Rotating` before starting its first stage that touches nodes. The wait is bounded by the per-node rotation deadline.
* **After the upgrade completes**: the batch resumes with the remaining targets. Nodes that were removed or replaced during the upgrade are handled as described in [Node Lifecycle Handling](#node-lifecycle-handling).
* **Request submitted during an upgrade**: rejected by the webhook with a message to retry after the upgrade.
* **Cancelling while paused**: allowed. The batch ends `Cancelled` without rotating any more nodes.
* Node promotion is handled the same way, with reason `PromotionInProgress`.

### Request ID Uniqueness

Without a controller-generated batch counter, the node agent and the master controller identify a batch solely by `requestID`. This is only safe if a target node never already holds the new `requestID` from an earlier batch. Otherwise the following silent skip happens:

1. Node-scope request `a` rotates `node-1` $\rightarrow$ `node-1.status.rotation = {a, Succeeded}`.
2. Node-scope request `b` rotates `node-2`. `node-1` is untouched and still holds `a`.
3. A user re-submits `a` targeting `node-1`. Both the agent and the controller see `{a, Succeeded}` on `node-1` and treat it as already done. Nothing is rotated, yet the batch reports `Succeeded`.

A node can only mistake a new batch for an old one if its own `status.rotation.requestID` equals the new ID. The set of IDs that can cause harm is therefore exactly the set currently recorded on `RKE2Certificate` CRs (plus the last observed one). The design enforces this in two layers:

* **Webhook (primary)**: rejects a new `requestID` that appears in `status.observedRequestID`, `status.lastRotation.requestID`, or any `RKE2Certificate.status.rotation.requestID` (read from the informer cache). The set is consistent at admission time: node rotation IDs only change during a batch, and new requests are rejected while a batch is pending or running.
* **Controller (fallback)**: when starting a batch, if any target node already holds the `requestID`, the batch fails immediately with `requestID already used` rather than silently succeeding. This covers webhook bypass or cache lag.

IDs that no node holds any more (for example, used only on nodes since rotated by a later batch, or on deleted nodes) may be reused safely, because no node can confuse them with its current state. UUIDs and timestamp-based `auto-` IDs make collisions practically impossible; the guard exists for copy-paste and manifest replays.

### Node Lifecycle Handling

* **Adding a New Node**:
  * RKE2 generates fresh 365-day certs during bootstrap.
  * `harvester-node-manager` creates the node's `RKE2Certificate` with an empty `status.rotation`.
  * A node joining during a batch is not in the `targetNodes` snapshot and is not rotated in that batch.
* **Removing a Node**:
  * `RKE2Certificate` has an `ownerReference` pointing to its `corev1.Node`; Kubernetes garbage collection automatically cleans up the CR.
  * No spec cleanup is needed: a consumed request is inert, so a node re-joining with the same hostname is never rotated by a stale `nodeNames` entry.
  * The node's `status.rotation.requestID` disappears from the [uniqueness](#request-id-uniqueness) set. This is safe: no remaining node holds that ID, and a re-joined node starts with an empty `status.rotation`.
  * Removal at each point of a request's life:

    | When the node is removed | Effect |
    |---|---|
    | Before the request is submitted | Webhook rejects `nodeNames` that do not exist. |
    | After admission, before the batch starts | Recorded in `skippedNodes` at batch start; if no target remains, the batch succeeds immediately. |
    | During the batch, not yet rotated | Detected as gone (name missing or UID changed) and added to `skippedNodes`; the batch continues. |
    | During the batch, while it is `currentNode` | Agent is gone with the node; controller adds it to `skippedNodes`, re-checks etcd/cluster health, and continues. |
    | During the batch, already rotated | No effect; the batch completes normally. |
    | Removed and re-joined with the same hostname during the batch | UID mismatch: treated as gone and skipped. The new node's agent ignores `currentNode` because its UID differs, so a node still joining is never stopped mid-bootstrap. |
    | After the batch failed on that node (`phase: Failed`) | `phase` stays `Failed`. `AutoRotationSuspended` follows its normal rules: it clears when a new request starts, or by itself once no remaining node is inside the renewal window. |
* **Cordoned and Maintenance-Mode Nodes**:
  * Rotation does not depend on scheduling, so cordoned nodes and nodes in Harvester maintenance mode (cordoned and drained) are rotated like any other node:
    * `harvester-node-manager` is a DaemonSet. The DaemonSet controller automatically tolerates `node.kubernetes.io/unschedulable:NoSchedule`, so the agent keeps running on, and is rescheduled to, cordoned nodes.
    * `kube-apiserver`, `kube-controller-manager`, `kube-scheduler`, and `etcd` are static pods managed by the local kubelet and are unaffected by cordon.
    * Rotation does not cordon, drain, or uncordon nodes, and does not change `spec.unschedulable`. A cordoned node stays cordoned afterwards.
  * Skipping them would be wrong: cordon is usually temporary, and a node uncordoned with expired certificates fails to rejoin. A drained node is in fact the lowest-risk node to rotate.
  * A maintenance-mode node that is powered off is `NotReady` and fails the pre-flight check with an explicit reason. The administrator either brings it back or removes it, then submits a new request (or a `scope: Node` request that excludes it).
* **Promoting a Node**:
  * The master controller pauses rollout (`Paused=True`, reason `PromotionInProgress`) while any node has `node.Annotations["harvesterhci.io/promote-status"] == "running"`, and the webhook rejects new requests during that time.
  * After promotion completes, `harvester-node-manager` detects `/server/tls` on the host and automatically appends server and etcd certificates to `.status.certificates`.

### Alternatives Considered

1. **Leverage Existing `NodeConfig` CRD instead of introducing `RKE2Certificate`**:
   * *Rejection Reason*: certificate is not a config.
1. **Node-Level Trigger in `RKE2Certificate.spec`**:
   * *Rejection Reason*: Splits user-facing operations across two kinds (RBAC, UI, GitOps must handle both), requires cross-object validation for concurrency, and forces the master controller to write `RKE2Certificate.spec` for batch rollouts, mixing controller state with user intent.
1. **Separate `spec.clusterRotation` and `spec.nodeRotations` fields**:
   * *Rejection Reason*: Two request fields can be pending at the same time, requiring precedence rules (cluster supersedes node) and per-node observed bookkeeping in status. Since only one batch runs at a time, a single request with an explicit `scope` models the operation directly.
1. **Monotonic Integer `generation` instead of `requestID`** (the pattern used by Rancher's `rkeConfig.rotateCertificates.generation`):
   * *Pros*: Ordered, so reverting or restoring an older manifest is ignored and never triggers a rotation.
   * *Rejection Reason*: Clients must read the current value before writing a higher one, and bare numbers cannot be correlated with a specific operation in audit logs or the UI. The revert risk is mitigated by the uniqueness check and documentation.
1. **Controller-Generated Integer Batch ID in `status.currentRotation`**:
   * *Pros*: The node agent can compare `id > applied`, making ID reuse irrelevant.
   * *Rejection Reason*: Introduces a second identifier alongside `requestID` for the same operation. The [uniqueness check](#request-id-uniqueness) removes the only hazard a separate counter protects against.
1. **Auto-Rotation Bypasses `spec` (status-only `trigger: Auto` batches)**:
   * *Rejection Reason*: Creates a second code path for starting batches and a second arbitration rule against manual requests. `ClusterRKE2Certificate.spec` is an imperative request interface rather than declarative configuration (the policy lives in the `auto-rotate-rke2-certs` setting), so the controller writing a request there does not cause meaningful GitOps drift.
1. **Other Escape Hatches for the Lock** (privileged-group bypass, force-unlock annotation, pending-request expiry, webhook checking the controller's leader lease):
   * *Rejection Reason*: None of them can stop a running batch, and the bypass/annotation/lease variants leave no record in status. Expiry only addresses stuck requests and needs a trusted submit timestamp. An explicit `cancel` field covers both stuck and running requests and records the outcome as `Cancelled`.
1. **Ephemeral Request CRs (e.g. `RKE2CertificateRotation`, Job-style)**:
   * *Rejection Reason*: Introduces a third kind, accumulates history objects that need TTL cleanup, and still requires a singleton lock to serialize requests.
1. **Store `clusterGen` in `auto-rotate-rke2-certs` Setting Value**:
   * *Rejection Reason*: Violates separation between declarative user policy and operational state. Controller updates cause drift in Terraform/GitOps and risk accidental overwrites during UI edits.
1. **Execute Rotation via Periodic Kubernetes Jobs**:
   * *Rejection Reason*: Inefficient for continuous 30-minute observability checks, high pod churn, and fails on cordoned/tainted nodes.
1. **Single-Node-Only Auto-Rotation (No Cluster Batch)**:
   * *Rejection Reason*: When nodes were joined at different times, single-node rotation results in frequent, scattered service restarts throughout the year. Batch rotation synchronizes all nodes to the same 365-day renewal cycle.

### Test Plan

#### Unit Tests
* `AutoRotateRKE2Certs` JSON unmarshaling and backward-compatibility tests (`expiringInHours` $\rightarrow$ `autoRenewalWindowHours`).
* Setting validating webhook test cases: bounds `72 <= hours <= 720` on new or changed values, rejection of negative or excessive values, both keys present rejected, and an unchanged legacy `Value` (e.g. `expiringInHours: 24`) accepted when only `Default` changes, as `SetAll` does.
* Setting migration: `24` $\rightarrow$ `72`, `240` unchanged, `2000` $\rightarrow$ `720`; legacy key rewritten to `autoRenewalWindowHours`; Event recorded on change.
* `ClusterRKE2Certificate` webhook: singleton name and deletion; `scope`/`nodeNames` shape; unknown or duplicate nodes; mutation of `scope`/`nodeNames` without a new `requestID`; `auto-` prefix from non-controller users; any change to `spec.rotation` (new, edit, removal) while `Rotating` or pending; new requests while upgrading or promoting; reuse of observed, last, or any node-held `requestID`; removal of `spec.rotation` allowed only when unlocked; `cancel: true` accepted while locked only with other fields unchanged, rejected when unlocked, rejected on new requests, and never reversible; status subresource writes rejected for non-controller users.
* `RKE2Certificate` webhook: rejects writes from non-system users and cross-node writes from node-manager.
* Controller: auto-rotation writes a request only when `Idle`, not suspended, with nothing pending; `AutoRotationSuspended` set after `Failed`/`Cancelled`, cleared on the next new request or when no node is inside the renewal window; `Paused` set during upgrade/promotion with no new node selected; per-node deadline fails the batch; a pending cancelled request is never started; cancel during a batch waits for the in-progress node and reports `Cancelled` or `Failed` accordingly; a request that becomes pending while `Rotating` is not started until the current batch finishes; reuse guard fails the batch; progress derivation from `RKE2Certificate.status.rotation`.
* Node sorting logic by role.

#### Integration & E2E Tests
* **CR Bootstrap**: Verify `ClusterRKE2Certificate/cluster` and per-node `RKE2Certificate` CRs are auto-created and populated.
* **Auto-Rotation Trigger**: Adjust expiration timestamp or setting window; verify the controller writes an `auto-` request and the cluster batch completes in role order.
* **Manual Cluster Rotation**: Submit `scope: Cluster` with a new `requestID`; verify all nodes rotate one at a time, `phase` returns to `Idle`, and `lastRotation.requestID` matches.
* **Manual Node Rotation**: Submit `scope: Node` with two `nodeNames`; verify only those nodes rotate in role order; verify a new request is rejected while `Rotating`.
* **Request ID Reuse**: Rotate `node-1` with `a`, then `node-2` with `b`, then submit `a` for `node-1`; verify the webhook rejects it.
* **Failure & Retry**: Inject a failure on one node; verify `phase: Failed`, `lastRotation.failedNode`, remaining nodes untouched, no auto-rotation request is written, and a new `requestID` retries.
* **Agent Restart**: Restart `harvester-node-manager` mid-rotation (including while RKE2 is stopped); verify it starts RKE2, resumes the same request, and the batch completes.
* **Failed `rke2 certificate rotate`**: Inject a rotate failure; verify the agent still restarts RKE2, the node returns to `Ready=True`, the node reports `Failed`, and the batch ends `Failed` instead of staying `Rotating`.
* **RKE2 Fails to Start**: Inject a service start failure; verify the batch fails at the per-node deadline at the latest.
* **Upgrade During Rotation**: Start a Harvester upgrade mid-batch; verify the in-progress node finishes, `Paused=True` with `UpgradeInProgress`, the upgrade waits for it before touching nodes, and the batch resumes after the upgrade. Verify cancel is accepted while paused and new requests are rejected during the upgrade.
* **Suspension UX**: Fail a batch while nodes are in the renewal window; verify the Warning Event and banner, escalation to Critical within 24h of expiry, and that **Rotate now** clears the suspension.
* **Read-only `RKE2Certificate`**: Verify user `kubectl edit/delete` is rejected.
* **Node Lifecycle**: Verify a node joining mid-batch is skipped, promotion adds server certs, and node deletion removes the CR.
* **Node Removal During Rotation**: Remove a not-yet-rotated target and the `currentNode` mid-batch; verify both appear in `skippedNodes` and the batch completes. Submit a `scope: Node` request, delete its only node before the batch starts; verify `result: Succeeded` with the node skipped. Remove and re-join a target under the same hostname mid-batch; verify it is skipped and the new node is not rotated. Power off (but do not delete) the `currentNode`; verify the batch fails on timeout instead of skipping.
* **Cancellation**: Cancel a cluster batch mid-rollout; verify the in-progress node completes, no further node starts, `result: Cancelled` with `cancelledNodes`, and `AutoRotationSuspended=True` (no auto request written even inside the renewal window). Stop the controller, submit then cancel a request, restart the controller; verify no node is rotated. Verify a subsequent manual request clears the suspension.
* **Cordoned / Maintenance-Mode Nodes**: Cordon one node and put another into maintenance mode; run a cluster batch; verify both rotate successfully and remain cordoned afterwards. Power off a maintenance-mode node; verify the batch fails at pre-flight with `node NotReady`.
* **Promotion Mid-Batch**: Remove a management node during a cluster batch so a worker is promoted; verify rollout pauses during promotion and the promoted node is rotated in management-node order.

### Upgrade Strategy

1. **CRD Installation**: `ClusterRKE2Certificate` and `RKE2Certificate` CRDs are bundled in `harvester-crd` and registered in `pkg/data/crd.go`.
2. **Singleton Bootstrap**: `pkg/data/add.go` initializes `ClusterRKE2Certificate/cluster` without `spec.rotation` if not present.
3. **Setting Migration**:
   * **Why the validator must tolerate existing values**: On startup, `SetAll` (`pkg/controller/global/settings/register.go`) updates the `Setting` through the API whenever its built-in `Default` changes. This upgrade changes it, because the default JSON switches to `autoRenewalWindowHours`. The setting validating webhook then validates the whole object, including the stored `Value` that `SetAll` did not modify (`validateUpdateAutoRotateRKE2Certs`). If the new bounds were applied to an unchanged legacy `Value`, `SetAll` would fail and block startup. The validator therefore applies the 72–720 bounds to `Default` and to a `Value` that differs from the old object, and accepts an unchanged `Value` as long as it parses.
   * **Normalization**: After `SetAll`, a one-time migration in the `harvester` pod rewrites a legacy `Value` to the new key, clamped into `[72, 720]`:
     * `expiringInHours` in `[72, 720]`: copied unchanged to `autoRenewalWindowHours`.
     * `expiringInHours` below 72 (1–71): raised to 72. This only renews certificates earlier than before, which is always safe.
     * `expiringInHours` above 720: lowered to 720. Renewal still happens 30 days before expiry.
     
     Every changed value is logged and recorded as a Kubernetes Event on the `Setting`, so administrators can see that their value was adjusted. The normalized value satisfies the new bounds, so later edits through the UI or API are not rejected because of a legacy value.
   * **Runtime compatibility**: Until normalization has run, the controller reads either key and applies the same clamping when evaluating the renewal window.
4. **Upgrade Webhook**: `pkg/webhook/resources/upgrade/validator.go` (`checkCerts`) is updated to compute the earliest `status.earliestExpirationTime` across `RKE2Certificate` CRs (via the informer cache), with a fallback to TCP dialing port 6443 if CRs are not yet populated. A running rotation does **not** block the upgrade. The batch pauses, and the upgrade controller waits for any node mid-rotation to finish before touching nodes (see [Interaction with Harvester Upgrade](#interaction-with-harvester-upgrade)).
5. **Decoupling Cleanup**: Removes the legacy syncer from `pkg/controller/master/setting/auto_rotate_rke2_certs.go` that updated `clusters.provisioning.cattle.io`.
