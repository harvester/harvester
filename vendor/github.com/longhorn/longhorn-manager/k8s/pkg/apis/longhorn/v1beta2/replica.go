package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ReplicaSpec defines the desired state of the Longhorn replica
type ReplicaSpec struct {
	InstanceSpec `json:""`
	// +optional
	EngineName string `json:"engineName"`
	// +optional
	HealthyAt string `json:"healthyAt"`
	// +optional
	FailedAt string `json:"failedAt"`
	// +optional
	DiskID string `json:"diskID"`
	// +optional
	DiskPath string `json:"diskPath"`
	// +optional
	DataDirectoryName string `json:"dataDirectoryName"`
	// +optional
	BackingImage string `json:"backingImage"`
	// +optional
	Active bool `json:"active"`
	// +optional
	HardNodeAffinity string `json:"hardNodeAffinity"`
	// +optional
	RevisionCounterDisabled bool `json:"revisionCounterDisabled"`
	// +optional
	RebuildRetryCount int `json:"rebuildRetryCount"`
	// Deprecated
	// +optional
	DataPath string `json:"dataPath"`
	// Deprecated. Rename to BackingImage
	// +optional
	BaseImage string `json:"baseImage"`
}

// ReplicaStatus defines the observed state of the Longhorn replica
type ReplicaStatus struct {
	InstanceStatus `json:""`
	// +optional
	EvictionRequested bool `json:"evictionRequested"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhr
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.currentState`,description="The current state of the replica"
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeID`,description="The node that the replica is on"
// +kubebuilder:printcolumn:name="Disk",type=string,JSONPath=`.spec.diskID`,description="The disk that the replica is on"
// +kubebuilder:printcolumn:name="InstanceManager",type=string,JSONPath=`.status.instanceManagerName`,description="The instance manager of the replica"
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.status.currentImage`,description="The current image of the replica"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Replica is where Longhorn stores replica object.
type Replica struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReplicaSpec   `json:"spec,omitempty"`
	Status ReplicaStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ReplicaList is a list of Replicas.
type ReplicaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Replica `json:"items"`
}
