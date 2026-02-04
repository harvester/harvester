package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type InstanceType string

const (
	InstanceTypeEngine  = InstanceType("engine")
	InstanceTypeReplica = InstanceType("replica")
	InstanceTypeNone    = InstanceType("")
)

type InstanceManagerState string

const (
	InstanceManagerStateError    = InstanceManagerState("error")
	InstanceManagerStateRunning  = InstanceManagerState("running")
	InstanceManagerStateStopped  = InstanceManagerState("stopped")
	InstanceManagerStateStarting = InstanceManagerState("starting")
	InstanceManagerStateUnknown  = InstanceManagerState("unknown")
)

// +kubebuilder:validation:Enum=aio;engine;replica
type InstanceManagerType string

const (
	InstanceManagerTypeAllInOne = InstanceManagerType("aio")
	// Deprecate
	InstanceManagerTypeEngine  = InstanceManagerType("engine")
	InstanceManagerTypeReplica = InstanceManagerType("replica")
)

const (
	InstanceConditionTypeInstanceCreation = "InstanceCreation"
)

const (
	InstanceConditionReasonInstanceCreationFailure = "InstanceCreationFailure"
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
	// Deprecated:Replaced by field `dataEngine`.
	// +optional
	BackendStoreDriver BackendStoreDriverType `json:"backendStoreDriver"`
	// +optional
	DataEngine DataEngineType `json:"dataEngine"`
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
	// Deprecated: Replaced by field `image`.
	// +optional
	EngineImage string `json:"engineImage"`
	// +optional
	Image string `json:"image"`
	// +optional
	DesireState InstanceState `json:"desireState"`
	// +optional
	LogRequested bool `json:"logRequested"`
	// +optional
	SalvageRequested bool `json:"salvageRequested"`
	// Deprecated:Replaced by field `dataEngine`.
	// +optional
	BackendStoreDriver BackendStoreDriverType `json:"backendStoreDriver"`
	// +kubebuilder:validation:Enum=v1;v2
	// +optional
	DataEngine DataEngineType `json:"dataEngine"`
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
	// +optional
	// +nullable
	Conditions []Condition `json:"conditions"`
}

type InstanceProcessStatus struct {
	// +optional
	Endpoint string `json:"endpoint"`
	// +optional
	ErrorMsg string `json:"errorMsg"`
	//+optional
	//+nullable
	Conditions map[string]bool `json:"conditions"`
	// +optional
	Listen string `json:"listen"`
	// +optional
	PortEnd int32 `json:"portEnd"`
	// +optional
	PortStart int32 `json:"portStart"`
	// +optional
	TargetPortEnd int32 `json:"targetPortEnd"`
	// +optional
	TargetPortStart int32 `json:"targetPortStart"`
	// +optional
	State InstanceState `json:"state"`
	// +optional
	Type InstanceType `json:"type"`
	// +optional
	ResourceVersion int64 `json:"resourceVersion"`
}

type V2DataEngineSpec struct {
	// +optional
	CPUMask string `json:"cpuMask"`
}

type DataEngineSpec struct {
	// +optional
	V2 V2DataEngineSpec `json:"v2"`
}

// InstanceManagerSpec defines the desired state of the Longhorn instance manager
type InstanceManagerSpec struct {
	// +optional
	Image string `json:"image"`
	// +optional
	NodeID string `json:"nodeID"`
	// +optional
	Type InstanceManagerType `json:"type"`
	// +optional
	DataEngine DataEngineType `json:"dataEngine"`
	// +optional
	DataEngineSpec DataEngineSpec `json:"dataEngineSpec"`
}

type V2DataEngineStatus struct {
	// +optional
	CPUMask string `json:"cpuMask"`
}

type DataEngineStatus struct {
	// +optional
	V2 V2DataEngineStatus `json:"v2"`
}

// InstanceManagerStatus defines the observed state of the Longhorn instance manager
type InstanceManagerStatus struct {
	// +optional
	OwnerID string `json:"ownerID"`
	// +optional
	CurrentState InstanceManagerState `json:"currentState"`
	// +optional
	// +nullable
	InstanceEngines map[string]InstanceProcess `json:"instanceEngines,omitempty"`
	// +optional
	// +nullable
	InstanceReplicas map[string]InstanceProcess `json:"instanceReplicas,omitempty"`
	// +optional
	// +nullable
	BackingImages map[string]BackingImageV2CopyInfo `json:"backingImages"`
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
	// +optional
	DataEngineStatus DataEngineStatus `json:"dataEngineStatus"`

	// Deprecated: Replaced by InstanceEngines and InstanceReplicas
	// +optional
	// +nullable
	Instances map[string]InstanceProcess `json:"instances,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhim
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Data Engine",type=string,JSONPath=`.spec.dataEngine`,description="The data engine of the instance manager"
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
