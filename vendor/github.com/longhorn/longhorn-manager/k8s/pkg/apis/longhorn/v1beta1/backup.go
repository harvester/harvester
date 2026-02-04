package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type BackupState string

const (
	BackupStateNew        = BackupState("")
	BackupStatePending    = BackupState("Pending")
	BackupStateInProgress = BackupState("InProgress")
	BackupStateCompleted  = BackupState("Completed")
	BackupStateError      = BackupState("Error")
	BackupStateUnknown    = BackupState("Unknown")
)

// BackupSpec defines the desired state of the Longhorn backup
type BackupSpec struct {
	// The time to request run sync the remote backup.
	SyncRequestedAt metav1.Time `json:"syncRequestedAt"`
	// The snapshot name.
	SnapshotName string `json:"snapshotName"`
	// The labels of snapshot backup.
	Labels map[string]string `json:"labels"`
}

// BackupStatus defines the observed state of the Longhorn backup
type BackupStatus struct {
	// The node ID on which the controller is responsible to reconcile this backup CR.
	OwnerID string `json:"ownerID"`
	// The backup creation state.
	// Can be "", "InProgress", "Completed", "Error", "Unknown".
	State BackupState `json:"state"`
	// The snapshot backup progress.
	Progress int `json:"progress"`
	// The address of the replica that runs snapshot backup.
	ReplicaAddress string `json:"replicaAddress"`
	// The error message when taking the snapshot backup.
	Error string `json:"error,omitempty"`
	// The snapshot backup URL.
	URL string `json:"url"`
	// The snapshot name.
	SnapshotName string `json:"snapshotName"`
	// The snapshot creation time.
	SnapshotCreatedAt string `json:"snapshotCreatedAt"`
	// The snapshot backup upload finished time.
	BackupCreatedAt string `json:"backupCreatedAt"`
	// The snapshot size.
	Size string `json:"size"`
	// The labels of snapshot backup.
	Labels map[string]string `json:"labels"`
	// The error messages when calling longhorn engine on listing or inspecting backups.
	Messages map[string]string `json:"messages"`
	// The volume name.
	VolumeName string `json:"volumeName"`
	// The volume size.
	VolumeSize string `json:"volumeSize"`
	// The volume creation time.
	VolumeCreated string `json:"volumeCreated"`
	// The volume's backing image name.
	VolumeBackingImageName string `json:"volumeBackingImageName"`
	// The last time that the backup was synced with the remote backup target.
	LastSyncedAt metav1.Time `json:"lastSyncedAt"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhb
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="SnapshotName",type=string,JSONPath=`.status.snapshotName`,description="The snapshot name"
// +kubebuilder:printcolumn:name="SnapshotSize",type=string,JSONPath=`.status.size`,description="The snapshot size"
// +kubebuilder:printcolumn:name="SnapshotCreatedAt",type=string,JSONPath=`.status.snapshotCreatedAt`,description="The snapshot creation time"
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`,description="The backup state"
// +kubebuilder:printcolumn:name="LastSyncedAt",type=string,JSONPath=`.status.lastSyncedAt`,description="The backup last synced time"

// Backup is where Longhorn stores backup object.
type Backup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Spec BackupSpec `json:"spec,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Status BackupStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackupList is a list of Backups.
type BackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Backup `json:"items"`
}
