# Migrate harvester API actions to standardized k8s API


## Summary

Harvester currently use rancher/steve to build custom api actions, such as stop vm with `?action=stop` which is not standard k8s API. 

We will leverage the Kubernetes API Aggregation Layer, which proxies client requests to a specified API server defined with a custom resource `APIService`.

This allows us to use the native Kubernetes client to invoke subresource APIs. 

```go
harvesterSubresourceClient.
	Post().
	Namespace("default").
	Resource("virtualmachines").
	SubResource("stop").
	Name("test").Do(context.Background())
```

### Related Issue

- https://github.com/harvester/harvester/issues/1078
  - This is root issue.
- https://github.com/harvester/harvester/issues/5627
  - This is sub-issue about we need to add documentation for the action APIs. 


## Motivation

### Goals

- Ensure the API format aligns with Kubernetes conventions for better consistency and security.
- Support old and new API actions at the same time for backward compatibility.

### Non-Goals

- Migrate current non action APIs to k8s standardized APIs.

## Proposal

### Use Case 1 - API Usage

In real usecase, the old api looks like this:

```
POST /v1/harvester/kubevirt.io.virtualmachines/default/test?action=stop
POST /v1/harvester/nodes/jacknode?action=cordon
GET  /v1/harvester/upgradeslogs/default/test/download
POST /v1/harvester/persistentvolumeclaims/default/test-disk-0-rlnlk?action=snapshot
```

The new api will be like this:

```yaml
POST /apis/subresources.harvesterhci.io/v1beta1/namespaces/default/virtualmachines/test/stop
POST /apis/subresources.harvesterhci.io/v1beta1/nodes/jacknode/uncordon
GET  /apis/subresources.harvesterhci.io/v1beta1/namespaces/default/upgradelogs/test/download
POST /apis/subresources.harvesterhci.io/v1beta1/namespaces/default/persistentvolumeclaims/test/snapshot
```

That means we can use k8s client to access the subresource API as well.

```go
harvesterCopyConfig := rest.CopyConfig(server.RESTConfig)
harvesterCopyConfig.GroupVersion = &k8sschema.GroupVersion{Group: "subresources.harvesterhci.io", Version: "v1beta1"}
harvesterCopyConfig.APIPath = "/apis"
harvesterCopyConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
harvesterSubresourceClient, err := rest.RESTClientFor(harvesterCopyConfig)

h.harvesterSubresourceClient.Post().Resource("nodes").SubResource("uncordon").Name(node.Name).Do(context.Background())
h.harvesterSubresourceClient.Post().Namespace("default").Resource("virtualmachines").SubResource("stop").Name("test").Do(context.Background())
```

Or a pure native k8s API.

```
https://{server}/apis/subresources.harvesterhci.io/v1beta1/namespaces/default/virtualmachines/test2/stop
```

### Use Case 2 - Security

When using subresource API, we can leverage the Kubernetes RBAC to control the access to the subresource API. For example, if we create a role like below, the user can only GET and POST the subresource.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: subresource-test
  namespace: default
rules:
- apiGroups:
  - subresources.harvesterhci.io
  resources:
  - virtualmachines/stop
  verbs:
  - get
  - create
```

It will get the 403 forbidden error if the user tries to access the subresource without the permission.

```json
{
  "kind": "Status",
  "apiVersion": "v1",
  "metadata": {},
  "status": "Failure",
  "message": "virtualmachines.subresources.harvesterhci.io \"test2\" is forbidden: User \"system:serviceaccount:default:test-subresource\" cannot create resource \"virtualmachines/start\" in API group \"subresources.harvesterhci.io\" in the namespace \"default\"",
  "reason": "Forbidden",
  "details": {
    "name": "test2",
    "group": "subresources.harvesterhci.io",
    "kind": "virtualmachines"
  },
  "code": 403
}
```

### Design

In order to implement this proposal, we need to take the following steps:

1. Define the `APIService` to proxy requests to Harvester API server.
    ```yaml
    apiVersion: apiregistration.k8s.io/v1
    kind: APIService
    metadata:
      name: v1beta1.subresources.harvesterhci.io
    spec:
      group: subresources.harvesterhci.io
      groupPriorityMinimum: 1000
      insecureSkipTLSVerify: true
      service:
        name: harvester
        namespace: harvester-system
        port: 8443
      version: v1beta1
      versionPriority: 15
    ```
2. Define the new routes path to accept following request path formats:
   - `/apis/subresources.harvesterhci.io/v1beta1/namespaces/{namespace}/{resource}/{name}/{subresource}` 
   - `/apis/subresources.harvesterhci.io/v1beta1/{resource}/{name}/{subresource}`

It's basically done because we've had the action business logic in the controller, we just need to create a new interface to fulfill two different API sources, which one is from steve and the other is from subresource api.

### Complete API List need migration


#### Caution 

Although we use CRD from other projects, e.g. KubeVirt,  but we have our own logic before calling other project APIs. For example:

Before we start VM, we do some pre-check here.

https://github.com/harvester/harvester/blob/11adde7ee31f06beba85ed85709f357e8d076950/pkg/api/vm/handler.go#L280-L289

However, we didn't do something else when stop/restart vm, but I think it's better that we control the whole flow on our side, just in case. And it makes the API more consistent.

#### API List

In general, we have those APIs need to be migrated:

1. `/v1/harvester/kubevirt.io.virtualmachines/:namespace/:name`
2. `/v1/harvester/persistentvolumeclaims/:namespace/:name`
3. `/v1/harvester/snapshot.storage.k8s.io.volumesnapshots/:namespace/:name`
4. `/v1/harvester/harvesterhci.io.upgradelog/:namespace/:name`
5. `/v1/harvester/nodes/:name`
6. `/v1/harvester/harvesterhci.io.keypair/:namespace/:name`
7. `/v1/harvester/harvesterhci.io.virtualmachineimages/:namespace/:name`

Each one has different actions:

- `/v1/harvester/kubevirt.io.virtualmachines/:namespace/:name`
  - POST, `?action=start`
  - POST, `?action=stop`
  - POST, `?action=restart`
  - POST, `?action=softreboot`
  - POST, `?action=pause`
  - POST, `?action=unpause`
  - POST, `?action=ejectCdRom`
  - POST, `?action=migrate`
  - POST, `?action=abortMigration`
  - POST, `?action=findMigratableNodes`
  - POST, `?action=backup`
  - POST, `?action=restore`
  - POST, `?action=createTemplate`
  - POST, `?action=addVolume`
  - POST, `?action=removeVolume`
  - POST, `?action=clone`
  - POST, `?action=forceStop`
  - POST, `?action=dismissInsufficientResourceQuota`
- `/v1/harvester/persistentvolumeclaims/:namespace/:name`
  - POST, `?action=export`
  - POST, `?action=cancelExpand`
  - POST, `?action=clone`
  - POST, `?action=snapshot`
- `/v1/harvester/snapshot.storage.k8s.io.volumesnapshots/:namespace/:name`
  - POST, `?action=restore`
- `/v1/harvester/harvesterhci.io.upgradelog/:namespace/:name`
  - GET, `?link=download`
  - POST, `?action=generate`
- `/v1/harvester/nodes/:name`
  - POST, `?action=enableMaintenanceMode`
  - POST, `?action=disableMaintenanceMode`
  - POST, `?action=cordon`
  - POST, `?action=uncordon`
  - POST, `?action=listUnhealthyVM`
  - POST, `?action=maintenancePossible`
  - POST, `?action=powerAction`
  - POST, `?action=powerActionPossible`
- `/v1/harvester/harvesterhci.io.keypair/:namespace/:name`
  - POST, `?action=keygen`
- `/v1/harvester/harvesterhci.io.virtualmachineimages/:namespace/:name`
  - POST, `?action=upload`
  - GET, `?link=download`

After migration, the new APIs will be like this:

1. `/apis/subresources.harvesterhci.io/v1beta1/namespaces/:namespace/kubevirt.io.virtualmachines/:name/:subresource`
2. `/apis/subresources.harvesterhci.io/v1beta1/namespaces/:namespace/persistentvolumeclaims/:name/:subresource`
3. `/apis/subresources.harvesterhci.io/v1beta1/namespaces/:namespace/snapshot.storage.k8s.io.volumesnapshots/:name/:subresource`
4. `/apis/subresources.harvesterhci.io/v1beta1/namespaces/:namespace/harvesterhci.io.upgradelog/:name/:subresource`
5. `/apis/subresources.harvesterhci.io/v1beta1/nodes/:name/:subresource`
6. `/apis/subresources.harvesterhci.io/v1beta1/namespaces/:namespace/harvesterhci.io.keypair/:name/:subresource`
7. `/apis/subresources.harvesterhci.io/v1beta1/namespaces/:namespace/harvesterhci.io.virtualmachineimages/:name/:subresource`

But, each subresource might contain different HTTP method. It depends on the behavior is idempotent or not. 
- If it's idempotent, use `PUT` method.
- If it's not idempotent, use `POST` or more matched HTTP method.
- For list API, should use `GET` method. For example, `findMigratableNodes` action, it should be `GET` cause it doesn't change anything.

#### Related Documentation Issue

Currently, we didn't have any documentation for the action APIs, we only have k8s resource API instead.

There is already an issue [[Doc] Add document for action APIs](https://github.com/harvester/harvester/issues/5627) for tracking this. After migration, we also need to document the new APIs. This documentation upgrade strategy will follow below `Upgrade Strategy` section.


### Test Plan

Should test old and new APIs path formats, both should work as expected.

### Upgrade strategy

We won't remove old APIs format immediately, we will support both old and new APIs at the same time for backward compatibility.

However, it's an effort to maintain two different APIs, so eventually we need to remove old one. So, the road map will be:

1. Introduce the new APIs, and still support old APIs. Here, we could also introduce the new feature flag of setting resource to enable/disable old APIs. By default, it should be enabled at this moment. 
2. Encourage the users to use the new APIs, and internal services/tools should use the new APIs.
3. Old APIs are disabled by default, and we could turn it on by feature flag of setting resource.
4. After a period of time, we could remove the old APIs and feature flag. Remain only new APIs.

***NOTE: we should add it into both APIs at the same time if we have new action APIs.***

In v1.4 harvester, we should accomplish first step. 

About the second step, we need to request each project owner to check whether they are using the old APIs or not. Then, open a new issue to track the progress. For the external users, because we didn't have any metrics to track all usages, so we can only note this in our release note. They could also use feature flag to test their application which is being used old APIs or not.

For the third step and the last step, it depends on the progress of the second step. So, we will keep tracking it in the future release.


## Note

Because kube api server will proxying request to the harvester api server, it may increase latency and pressure of kube api server if there are some huge usages with our subresource APIs.