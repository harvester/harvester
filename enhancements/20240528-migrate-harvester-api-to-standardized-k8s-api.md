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


#### APIs For GUI

Currently the GUI gets the `actions` and `links` to construct the GUI actions which current vm can do. It varies by current vm status. For example, when we fetch `virtualmachines` resource, it gives us this:

```
"data": [
  {
      "id": "default/nfs",
      "type": "kubevirt.io.virtualmachine",
      "links": {
          "remove": "https://192.168.1.122/k8s/clusters/c-m-87f5m4xz/v1/harvester/kubevirt.io.virtualmachines/default/nfs",
          "self": "https://192.168.1.122/k8s/clusters/c-m-87f5m4xz/v1/harvester/kubevirt.io.virtualmachines/default/nfs",
          "update": "https://192.168.1.122/k8s/clusters/c-m-87f5m4xz/v1/harvester/kubevirt.io.virtualmachines/default/nfs",
          "view": "https://192.168.1.122/k8s/clusters/c-m-87f5m4xz/apis/kubevirt.io/v1/namespaces/default/virtualmachines/nfs"
      },
      "actions": {
          "addVolume": "https://192.168.1.122/k8s/clusters/c-m-87f5m4xz/v1/harvester/kubevirt.io.virtualmachines/default/nfs?action=addVolume",
          "clone": "https://192.168.1.122/k8s/clusters/c-m-87f5m4xz/v1/harvester/kubevirt.io.virtualmachines/default/nfs?action=clone",
          "removeVolume": "https://192.168.1.122/k8s/clusters/c-m-87f5m4xz/v1/harvester/kubevirt.io.virtualmachines/default/nfs?action=removeVolume",
          "restart": "https://192.168.1.122/k8s/clusters/c-m-87f5m4xz/v1/harvester/kubevirt.io.virtualmachines/default/nfs?action=restart",
          "start": "https://192.168.1.122/k8s/clusters/c-m-87f5m4xz/v1/harvester/kubevirt.io.virtualmachines/default/nfs?action=start"
      },
  ..... // other data
```

Those action links are constructed here, it's highly coupling with rancher/steve. 

https://github.com/harvester/harvester/blob/709ae2a39f3b634e3b27da0f9a02fa33d20ff3fe/pkg/api/vm/formatter.go#L44-L119

Those action APIs are `POST` only method. The rancher/steve validates HTTP method is `POST` or not here:

https://github.com/rancher/apiserver/blob/8c448886365e4a1d60447361f8fceffc41af6d68/pkg/server/validate.go#L22-L24

However, if we plan to change the HTTP method to suitable one, which might be from `POST` to `PUT` or from `POST` to `GET`. There will be **huge efforts** for GUI because GUI need to support two different API schema, even different HTTP method.

So, there are two options for this:

1. Don't change HTTP method, keep both same.
2. Don't migrate, create new API schema with new HTTP method and still keep old API original.

Migration will be easier with first decision than second one. Since in second one, we need to create a way to sync endpoint and HTTP method between GUI and backend.


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
- If it's not idempotent, use `POST` or more matched HTTP method. **However, there shouldn't be `POST` or more matched HTTP method because most APIs aim to handle existing resource instead of creating new one.**
- For list API, should use `GET` method. For example, `findMigratableNodes` action, it should be `GET` cause it doesn't change anything.

In short words, **there should be only `GET` or `PUT` method for subresource APIs**. And We should also check which action is not used anymore, and remove it on new APIs. But, it depends on the decision of [APIs For GUI](#apis-for-gui).

#### Related Documentation Issue

Currently, we didn't have any documentation for the action APIs, we only have k8s resource API instead.

There is already an issue [[Doc] Add document for action APIs](https://github.com/harvester/harvester/issues/5627) for tracking this. After migration, we also need to document the new APIs. This documentation upgrade strategy will follow below `Upgrade Strategy` section.


### Test Plan

Please check [api list](#api-list), and we need to test old and new APIs. 

There are two ways to test it:

1. Test Entry Point
   - The server shouldn't return 404 not found error for old and new endpoints.
2. Test Functionality
   - The server should correctly handle the request.

For first one, just make sure the endpoint is accessible. For the second one, we could copy existing test cases and modify the request path to new APIs. But, it might increase the time of testing, so we might pick some important APIs to test first. If time is enough, we could test it all.

### Upgrade strategy

We won't remove old APIs format immediately, we will support both old and new APIs at the same time for backward compatibility.

However, it's an effort to maintain two different APIs, so eventually we need to remove old one. So, the road map will be:

1. (v1.4.0) Introduce the new APIs, and still support old APIs. Here, we could also introduce the new feature flag of setting resource to enable/disable old APIs. By default, it should be enabled at this moment. 
2. (v1.5.0) Encourage the users to use the new APIs, and internal services/tools should use the new APIs.
3. (v1.6.0) Old APIs are disabled by default, and we could turn it on by feature flag of setting resource. When old APIs are disabled, we will provide new actions API link to GUI, more detail in [APIs For GUI ](#apis-for-gui)
4. (v1.7.0) After a period of time, we could remove the old APIs and feature flag. Only keep new APIs.

Those releases are just for reference, we could adjust it based on the progress of the migration.

> Thanks @m-ildefons for the milestone suggestion.

In v1.4.0 harvester, we should accomplish first step at least.

About the second step, we need to request each project owner to check whether they are using the old APIs or not. Then, open a new issue to track the progress. For the external users, because we didn't have any metrics to track all usages, so we can only note this in our release note. They could also use feature flag to test their application which is being used old APIs or not.

For the third step and the last step, it depends on the progress of the second step. So, we will keep tracking it in the future release.

***NOTE: if we have new action APIs, we should add them into both APIs at the same time.***

## Note

Because kube api server will proxying request to the harvester api server, it may increase latency and pressure of kube api server if there are some huge usages with our subresource APIs.

## Overall TODO

- [ ] Check which action is not used anymore, remove it on new APIs.
- [ ] Create new APIs for each action, check [api list](#api-list).
  - [ ] Define the `APIService`
  - [ ] New interface to fulfill two different API sources
  - [ ] Customize the actions link for GUI
- [ ] Feature flag to enabled/disable old APIs
- [ ] Internal services/tools convert old APIs to new APIs
  - [ ] QA Tools
  - [ ] Harvester Dashboard
  - [ ] Other internal services
- [ ] Release note to encourage users to use new APIs
- [ ] Document for new APIs
- [ ] Document for old APIs (Tracked in other issue)

## Open Discussion

Since we highly couple with rancher/steve, it might not be a good option to change the way how we use rancher/steve. So, I'm thinking we could just only support new API schema instead of migration.