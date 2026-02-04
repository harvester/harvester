package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type EngineImageState string

const (
	EngineImageStateDeploying = EngineImageState("deploying")
	EngineImageStateDeployed  = EngineImageState("deployed")
	// EngineImageStateIncompatible is deprecated from 1.6.0. It has been replaced with `incompatible` of EngineImageStatus
	EngineImageStateIncompatible = EngineImageState("incompatible")
	EngineImageStateError        = EngineImageState("error")
)

const (
	EngineImageConditionTypeReady = "ready"

	EngineImageConditionTypeReadyReasonDaemonSet = "daemonSet"
	EngineImageConditionTypeReadyReasonBinary    = "binary"
)

type EngineVersionDetails struct {
	// +optional
	Version string `json:"version"`
	// +optional
	GitCommit string `json:"gitCommit"`
	// +optional
	BuildDate string `json:"buildDate"`
	// +optional
	CLIAPIVersion int `json:"cliAPIVersion"`
	// +optional
	CLIAPIMinVersion int `json:"cliAPIMinVersion"`
	// +optional
	ControllerAPIVersion int `json:"controllerAPIVersion"`
	// +optional
	ControllerAPIMinVersion int `json:"controllerAPIMinVersion"`
	// +optional
	DataFormatVersion int `json:"dataFormatVersion"`
	// +optional
	DataFormatMinVersion int `json:"dataFormatMinVersion"`
}

// EngineImageSpec defines the desired state of the Longhorn engine image
type EngineImageSpec struct {
	// +kubebuilder:validation:MinLength:=1
	Image string `json:"image"`
}

// EngineImageStatus defines the observed state of the Longhorn engine image
type EngineImageStatus struct {
	// +optional
	OwnerID string `json:"ownerID"`
	// +optional
	State EngineImageState `json:"state"`
	// +optional
	RefCount int `json:"refCount"`
	// +optional
	NoRefSince string `json:"noRefSince"`
	// +optional
	Incompatible bool `json:"incompatible"`
	// +optional
	// +nullable
	Conditions []Condition `json:"conditions"`
	// +optional
	// +nullable
	NodeDeploymentMap    map[string]bool `json:"nodeDeploymentMap"`
	EngineVersionDetails `json:""`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhei
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Incompatible",type=boolean,JSONPath=`.status.incompatible`,description="Compatibility of the engine image"
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`,description="State of the engine image"
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`,description="The Longhorn engine image"
// +kubebuilder:printcolumn:name="RefCount",type=integer,JSONPath=`.status.refCount`,description="Number of resources using the engine image"
// +kubebuilder:printcolumn:name="BuildDate",type=date,JSONPath=`.status.buildDate`,description="The build date of the engine image"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// EngineImage is where Longhorn stores engine image object.
type EngineImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EngineImageSpec   `json:"spec,omitempty"`
	Status EngineImageStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EngineImageList is a list of EngineImages.
type EngineImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EngineImage `json:"items"`
}

// Hub defines the current version (v1beta2) is the storage version
// so mark this as Hub
func (ei *EngineImage) Hub() {}
