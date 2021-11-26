package v1beta1

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

type InstanceManagerType string

const (
	InstanceManagerTypeEngine  = InstanceManagerType("engine")
	InstanceManagerTypeReplica = InstanceManagerType("replica")
)

type InstanceProcess struct {
	Spec   InstanceProcessSpec   `json:"spec"`
	Status InstanceProcessStatus `json:"status"`
}

type InstanceProcessSpec struct {
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
	VolumeName       string        `json:"volumeName"`
	VolumeSize       int64         `json:"volumeSize,string"`
	NodeID           string        `json:"nodeID"`
	EngineImage      string        `json:"engineImage"`
	DesireState      InstanceState `json:"desireState"`
	LogRequested     bool          `json:"logRequested"`
	SalvageRequested bool          `json:"salvageRequested"`
}

type InstanceStatus struct {
	OwnerID             string        `json:"ownerID"`
	InstanceManagerName string        `json:"instanceManagerName"`
	CurrentState        InstanceState `json:"currentState"`
	CurrentImage        string        `json:"currentImage"`
	IP                  string        `json:"ip"`
	Port                int           `json:"port"`
	Started             bool          `json:"started"`
	LogFetched          bool          `json:"logFetched"`
	SalvageExecuted     bool          `json:"salvageExecuted"`
}

type InstanceProcessStatus struct {
	Endpoint        string        `json:"endpoint"`
	ErrorMsg        string        `json:"errorMsg"`
	Listen          string        `json:"listen"`
	PortEnd         int32         `json:"portEnd"`
	PortStart       int32         `json:"portStart"`
	State           InstanceState `json:"state"`
	Type            InstanceType  `json:"type"`
	ResourceVersion int64         `json:"resourceVersion"`
}

type InstanceManagerSpec struct {
	Image  string              `json:"image"`
	NodeID string              `json:"nodeID"`
	Type   InstanceManagerType `json:"type"`

	// TODO: deprecate this field
	EngineImage string `json:"engineImage"`
}

type InstanceManagerStatus struct {
	OwnerID       string                     `json:"ownerID"`
	CurrentState  InstanceManagerState       `json:"currentState"`
	Instances     map[string]InstanceProcess `json:"instances"`
	IP            string                     `json:"ip"`
	APIMinVersion int                        `json:"apiMinVersion"`
	APIVersion    int                        `json:"apiVersion"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type InstanceManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              InstanceManagerSpec   `json:"spec"`
	Status            InstanceManagerStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type InstanceManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []InstanceManager `json:"items"`
}
