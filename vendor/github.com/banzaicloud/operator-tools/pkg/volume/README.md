
## KubernetesVolume

Configure volumes in custom types for underlying pods in a uniform way.

```go
type SomeCustomApp struct {
	BufferStorage *volume.KubernetesVolume `json:"bufferStorage,omitempty"` 
}
```

And this is how you would configure it through yaml
```yaml
someCustomApp:
  bufferStorage:
    hostPath:
      path: "/buffer" # set this here, or provide a default in the code
```

In the operator you can use the `GetVolume` method on the object in a generic way:
```go
// provide a default path if hostPath is used but no actual path has been configured explicitly
someCustomApp.bufferStorage.WithDefaultHostPath("/opt/buffer")

volume := someCustomApp.bufferStorage.GetVolume(
  "volumeName",
))
```
