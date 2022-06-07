package v1beta2

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
	// +optional
	// +nullable
	SyncRequestedAt metav1.Time `json:"syncRequestedAt"`
	// The snapshot name.
	// +optional
	SnapshotName string `json:"snapshotName"`
	// The labels of snapshot backup.
	// +optional
	Labels map[string]string `json:"labels"`
}

// BackupStatus defines the observed state of the Longhorn backup
type BackupStatus struct {
	// The node ID on which the controller is responsible to reconcile this backup CR.
	// +optional
	OwnerID string `json:"ownerID"`
	// The backup creation state.
	// Can be "", "InProgress", "Completed", "Error", "Unknown".
	// +optional
	State BackupState `json:"state"`
	// The snapshot backup progress.
	// +optional
	Progress int `json:"progress"`
	// The address of the replica that runs snapshot backup.
	// +optional
	ReplicaAddress string `json:"replicaAddress"`
	// The error message when taking the snapshot backup.
	// +optional
	Error string `json:"error,omitempty"`
	// The snapshot backup URL.
	// +optional
	URL string `json:"url"`
	// The snapshot name.
	// +optional
	SnapshotName string `json:"snapshotName"`
	// The snapshot creation time.
	// +optional
	SnapshotCreatedAt string `json:"snapshotCreatedAt"`
	// The snapshot backup upload finished time.
	// +optional
	BackupCreatedAt string `json:"backupCreatedAt"`
	// The snapshot size.
	// +optional
	Size string `json:"size"`
	// The labels of snapshot backup.
	// +optional
	// +nullable
	Labels map[string]string `json:"labels"`
	// The error messages when calling longhorn engine on listing or inspecting backups.
	// +optional
	// +nullable
	Messages map[string]string `json:"messages"`
	// The volume name.
	// +optional
	VolumeName string `json:"volumeName"`
	// The volume size.
	// +optional
	VolumeSize string `json:"volumeSize"`
	// The volume creation time.
	// +optional
	VolumeCreated string `json:"volumeCreated"`
	// The volume's backing image name.
	// +optional
	VolumeBackingImageName string `json:"volumeBackingImageName"`
	// The last time that the backup was synced with the remote backup target.
	// +optional
	// +nullable
	LastSyncedAt metav1.Time `json:"lastSyncedAt"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhb
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="SnapshotName",type=string,JSONPath=`.status.snapshotName`,description="The snapshot name"
// +kubebuilder:printcolumn:name="SnapshotSize",type=string,JSONPath=`.status.size`,description="The snapshot size"
// +kubebuilder:printcolumn:name="SnapshotCreatedAt",type=string,JSONPath=`.status.snapshotCreatedAt`,description="The snapshot creation time"
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`,description="The backup state"
// +kubebuilder:printcolumn:name="LastSyncedAt",type=string,JSONPath=`.status.lastSyncedAt`,description="The backup last synced time"

// Backup is where Longhorn stores backup object.
type Backup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupSpec   `json:"spec,omitempty"`
	Status BackupStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackupList is a list of Backups.
type BackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Backup `json:"items"`
}
