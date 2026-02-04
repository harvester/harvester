package types

const (
	ProcessContainerd     = "containerd"
	ProcessContainerdShim = "containerd-shim"
	ProcessDockerd        = "dockerd"
	ProcessNone           = ""
	ProcessSelf           = "self"

	// ProcessKubelet is not a standard process name for Kubelet, used by Talos Linux only.
	ProcessKubelet = "kubelet"
)
