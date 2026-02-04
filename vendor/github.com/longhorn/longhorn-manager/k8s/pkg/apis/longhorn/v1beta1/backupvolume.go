package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BackupVolumeSpec defines the desired state of the Longhorn backup volume
type BackupVolumeSpec struct {
	// The time to request run sync the remote backup volume.
	SyncRequestedAt metav1.Time `json:"syncRequestedAt"`
}

// BackupVolumeStatus defines the observed state of the Longhorn backup volume
type BackupVolumeStatus struct {
	// The node ID on which the controller is responsible to reconcile this backup volume CR.
	OwnerID string `json:"ownerID"`
	// The backup volume config last modification time.
	LastModificationTime metav1.Time `json:"lastModificationTime"`
	// The backup volume size.
	Size string `json:"size"`
	// The backup volume labels.
	Labels map[string]string `json:"labels"`
	// The backup volume creation time.
	CreatedAt string `json:"createdAt"`
	// The latest volume backup name.
	LastBackupName string `json:"lastBackupName"`
	// The latest volume backup time.
	LastBackupAt string `json:"lastBackupAt"`
	// The backup volume block count.
	DataStored string `json:"dataStored"`
	// The error messages when call longhorn engine on list or inspect backup volumes.
	Messages map[string]string `json:"messages"`
	// The backing image name.
	BackingImageName string `json:"backingImageName"`
	// the backing image checksum.
	BackingImageChecksum string `json:"backingImageChecksum"`
	// The last time that the backup volume was synced into the cluster.
	LastSyncedAt metav1.Time `json:"lastSyncedAt"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhbv
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="CreatedAt",type=string,JSONPath=`.status.createdAt`,description="The backup volume creation time"
// +kubebuilder:printcolumn:name="LastBackupName",type=string,JSONPath=`.status.lastBackupName`,description="The backup volume last backup name"
// +kubebuilder:printcolumn:name="LastBackupAt",type=string,JSONPath=`.status.lastBackupAt`,description="The backup volume last backup time"
// +kubebuilder:printcolumn:name="LastSyncedAt",type=string,JSONPath=`.status.lastSyncedAt`,description="The backup volume last synced time"

// BackupVolume is where Longhorn stores backup volume object.
type BackupVolume struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Spec BackupVolumeSpec `json:"spec,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Status BackupVolumeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackupVolumeList is a list of BackupVolumes.
type BackupVolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupVolume `json:"items"`
}
