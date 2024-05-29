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

- Remove old API actions immediately.
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

### Test Plan

Should test old and new APIs path formats, both should work as expected.

### Upgrade strategy

We won't remove old APIs format immediately, we will support both old and new APIs at the same time for backward compatibility.


## Note

Because kube api server will proxying request to the harvester api server, it may increase latency and pressure of kube api server if there are some huge usages with our subresource APIs.