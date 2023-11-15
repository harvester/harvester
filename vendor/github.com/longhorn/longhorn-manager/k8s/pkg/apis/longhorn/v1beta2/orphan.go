package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type OrphanType string

const (
	OrphanTypeReplica = OrphanType("replica")
)

const (
	OrphanConditionTypeError         = "Error"
	OrphanConditionTypeDataCleanable = "DataCleanable"

	OrphanConditionTypeDataCleanableReasonNodeUnavailable = "NodeUnavailable"
	OrphanConditionTypeDataCleanableReasonNodeEvicted     = "NodeEvicted"
	OrphanConditionTypeDataCleanableReasonDiskInvalid     = "DiskInvalid"
	OrphanConditionTypeDataCleanableReasonDiskEvicted     = "DiskEvicted"
	OrphanConditionTypeDataCleanableReasonDiskChanged     = "DiskChanged"
)

const (
	OrphanDataName = "DataName"
	OrphanDiskName = "DiskName"
	OrphanDiskUUID = "DiskUUID"
	OrphanDiskPath = "DiskPath"
	OrphanDiskType = "DiskType"
)

// OrphanSpec defines the desired state of the Longhorn orphaned data
type OrphanSpec struct {
	// The node ID on which the controller is responsible to reconcile this orphan CR.
	// +optional
	NodeID string `json:"nodeID"`
	// The type of the orphaned data.
	// Can be "replica".
	// +optional
	Type OrphanType `json:"orphanType"`

	// The parameters of the orphaned data
	// +optional
	Parameters map[string]string `json:"parameters"`
}

// OrphanStatus defines the observed state of the Longhorn orphaned data
type OrphanStatus struct {
	// +optional
	OwnerID string `json:"ownerID"`
	// +optional
	// +nullable
	Conditions []Condition `json:"conditions"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lho
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.orphanType`,description="The type of the orphan"
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeID`,description="The node that the orphan is on"
// Orphan is where Longhorn stores orphan object.
type Orphan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OrphanSpec   `json:"spec,omitempty"`
	Status OrphanStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OrphanList is a list of orphans.
type OrphanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Orphan `json:"items"`
}
