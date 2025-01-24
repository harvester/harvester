package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	// error messages captured from the gRPC error code
	ReplicaRebuildFailedCanceledErrorMSG         = "rpc error: code = Canceled"
	ReplicaRebuildFailedDeadlineExceededErrorMSG = "rpc error: code = DeadlineExceeded"
	ReplicaRebuildFailedUnavailableErrorMSG      = "rpc error: code = Unavailable"
)

const (
	ReplicaConditionTypeRebuildFailed                = "RebuildFailed"
	ReplicaConditionTypeWaitForBackingImage          = "WaitForBackingImage"
	ReplicaConditionReasonWaitForBackingImageFailed  = "GetBackingImageFailed"
	ReplicaConditionReasonWaitForBackingImageWaiting = "Waiting"

	ReplicaConditionReasonRebuildFailedDisconnection = "Disconnection"
	ReplicaConditionReasonRebuildFailedGeneral       = "General"
)

// ReplicaSpec defines the desired state of the Longhorn replica
type ReplicaSpec struct {
	InstanceSpec `json:""`
	// +optional
	EngineName string `json:"engineName"`
	// +optional
	// MigrationEngineName is indicating the migrating engine which current connected to this replica. This is only
	// used for live migration of v2 data engine
	MigrationEngineName string `json:"migrationEngineName"`
	// +optional
	// HealthyAt is set the first time a replica becomes read/write in an engine after creation or rebuild. HealthyAt
	// indicates the time the last successful rebuild occurred. When HealthyAt is set, a replica is likely to have
	// useful (though possibly stale) data. HealthyAt is cleared before a rebuild. HealthyAt may be later than the
	// corresponding entry in an engine's replicaTransitionTimeMap because it is set when the volume controller
	// acknowledges the change.
	HealthyAt string `json:"healthyAt"`
	// +optional
	// LastHealthyAt is set every time a replica becomes read/write in an engine. Unlike HealthyAt, LastHealthyAt is
	// never cleared. LastHealthyAt is not a reliable indicator of the state of a replica's data. For example, a
	// replica with LastHealthyAt set may be in the middle of a rebuild. However, because it is never cleared, it can be
	// compared to LastFailedAt to help prevent dangerous replica deletion in some corner cases. LastHealthyAt may be
	// later than the corresponding entry in an engine's replicaTransitionTimeMap because it is set when the volume
	// controller acknowledges the change.
	LastHealthyAt string `json:"lastHealthyAt"`
	// +optional
	// FailedAt is set when a running replica fails or when a running engine is unable to use a replica for any reason.
	// FailedAt indicates the time the failure occurred. When FailedAt is set, a replica is likely to have useful
	// (though possibly stale) data. A replica with FailedAt set must be rebuilt from a non-failed replica (or it can
	// be used in a salvage if all replicas are failed). FailedAt is cleared before a rebuild or salvage. FailedAt may
	// be later than the corresponding entry in an engine's replicaTransitionTimeMap because it is set when the volume
	// controller acknowledges the change.
	FailedAt string `json:"failedAt"`
	// +optional
	// LastFailedAt is always set at the same time as FailedAt. Unlike FailedAt, LastFailedAt is never cleared.
	// LastFailedAt is not a reliable indicator of the state of a replica's data. For example, a replica with
	// LastFailedAt may already be healthy and in use again. However, because it is never cleared, it can be compared to
	// LastHealthyAt to help prevent dangerous replica deletion in some corner cases. LastFailedAt may be later than the
	// corresponding entry in an engine's replicaTransitionTimeMap because it is set when the volume controller
	// acknowledges the change.
	LastFailedAt string `json:"lastFailedAt"`
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
	UnmapMarkDiskChainRemovedEnabled bool `json:"unmapMarkDiskChainRemovedEnabled"`
	// +optional
	RebuildRetryCount int `json:"rebuildRetryCount"`
	// +optional
	EvictionRequested bool `json:"evictionRequested"`
	// +optional
	SnapshotMaxCount int `json:"snapshotMaxCount"`
	// +kubebuilder:validation:Type=string
	// +optional
	SnapshotMaxSize int64 `json:"snapshotMaxSize,string"`
}

// ReplicaStatus defines the observed state of the Longhorn replica
type ReplicaStatus struct {
	InstanceStatus `json:""`
	// Deprecated: Replaced by field `spec.evictionRequested`.
	// +optional
	EvictionRequested bool `json:"evictionRequested"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhr
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Data Engine",type=string,JSONPath=`.spec.dataEngine`,description="The data engine of the replica"
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
