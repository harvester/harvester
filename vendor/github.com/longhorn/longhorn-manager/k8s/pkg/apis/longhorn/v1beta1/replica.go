package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ReplicaSpec defines the desired state of the Longhorn replica
type ReplicaSpec struct {
	InstanceSpec            `json:""`
	EngineName              string `json:"engineName"`
	HealthyAt               string `json:"healthyAt"`
	FailedAt                string `json:"failedAt"`
	DiskID                  string `json:"diskID"`
	DiskPath                string `json:"diskPath"`
	DataDirectoryName       string `json:"dataDirectoryName"`
	BackingImage            string `json:"backingImage"`
	Active                  bool   `json:"active"`
	HardNodeAffinity        string `json:"hardNodeAffinity"`
	RevisionCounterDisabled bool   `json:"revisionCounterDisabled"`

	RebuildRetryCount int `json:"rebuildRetryCount"`
	// Deprecated
	DataPath string `json:"dataPath"`
	// Deprecated. Rename to BackingImage
	BaseImage string `json:"baseImage"`
}

// ReplicaStatus defines the observed state of the Longhorn replica
type ReplicaStatus struct {
	InstanceStatus    `json:""`
	EvictionRequested bool `json:"evictionRequested"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhr
// +kubebuilder:subresource:status
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

	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Spec ReplicaSpec `json:"spec,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Status ReplicaStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ReplicaList is a list of Replicas.
type ReplicaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Replica `json:"items"`
}
