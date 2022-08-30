package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type InstanceType string

const (
	InstanceTypeEngine  = InstanceType("engine")
	InstanceTypeReplica = InstanceType("replica")
)

type InstanceManagerState string

const (
	InstanceManagerStateError    = InstanceManagerState("error")
	InstanceManagerStateRunning  = InstanceManagerState("running")
	InstanceManagerStateStopped  = InstanceManagerState("stopped")
	InstanceManagerStateStarting = InstanceManagerState("starting")
	InstanceManagerStateUnknown  = InstanceManagerState("unknown")
)

// +kubebuilder:validation:Enum=engine;replica
type InstanceManagerType string

const (
	InstanceManagerTypeEngine  = InstanceManagerType("engine")
	InstanceManagerTypeReplica = InstanceManagerType("replica")
)

type InstanceProcess struct {
	// +optional
	Spec InstanceProcessSpec `json:"spec"`
	// +optional
	Status InstanceProcessStatus `json:"status"`
}

type InstanceProcessSpec struct {
	// +optional
	Name string `json:"name"`
}

type InstanceState string

const (
	InstanceStateRunning  = InstanceState("running")
	InstanceStateStopped  = InstanceState("stopped")
	InstanceStateError    = InstanceState("error")
	InstanceStateStarting = InstanceState("starting")
	InstanceStateStopping = InstanceState("stopping")
	InstanceStateUnknown  = InstanceState("unknown")
)

type InstanceSpec struct {
	// +optional
	VolumeName string `json:"volumeName"`
	// +kubebuilder:validation:Type=string
	// +optional
	VolumeSize int64 `json:"volumeSize,string"`
	// +optional
	NodeID string `json:"nodeID"`
	// +optional
	EngineImage string `json:"engineImage"`
	// +optional
	DesireState InstanceState `json:"desireState"`
	// +optional
	LogRequested bool `json:"logRequested"`
	// +optional
	SalvageRequested bool `json:"salvageRequested"`
}

type InstanceStatus struct {
	// +optional
	OwnerID string `json:"ownerID"`
	// +optional
	InstanceManagerName string `json:"instanceManagerName"`
	// +optional
	CurrentState InstanceState `json:"currentState"`
	// +optional
	CurrentImage string `json:"currentImage"`
	// +optional
	IP string `json:"ip"`
	// +optional
	StorageIP string `json:"storageIP"`
	// +optional
	Port int `json:"port"`
	// +optional
	Started bool `json:"started"`
	// +optional
	LogFetched bool `json:"logFetched"`
	// +optional
	SalvageExecuted bool `json:"salvageExecuted"`
}

type InstanceProcessStatus struct {
	// +optional
	Endpoint string `json:"endpoint"`
	// +optional
	ErrorMsg string `json:"errorMsg"`
	// +optional
	Listen string `json:"listen"`
	// +optional
	PortEnd int32 `json:"portEnd"`
	// +optional
	PortStart int32 `json:"portStart"`
	// +optional
	State InstanceState `json:"state"`
	// +optional
	Type InstanceType `json:"type"`
	// +optional
	ResourceVersion int64 `json:"resourceVersion"`
}

// InstanceManagerSpec defines the desired state of the Longhorn instancer manager
type InstanceManagerSpec struct {
	// +optional
	Image string `json:"image"`
	// +optional
	NodeID string `json:"nodeID"`
	// +optional
	Type InstanceManagerType `json:"type"`
	// TODO: deprecate this field
	// +optional
	EngineImage string `json:"engineImage"`
}

// InstanceManagerStatus defines the observed state of the Longhorn instance manager
type InstanceManagerStatus struct {
	// +optional
	OwnerID string `json:"ownerID"`
	// +optional
	CurrentState InstanceManagerState `json:"currentState"`
	// +optional
	// +nullable
	Instances map[string]InstanceProcess `json:"instances"`
	// +optional
	IP string `json:"ip"`
	// +optional
	APIMinVersion int `json:"apiMinVersion"`
	// +optional
	APIVersion int `json:"apiVersion"`
	// +optional
	ProxyAPIMinVersion int `json:"proxyApiMinVersion"`
	// +optional
	ProxyAPIVersion int `json:"proxyApiVersion"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhim
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.currentState`,description="The state of the instance manager"
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`,description="The type of the instance manager (engine or replica)"
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeID`,description="The node that the instance manager is running on"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// InstanceManager is where Longhorn stores instance manager object.
type InstanceManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InstanceManagerSpec   `json:"spec,omitempty"`
	Status InstanceManagerStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// InstanceManagerList is a list of InstanceManagers.
type InstanceManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InstanceManager `json:"items"`
}
