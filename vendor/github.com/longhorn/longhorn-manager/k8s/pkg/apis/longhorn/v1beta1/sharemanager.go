package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type ShareManagerState string

const (
	ShareManagerStateUnknown  = ShareManagerState("unknown")
	ShareManagerStateStarting = ShareManagerState("starting")
	ShareManagerStateRunning  = ShareManagerState("running")
	ShareManagerStateStopping = ShareManagerState("stopping")
	ShareManagerStateStopped  = ShareManagerState("stopped")
	ShareManagerStateError    = ShareManagerState("error")
)

// ShareManagerSpec defines the desired state of the Longhorn share manager
type ShareManagerSpec struct {
	Image string `json:"image"`
}

// ShareManagerStatus defines the observed state of the Longhorn share manager
type ShareManagerStatus struct {
	OwnerID  string            `json:"ownerID"`
	State    ShareManagerState `json:"state"`
	Endpoint string            `json:"endpoint"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhsm
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`,description="The state of the share manager"
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.ownerID`,description="The node that the share manager is owned by"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ShareManager is where Longhorn stores share manager object.
type ShareManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Spec ShareManagerSpec `json:"spec,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Status ShareManagerStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ShareManagerList is a list of ShareManagers.
type ShareManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ShareManager `json:"items"`
}
