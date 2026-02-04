package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type SystemBackupState string

const (
	SystemBackupStateDeleting           = SystemBackupState("Deleting")
	SystemBackupStateError              = SystemBackupState("Error")
	SystemBackupStateGenerating         = SystemBackupState("Generating")
	SystemBackupStateNone               = SystemBackupState("")
	SystemBackupStateReady              = SystemBackupState("Ready")
	SystemBackupStateSyncing            = SystemBackupState("Syncing")
	SystemBackupStateUploading          = SystemBackupState("Uploading")
	SystemBackupStateVolumeBackup       = SystemBackupState("CreatingVolumeBackups")
	SystemBackupStateBackingImageBackup = SystemBackupState("CreatingBackingImageBackups")

	SystemBackupConditionTypeError = "Error"

	SystemBackupConditionReasonDelete   = "Delete"
	SystemBackupConditionReasonGenerate = "Generate"
	SystemBackupConditionReasonUpload   = "Upload"
	SystemBackupConditionReasonSync     = "Sync"
)

type SystemBackupCreateVolumeBackupPolicy string

const (
	SystemBackupCreateVolumeBackupPolicyAlways       = SystemBackupCreateVolumeBackupPolicy("always")
	SystemBackupCreateVolumeBackupPolicyDisabled     = SystemBackupCreateVolumeBackupPolicy("disabled")
	SystemBackupCreateVolumeBackupPolicyIfNotPresent = SystemBackupCreateVolumeBackupPolicy("if-not-present")
)

// SystemBackupSpec defines the desired state of the Longhorn SystemBackup
type SystemBackupSpec struct {
	// The create volume backup policy
	// Can be "if-not-present", "always" or "disabled"
	// +optional
	// +nullable
	VolumeBackupPolicy SystemBackupCreateVolumeBackupPolicy `json:"volumeBackupPolicy"`
}

// SystemBackupStatus defines the observed state of the Longhorn SystemBackup
type SystemBackupStatus struct {
	// The node ID of the responsible controller to reconcile this SystemBackup.
	// +optional
	OwnerID string `json:"ownerID"`
	// The saved Longhorn version.
	// +optional
	// +nullable
	Version string `json:"version"`
	// The saved Longhorn manager git commit.
	// +optional
	// +nullable
	GitCommit string `json:"gitCommit"`
	// The saved manager image.
	// +optional
	ManagerImage string `json:"managerImage"`
	// The system backup state.
	// +optional
	State SystemBackupState `json:"state"`
	// +optional
	// +nullable
	Conditions []Condition `json:"conditions"`
	// The system backup creation time.
	// +optional
	CreatedAt metav1.Time `json:"createdAt"`
	// The last time that the system backup was synced into the cluster.
	// +optional
	// +nullable
	LastSyncedAt metav1.Time `json:"lastSyncedAt"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhsb
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`,description="The system backup Longhorn version"
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`,description="The system backup state"
// +kubebuilder:printcolumn:name="Created",type=string,JSONPath=`.status.createdAt`,description="The system backup creation time"
// +kubebuilder:printcolumn:name="LastSyncedAt",type=string,JSONPath=`.status.lastSyncedAt`,description="The last time that the system backup was synced into the cluster"

// SystemBackup is where Longhorn stores system backup object
type SystemBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SystemBackupSpec   `json:"spec,omitempty"`
	Status SystemBackupStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SystemBackupList is a list of SystemBackups
type SystemBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SystemBackup `json:"items"`
}
