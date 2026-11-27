# Refactor auto-rotate-rke2-certs and Expose Certificate Management

## Summary

Harvester's `auto-rotate-rke2-certs` setting renews RKE2 certificates before they expire. Today it only inspects the `kube-apiserver` serving certificate, and it delegates the rotation to embedded Rancher.

This proposal moves certificate rotation into Harvester itself:

* Every node reports all of its RKE2 certificates in a new read-only `RKE2Certificate` resource.
* All rotations, automatic or manual, for the whole cluster or for selected nodes, are requested through one new singleton resource, `RKE2CertificateRotation`.
* The Harvester controller rotates one node at a time, and `harvester-node-manager` executes the rotation on each node. Embedded Rancher is no longer involved.
* The setting field `expiringInHours` is renamed to `autoRenewalWindowHours` and bounded to 72–720 hours.

### Related Issues

* https://github.com/harvester/harvester/issues/10407
* https://github.com/harvester/harvester/issues/11444
* https://github.com/harvester/harvester/issues/11792

## Motivation

* **Incomplete detection.** Dialing port 6443 only sees the `kube-apiserver` serving certificate. Client certificates, etcd certificates, and every certificate on worker nodes are never checked.
* **No visibility.** Expiration dates are only logged. Users and the UI cannot query them.
* **Coupling with embedded Rancher.** Rotation is triggered through `clusters.provisioning.cattle.io`. This has raced with upgrades and left clusters stuck in `paused: true` ([#11792](https://github.com/harvester/harvester/issues/11792)), and it blocks the embedded Rancher decoupling work ([#10712](https://github.com/harvester/harvester/issues/10712), [#11929](https://github.com/harvester/harvester/issues/11929)).

### Goals

* Rotate RKE2 certificates without embedded Rancher.
* Expose every node's certificates and expiration dates as Kubernetes resources.
* Offer one user-facing resource for automatic and manual rotation, cluster-wide or per node.
* Rotate one node at a time without interrupting workloads, and keep all nodes on the same 365-day cycle.
* Rename `expiringInHours` to `autoRenewalWindowHours` (72–720 hours) without breaking upgrades.

### Non-goals

* Rotating RKE2 CA certificates. They are valid for 10 years and need a manual, disruptive procedure. Their expiration is reported but never rotated.
* Coordinating with other operations that restart RKE2 through `rancher-system-agent` plans (for example applying the `containerd-registry` or `additional-ca` settings).

## Proposal

### User Stories

#### Story 1: See every certificate

Before: an administrator must log in to each node to check certificate expiration.

After: `kubectl get rke2certificates` and the Harvester UI show the expiration of every certificate on every node.

#### Story 2: Reliable auto-rotation

Before: auto-rotation watches one certificate and depends on embedded Rancher.

After: Harvester rotates all leaf certificates when any node enters the renewal window, one node at a time, while VMs keep running.

#### Story 3: Rotate on demand

Before: a manual rotation means editing a Rancher resource.

After: an administrator requests a rotation for the whole cluster or for selected nodes with one patch or one UI button.

### User Experience In Detail

**View certificates**

```bash
$ kubectl get rke2certificaterotation
NAME      PHASE   SCOPE   REQUEST-ID   CURRENT-NODE   AGE
cluster   Idle                                        10d

$ kubectl get rke2certificates
NAME               EARLIEST-EXPIRY        CA-EXPIRY              ROTATION    LAST-ROTATED           AGE
harvester-node-0   2027-10-22T08:05:00Z   2036-10-18T08:00:00Z   Succeeded   2026-10-22T08:05:00Z   30d
harvester-node-1   2027-10-22T08:05:00Z   2036-10-18T08:00:00Z   Succeeded   2026-10-22T08:09:00Z   30d
```

**Rotate the whole cluster, or selected nodes**

```bash
kubectl patch rke2certificaterotation cluster --type merge \
  -p '{"spec":{"request":{"requestID":"'"$(date -u +%Y%m%dT%H%M%SZ)"'","scope":"Cluster","nodeNames":null}}}'

kubectl patch rke2certificaterotation cluster --type merge \
  -p '{"spec":{"request":{"requestID":"'"$(date -u +%Y%m%dT%H%M%SZ)"'","scope":"Node","nodeNames":["harvester-node-1"]}}}'
```

The phase becomes `Rotating` and the nodes are rotated one by one. While a rotation is running, a new request is rejected.

`requestID` only has to differ from the previous request. A UTC timestamp is recommended, because it is easy to produce and readable in the history. The UI and API clients use the [action endpoint](#action-endpoint) instead and never handle a `requestID`.

A rotation takes about a minute per management node and 20 seconds per worker, and cannot be cancelled once it has started.

**When a rotation does not complete**

| Result | Meaning | What the administrator does |
|---|---|---|
| `PartiallySucceeded` | Some nodes were unreachable and were skipped. | Bring the nodes back or remove them. |
| `Interrupted` | An upgrade, a node promotion, or an unhealthy control plane stopped the rotation between nodes. Nothing is broken. | Nothing for auto-rotation; it starts again by itself. Submit a manual request again afterwards. |
| `Failed` | A node failed during its rotation; later nodes were not started. | Read the newest entry in `status.history`, fix the node, submit a new request. |

After `Failed`, automatic rotation is suspended until a new rotation starts. The UI shows a banner with a **Rotate now** button, and a `Warning` Event is emitted daily. The banner becomes non-dismissible when a certificate is within 24 hours of expiry.

### API changes

Both resources are cluster-scoped and live in `node.harvesterhci.io/v1beta1`.

#### `RKE2CertificateRotation` (singleton named `cluster`)

There is exactly one object. It is not created per rotation: each new rotation overwrites `spec.request`, and `status` tracks the running batch and the results of the last 10.

```yaml
spec:
  request:                         # the single rotation request
    requestID: 20261030T080000Z    # chosen by the client; a new value means a new request
    scope: Node                    # Cluster | Node
    nodeNames: [harvester-node-1]  # required for Node, empty for Cluster
status:
  phase: Rotating                  # Idle | Rotating | Failed
  observedRequestID: 20261030T080000Z  # last request the controller started
  currentRotation:                 # the running batch; empty unless phase is Rotating
    batchID: 0b9c2f6e-...          # generated by the controller for each batch
    requestID: 20261030T080000Z
    scope: Node
    targetNodes: [{name: harvester-node-1, uid: 3f0c...}]
    currentNode: {name: harvester-node-1, uid: 3f0c...}
    currentNodeStartTime: "2026-10-30T08:04:00Z"
    skippedNodes: []
    startTime: "2026-10-30T08:00:00Z"
  history:                         # finished batches, newest first; the last 10 are kept
    - requestID: auto-20261022T080400Z
      batchID: 6d2a8c11-...
      scope: Cluster
      result: PartiallySucceeded   # Succeeded | PartiallySucceeded | Interrupted | Failed
      rotatedNodes: [harvester-node-0, harvester-node-1, harvester-node-2]
      failedNode: ""
      skippedNodes: [{name: harvester-node-3, reason: Unreachable}]   # Gone | Unreachable
      notStartedNodes: []          # targets never started because the batch stopped early
      message: ""
      startTime: "2026-10-22T08:04:00Z"
      completionTime: "2026-10-22T08:16:00Z"
  conditions:
    - type: AutoRotationSuspended  # set after a Failed batch
```

Request rules:

* A request is **pending** when `spec.request.requestID` differs from `status.observedRequestID`.
* `requestID` makes a request safe to repeat: applying the same request twice rotates once.
* While a request is pending or a batch is running, `spec.request` cannot be changed.
* A change to the request without a new `requestID` is rejected, because the controller would ignore it.
* Auto-rotation writes the same field, with a `requestID` of `auto-<timestamp>` and `scope: Cluster`.
* `spec.request` is a one-shot request, not desired state. It should not be kept in GitOps manifests, because re-applying an old manifest starts one more rotation.

#### `RKE2Certificate` (one per node, no `spec`)

```yaml
metadata:
  name: harvester-node-0           # owned by the Node, garbage-collected with it
status:
  earliestExpirationTime: "2027-10-22T08:05:00Z"    # earliest leaf certificate; drives auto-rotation
  caEarliestExpirationTime: "2036-10-18T08:00:00Z"  # reported only
  lastScanTime: "2026-10-30T08:30:00Z"
  rotation:
    batchID: 6d2a8c11-...          # last batch this node acted on
    requestID: auto-20261022T080400Z
    state: Succeeded               # Rotating | Succeeded | Failed
    message: ""
    lastRotatedTime: "2026-10-22T08:05:00Z"
  certificates:
    - service: api-server
      filename: serving-kube-apiserver.crt
      subject: kube-apiserver
      issuer: rke2-server-ca@1749726675
      isCA: false
      usages: [ServerAuth]
      notBefore: "2026-10-22T08:05:00Z"
      notAfter: "2027-10-22T08:05:00Z"
```

#### Action endpoint

The Harvester API exposes a `rotate` action on the singleton, in the same way as the existing VM and node actions:

```
POST /v1/harvester/node.harvesterhci.io.rke2certificaterotations/cluster?action=rotate
{"scope": "Node", "nodeNames": ["harvester-node-1"]}
```

* It generates a timestamp `requestID` and writes `spec.request`.
* It is a convenience layer, not a second path: the write goes through the same webhook rules, and a rejected request is returned to the caller as an error.
* The caller needs `update` permission on the `RKE2CertificateRotation` object.

#### Setting `auto-rotate-rke2-certs`

```json
{ "enable": false, "autoRenewalWindowHours": 240 }
```

* `expiringInHours` is renamed to `autoRenewalWindowHours`. The old key is still accepted; setting both is rejected.
* Allowed range: 72–720 hours (previously 1–8759).

## Design

### Implementation Overview

#### Components

| Component | Responsibility | Writes |
|---|---|---|
| Master controller (`harvester` pod) | Decides when to auto-rotate, and drives a batch node by node. | `RKE2CertificateRotation.status`; `spec.request` for auto-rotation only |
| Node agent (`harvester-node-manager`) | Scans certificates every 30 minutes; rotates its own node when told to. | Its own `RKE2Certificate.status` |
| Action handler (`harvester` pod) | Turns a `rotate` API call into a write to `spec.request`. | `spec.request`, on behalf of the caller |
| Admission webhooks | Enforce the request rules and keep both resources' status trustworthy. | — |

The node agent never reads `spec`. It acts only on `status.currentRotation.currentNode`, which only the controller sets, so the controller is the single place where rotations are serialized. The controller keeps no state in memory and resumes a batch from `status` after a restart.

#### Batch flow (master controller)

1. A pending request starts a batch: the controller snapshots the target nodes, generates a `batchID`, and sets `phase: Rotating`.
2. It picks the next target in the order witness $\rightarrow$ management $\rightarrow$ worker (by name within a role) and sets `currentNode`.
3. It waits for that node's agent to report `Succeeded` for this `batchID` and for the node to be healthy again, then clears `currentNode` and repeats step 2.
4. When no target is left, or the batch has to stop early, it moves `currentRotation` to the front of `status.history` with the result, drops entries beyond the last 10, and returns to `Idle` (or `Failed`).

A batch never waits for something outside the rotation. Before picking each node, including the first, the controller checks:

| Situation | Behavior |
|---|---|
| Harvester upgrade or node promotion in progress | End the batch as `Interrupted`. |
| A management or witness node is not healthy | End the batch as `Interrupted`; rotating another etcd member could lose quorum. |
| Target node is `NotReady` or its agent is not running | Skip it (`Unreachable`) and continue. RKE2 renews near-expiry certificates by itself when the node starts again. |
| Target node was removed or replaced (UID changed) | Skip it (`Gone`). |
| Another node still reports `Rotating` and its agent is running | Do not start a batch or pick a node yet. The agent's deadline ends this within minutes. |

While a node is rotating:

| Situation | Behavior |
|---|---|
| The node reports `Succeeded` and is healthy | Move on to the next node. |
| The node reports `Failed`, or takes longer than 10 minutes | End the batch as `Failed`; do not start the remaining nodes. |

A node is **healthy** when it is `Ready` and, on management and witness nodes, its `etcd` and `kube-apiserver` pods are `Ready` and were started after the rotation began.

An `Interrupted` batch is not resumed. Rotating a node again is harmless and takes about a minute, so a later batch simply starts over: auto-rotation creates one as soon as its conditions hold again, and a manual request has to be submitted again.

The upgrade and promote controllers wait in the other direction: before touching a node, they wait until `phase` is no longer `Rotating`. A batch ends at the latest when its current node finishes, so this wait is bounded by the 10-minute per-node deadline.

#### Node rotation (node agent)

When `currentNode` names this node (name and UID) with a `batchID` it has not acted on, the agent:

1. Reports `state: Rotating`, writes a marker file on the host, and arms a systemd timer that starts RKE2 after 9 minutes.
2. Stops `rke2-server` or `rke2-agent`.
3. Runs `rke2 certificate rotate`.
4. Starts RKE2 again, whether step 3 succeeded or not.
5. Re-scans the certificates, cancels the timer, removes the marker, and prunes old certificate backups.
6. Reports `Succeeded` if RKE2 started and every leaf certificate was reissued, otherwise `Failed`.

Safeguards:

* **Deadline.** The agent gives up after 8 minutes, starts RKE2, and reports `Failed`. After that it never stops RKE2 for this batch, so a batch the controller has failed cannot still take a node down.
* **Safety timer.** Stopping RKE2 also stops the kubelet, so an agent that dies in the middle is not restarted. The timer guarantees RKE2 is started again.
* **Startup recovery.** An agent that starts and finds the marker file first makes sure RKE2 is running, then resumes the batch if it is still current, or reports `Failed`.

#### Auto-rotation

The controller writes an `auto-` request when all of these hold:

* the setting is enabled;
* `phase` is `Idle` and no request is pending;
* no upgrade or promotion is in progress, and every management and witness node is healthy;
* `AutoRotationSuspended` is not `True`;
* some `Ready` node has `earliestExpirationTime` within `autoRenewalWindowHours`.

`AutoRotationSuspended` is set when a batch ends `Failed`. It is cleared when a new batch starts, or when no `Ready` node is inside the window. `Interrupted` and `PartiallySucceeded` do not set it, because no node broke.

#### Admission webhooks

`RKE2CertificateRotation`:

* Only the object named `cluster` may exist, and it cannot be deleted.
* `scope: Node` requires existing node names; `scope: Cluster` requires none.
* While a request is pending or a batch is running, every change to `spec.request` is rejected.
* A change to `spec.request` must come with a new `requestID`, different from `status.observedRequestID`.
* New requests are rejected during an upgrade or a promotion.
* The `auto-` prefix and all status writes are reserved for the `harvester` service account.

`RKE2Certificate`:

* Only `harvester-node-manager` may write, and only the object for its own node. The node is taken from the node-name claim of the service account token.
* Users have read-only access.

### Test plan

Unit tests:

* Setting: both keys parse, bounds, both keys rejected, migration clamping.
* Webhooks: every rule above, with an accepted and a rejected case.
* Action handler: `rotate` writes a request with a generated `requestID`; a caller without `update` permission is refused; a webhook rejection is returned as an error.
* Controller: each row of the two situation tables; auto-rotation conditions and suspension; resuming a batch from `status` after a restart; `status.history` ordered newest first and capped at 10.
* Node agent: result taken from the re-scan; deadline; safety timer armed and cancelled; startup recovery.

Integration and end-to-end tests:

* Both resources are created and populated on a new cluster.
* Auto-rotation: move a certificate into the window; all nodes are rotated in role order.
* Manual rotation for the cluster and for selected nodes, through `kubectl` and through the action endpoint; a second request is rejected while one runs.
* Workloads: a running VM and a pod writing to a Longhorn volume see no interruption.
* Failures: `rke2 certificate rotate` fails; RKE2 fails to start; the agent is killed while RKE2 is stopped (the timer starts it).
* A powered-off worker is skipped; a powered-off management node ends the batch as `Interrupted`.
* An upgrade or a promotion that starts during a batch: the current node finishes, the batch ends as `Interrupted`, the upgrade or promotion waits for that, and auto-rotation starts a new batch afterwards.
* Single control-plane cluster: the `harvester` pod restarts in the middle of the batch and the batch still completes.
* Node removed, replaced, or added during a batch; cordoned and maintenance-mode nodes are rotated.

### Upgrade strategy

1. The two CRDs are added to `harvester-crd`, and `RKE2CertificateRotation/cluster` is created at startup with an empty `spec`.
2. Setting migration:
   * The validator applies the new bounds only to a changed value, so an existing out-of-range value does not block startup.
   * A one-time migration rewrites the stored value to `autoRenewalWindowHours`, clamped to 72–720, and records an Event.
   * During the upgrade, old webhook pods reject the new default. The new `harvester` pod may restart a few times until the new webhook is serving. This is expected.
3. The upgrade pre-check (`checkCerts`) reads the earliest expiration from `RKE2Certificate` resources, and falls back to dialing port 6443 when they are not populated yet.
4. The legacy code that updates `clusters.provisioning.cattle.io` is removed.
