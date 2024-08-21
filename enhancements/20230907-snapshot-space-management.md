# Snapshot Space Management

## Summary

This proposal introduces soft limitation about snapshot size. Users can set maximal total snapshot size at VM or Namespace level.
Before creating a new VMBackup or VolumeSnapshot, webhook will check snapshot size usage from LH Engine status. This proposal focuses on snapshot size usage in the system, not backup target.

### Related Issues

https://github.com/harvester/harvester/issues/4478

## Motivation

### Goals

- Provide a way to control snapshot size usage at VM or Namespace level.

## Proposal

When receiving a new VMBackup or VolumeSnapshot request, webhook will check the snapshot size usage from the related LH Engine status.
If the usage is more than limit, deny the request.

### User Stories

With snapshot space management, snapshot space usage is controllable. Users can set snapshot limitation and evaluate maximum space usage.

### User Experience In Detail

Before snapshot space management:
The default maximum snapshot count for each volume is `250`. This is a constants value in the [source code](https://github.com/longhorn/longhorn-engine/blob/8b4c80ab174b4f454a992ff998b6cb1041faf63d/pkg/replica/replica.go#L33) and users don't have a way to control it. If a volume size is 1G, the maximum snapshot space usage will be 250G.

After snapshot space management:
Administrators can control snapshot space usage at VM or Namespace level.

### API changes

#### Update Resource Quota API on Namespace [POST]

Endpoint: `/v1/harvester/namespace/<namespace>?action=updateResourceQuota`

Request (application/json):
```json
{
    "totalSnapshotSizeQuota": "10Gi"
}
```

Response:
- 403: if users don't have permission to update `resourcequotas.harvesterhci.io`.
- 200: if the request is successful.

This API will update `spec.snapshotLimit.namespaceTotalSnapshotSizeQuota` in `ResourceQuota` CRD. For CRD details, please refer to the [Design](#ResourceQuota-CRD-in-harvesterhcioio-group) section.

#### Delete Resource Quota API on Namespace [POST]

Endpoint: `/v1/harvester/namespace/<namespace>?action=deleteResourceQuotaAction`

Response:
- 403: if users don't have permission to update `resourcequotas.harvesterhci.io`.
- 200: if the request is successful.

This API will delete `spec.snapshotLimit.namespaceTotalSnapshotSizeQuota` in `ResourceQuota` CRD.

#### Update Resource Quota API on VM [POST]

Endpoint: `/v1/harvester/kubevirt.io.virtualmachines/<namespace>/<name>?action=updateResourceQuota`

Request (application/json):
```json
{
    "totalSnapshotSizeQuota": "10Gi"
}
```

Response:
- 403: if users don't have permission to update `resourcequotas.harvesterhci.io`.
- 204: if the request is successful.

This API will update `spec.snapshotLimit.vmTotalSnapshotSizeUsage[vmName]` in `ResourceQuota` CRD. For CRD details, please refer to the [Design](#ResourceQuota-CRD-in-harvesterhcioio-group) section.

#### Delete Resource Quota API on VM [POST]

Endpoint: `/v1/harvester/kubevirt.io.virtualmachines/<namespace>/<name>?action=deleteResourceQuota`

Response:
- 403: if users don't have permission to update `resourcequotas.harvesterhci.io`.
- 204: if the request is successful.

This API will delete `spec.snapshotLimit.vmTotalSnapshotSizeUsage[vmName]` in `ResourceQuota` CRD.

## Design

### `ResourceQuota` CRD in `harvesterhci.io` group

Although there is default `ResourceQuota` in `core` group, it can't fulfill our use cases.
For example, we want to have sum of snapshots size in a VM, but it can only limit sum of storage requests.
We would like to have a quota limit at VM or Namespace level, so we introduce `ResourceQuota` CRD in `harvesterhci.io` group.
At first, we will only have snapshot limit in this CRD. In the future, we can have different limits like pci devices, gpu, etc.

The new CRD is Namescoped resource. The Namescoped resource can have many different names CR in each namespace.
Currently, we only want to use one resource in each namespace to enforce limit, so each namespace has one `default-resource-quota` resource.

The CRD definition is:
```go
type ResourceQuota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceQuotaSpec   `json:"spec,omitempty"`
	Status ResourceQuotaStatus `json:"status,omitempty"`
}

type ResourceQuotaSpec struct {
	// +kubebuilder:validation:Optional
	SnapshotLimit SnapshotLimit `json:"snapshotLimit,omitempty"`
}

type ResourceQuotaStatus struct {
	// +kubebuilder:validation:Optional
	SnapshotLimitStatus SnapshotLimitStatus `json:"snapshotLimitStatus,omitempty"`
}

type SnapshotLimit struct {
	NamespaceTotalSnapshotSizeQuota int64            `json:"namespaceTotalSnapshotSizeQuota,omitempty"`
	VMTotalSnapshotSizeQuota        map[string]int64 `json:"vmTotalSnapshotSizeQuota,omitempty"`
}

type SnapshotLimitStatus struct {
	NamespaceTotalSnapshotSizeUsage int64            `json:"namespaceTotalSnapshotSizeUsage,omitempty"`
	VMTotalSnapshotSizeUsage        map[string]int64 `json:"vmTotalSnapshotSizeUsage,omitempty"`
}
```

### `RoleTemplate` in Rancher

The administrator has permission to set limit on `ResourceQuota`, but other users only have permission to read it, because they cannot overwrite settings from adminstrator.
We offer two `RoleTemplate` at cluster level in Rancher to control the permission.

harvester-resourcequotas-admin:
```yaml
apiVersion: management.cattle.io/v3
builtin: false
context: cluster
description: "Harvester ResourceQuotas Admin can manage all resourcequotas.harvesterhci.io on downstream Harvester cluster"
displayName: Harvester ResourceQuotas Admin Role
external: false
hidden: false
kind: RoleTemplate
metadata:
  name: harvester-resourcequotas-admin
rules:
- apiGroups:
  - harvesterhci.io
  resources:
  - resourcequotas
  verbs:
  - '*'
```

harvester-resourcequotas-read-only:
```yaml
apiVersion: management.cattle.io/v3
builtin: false
context: cluster
description: "Harvester ResourceQuotas Read Only can view resourcequotas.harvesterhci.io on downstream Harvester cluster"
displayName: Harvester ResourceQuotas Read Role
external: false
hidden: false
kind: RoleTemplate
metadata:
  name: harvester-resourcequotas-read-only
rules:
- apiGroups:
  - harvesterhci.io
  resources:
  - resourcequotas
  verbs:
  - get
  - list
  - watch
```

#### VMBackup webhook

1. Check whether the source VM has related `default-resource-quota` CR in the same namespace. Also, whether the CR has `spec.snapshotLimit.vmTotalSnapshotSizeUsage[vmName]`.  If not, jump to step 4.
2. Sum up all snapshots size for each volume in the VM.
3. If the total size is over the limit, deny the request.
4. Check whether `default-resource-quota` CR in the same namespace has `spec.snapshotLimit.namespaceTotalSnapshotSizeUsage`. If not, skip following steps.
5. Sum up all snapshots size for each volume in the Namespace.
6. If the total size is over the limit, deny the request.

#### VolumeSnapshot webhook

1. Check whether `default-resource-quota` CR in the same namespace has `spec.snapshotLimit.namespaceTotalSnapshotSizeUsage`. If not, skip following steps.
2. Check whether the owner reference is a VMBackup. Skip following steps if yes, because we already done the check in VMBackup webhook.
3. Sum up all snapshots size for each volume in the Namespace.
4. If the total size is over the limit, deny the request.

### Test plan

1. `namespaceTotalSnapshotSizeUsage` and `vmTotalSnapshotSizeUsage` values format.
    - Deny value which can't be parse by [ParseQuantity](https://github.com/kubernetes/apimachinery/blob/e126c655b8146add9b9c19bf89dab6eb85578814/pkg/api/resource/quantity.go#L276-L277).
    - Deny negative value.
2. Set `30Gi` limit on a VM after it's created. The VM has two volumes. The first volume size is `10Gi` and the second volume size is `20Gi`.
    - Write data to make two volumes full. Create the first VMBackup. It should be allowed. The total snapshot size is `30Gi`.
    - Clear data and write data to make two volumes full again. Create the seoncd VMBackup. It should be allowed, because we haven't set the annotation. The total snapshot size is `60Gi`.
    - Set `30Gi` on the VM. It should be allowed.
    - Clear data and write data to make two volumes full again.  Create the third VMBackup. It should be denied, because the total snapshot size will be `90Gi` if the request is allowed.
    - Remove the first and the second VMBackup. The total snapshot size is `0Gi`.
    - Create a new VMBackup. It should be allowed. The total snapshot size is `30Gi`.
4. Set `10Gi` limit on a Namespace. Create a VM with size `10Gi`.
    - Write data to make the volume full. Create the first VMBackup. It should be allowed. The total snapshot size is `10Gi`.
    - Clear data and write data to make the volume full again. Create the second VMBackup. It should be denied, because the total snapshot size will be `20Gi` if the request is allowed.
    - Remove the first VMBackup. The total snapshot size is `0Gi`.
    - Create a new VMBackup. It should be allowed. The total snapshot size is `10Gi`.
5. Set `10Gi` limit on a Namespace. Create a volume with size `10Gi`.
    - Write data to make the volume full. Create the first VolumeSnapshot. It should be allowed. The total snapshot size is `10Gi`.
    - Clear data and write data to make the volume full again. Create the second VolumeSnapshot. It should be denied, because the total snapshot size will be `20Gi` if the request is allowed.
    - Remove the first VolumeSnapshot. The total snapshot size is `0Gi`.
    - Create a new VolumeSnapshot. It should be allowed. The total snapshot size is `10Gi`.
6. Users with `Harvester ResourceQuotas Read Only` role.
    - Apply `harvester-resourcequotas-admin` and `harvester-resourcequotas-read-only` to a Rancher.
    - Import Harvester to a Rancher.
    - Create a new user with `Harvester ResourceQuotas Read Only` role.
    - Add the user to a project as member.
    - Send requests to update Resource Quotas via API on Namespace / VM. Both requests should be denied.
7. Users with `Harvester ResourceQuotas Admin` role.
    - Apply `harvester-resourcequotas-admin` and `harvester-resourcequotas-read-only` to a Rancher.
    - Import Harvester to a Rancher.
    - Create a new user with `Harvester ResourceQuotas Admin` role.
    - Add the user to a project as readonly user.
    - Send requests to update Resource Quotas via API on Namespace / VM. Both requests should be allowed.

### Upgrade strategy

None

## Rejected Alternatives

Leveraging [LH Snapshot Space Management](https://github.com/longhorn/longhorn/issues/6563) to control snapshot space usage in the system.
LH can limit snapshot count and size in data path. It's most accurate way to control snapshot space usage.
There are two reason we don't use this way:

* This limitation introduces some risks to the system. For example, to rebuild a replica, the system needs to create a snapshot.
If the snapshot count is over the limitation, the system can't create a snapshot and the rebuild process will be failed.
If the volume is not ready, the VM can't start.
* From Harvester perspective, VM is a basic instance. If a VMBackup suceeds, all snapshots should be created successfully.
However, there is always time gap between real data status and status on CRD.
When webhook check snapshot usage for a new VMBackup request, it can't make sure all snapshots will be successfully created, because the status webhook can check may be stale.

To minimize the risk for the system, we decide to use soft limitation currently.

## Note [optional]

None
