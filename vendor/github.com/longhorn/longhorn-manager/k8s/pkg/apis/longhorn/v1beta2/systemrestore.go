package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type SystemRestoreState string

const (
	SystemRestoreStateCompleted    = SystemRestoreState("Completed")
	SystemRestoreStateDeleting     = SystemRestoreState("Deleting")
	SystemRestoreStateDownloading  = SystemRestoreState("Downloading")
	SystemRestoreStateError        = SystemRestoreState("Error")
	SystemRestoreStateInitializing = SystemRestoreState("Initializing")
	SystemRestoreStateInProgress   = SystemRestoreState("InProgress")
	SystemRestoreStateNone         = SystemRestoreState("")
	SystemRestoreStatePending      = SystemRestoreState("Pending")
	SystemRestoreStateRestoring    = SystemRestoreState("Restoring")
	SystemRestoreStateUnknown      = SystemRestoreState("Unknown")
	SystemRestoreStateUnpacking    = SystemRestoreState("Unpacking")

	SystemRestoreConditionTypeError = "Error"

	SystemRestoreConditionReasonRestore = "Restore"
	SystemRestoreConditionReasonUnpack  = "Unpack"

	SystemRestoreConditionMessageFailed       = "failed to restore Longhorn system"
	SystemRestoreConditionMessageUnpackFailed = "failed to unpack system backup from file"
)

// SystemRestoreSpec defines the desired state of the Longhorn SystemRestore
type SystemRestoreSpec struct {
	// The system backup name in the object store.
	SystemBackup string `json:"systemBackup"`
}

// SystemRestoreStatus defines the observed state of the Longhorn SystemRestore
type SystemRestoreStatus struct {
	// The node ID of the responsible controller to reconcile this SystemRestore.
	// +optional
	OwnerID string `json:"ownerID"`
	// The system restore state.
	// +optional
	State SystemRestoreState `json:"state,omitempty"`
	// The source system backup URL.
	// +optional
	SourceURL string `json:"sourceURL,omitempty"`
	// +optional
	// +nullable
	Conditions []Condition `json:"conditions"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhsr
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`,description="The system restore state"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SystemRestore is where Longhorn stores system restore object
type SystemRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SystemRestoreSpec   `json:"spec,omitempty"`
	Status SystemRestoreStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SystemRestoreList is a list of SystemRestores
type SystemRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SystemRestore `json:"items"`
}
