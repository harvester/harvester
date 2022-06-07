package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SnapshotSpec defines the desired state of Longhorn Snapshot
type SnapshotSpec struct {
	// the volume that this snapshot belongs to.
	// This field is immutable after creation.
	// Required
	Volume string `json:"volume"`
	// require creating a new snapshot
	// +optional
	CreateSnapshot bool `json:"createSnapshot"`
	// The labels of snapshot
	// +optional
	// +nullable
	Labels map[string]string `json:"labels"`
}

// SnapshotStatus defines the observed state of Longhorn Snapshot
type SnapshotStatus struct {
	// +optional
	Parent string `json:"parent"`
	// +optional
	// +nullable
	Children map[string]bool `json:"children"`
	// +optional
	MarkRemoved bool `json:"markRemoved"`
	// +optional
	UserCreated bool `json:"userCreated"`
	// +optional
	CreationTime string `json:"creationTime"`
	// +optional
	Size int64 `json:"size"`
	// +optional
	// +nullable
	Labels map[string]string `json:"labels"`
	// +optional
	OwnerID string `json:"ownerID"`
	// +optional
	Error string `json:"error,omitempty"`
	// +optional
	RestoreSize int64 `json:"restoreSize"`
	// +optional
	ReadyToUse bool `json:"readyToUse"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhsnap
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Volume",type=string,JSONPath=`.spec.volume`,description="The volume that this snapshot belongs to"
// +kubebuilder:printcolumn:name="CreationTime",type=string,JSONPath=`.status.creationTime`,description="Timestamp when the point-in-time snapshot was taken"
// +kubebuilder:printcolumn:name="ReadyToUse",type=boolean,JSONPath=`.status.readyToUse`,description="Indicates if the snapshot is ready to be used to restore/backup a volume"
// +kubebuilder:printcolumn:name="RestoreSize",type=string,JSONPath=`.status.restoreSize`,description="Represents the minimum size of volume required to rehydrate from this snapshot"
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.status.size`,description="The actual size of the snapshot"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Snapshot is the Schema for the snapshots API
type Snapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SnapshotSpec   `json:"spec,omitempty"`
	Status SnapshotStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SnapshotList contains a list of Snapshot
type SnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Snapshot `json:"items"`
}
