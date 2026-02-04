package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BackupBackingImageSpec defines the desired state of the Longhorn backing image backup
type BackupBackingImageSpec struct {
	// The backing image name.
	// +kubebuilder:validation:Required
	BackingImage string `json:"backingImage"`
	// The backup target name.
	// +optional
	// +nullable
	BackupTargetName string `json:"backupTargetName"`
	// The time to request run sync the remote backing image backup.
	// +optional
	// +nullable
	SyncRequestedAt metav1.Time `json:"syncRequestedAt"`
	// Is this CR created by user through API or UI.
	UserCreated bool `json:"userCreated"`
	// The labels of backing image backup.
	// +optional
	Labels map[string]string `json:"labels"`
}

// BackupBackingImageStatus defines the observed state of the Longhorn backing image backup
type BackupBackingImageStatus struct {
	// The backing image name.
	// +optional
	BackingImage string `json:"backingImage"`
	// The node ID on which the controller is responsible to reconcile this CR.
	// +optional
	OwnerID string `json:"ownerID"`
	// The checksum of the backing image.
	// +optional
	Checksum string `json:"checksum"`
	// The backing image backup URL.
	// +optional
	URL string `json:"url"`
	// The backing image size.
	// +optional
	Size int64 `json:"size"`
	// The labels of backing image backup.
	// +optional
	// +nullable
	Labels map[string]string `json:"labels"`
	// The backing image backup creation state.
	// Can be "", "InProgress", "Completed", "Error", "Unknown".
	// +optional
	State BackupState `json:"state"`
	// The backing image backup progress.
	// +optional
	Progress int `json:"progress"`
	// The error message when taking the backing image backup.
	// +optional
	Error string `json:"error,omitempty"`
	// The error messages when listing or inspecting backing image backup.
	// +optional
	// +nullable
	Messages map[string]string `json:"messages"`
	// The address of the backing image manager that runs backing image backup.
	// +optional
	ManagerAddress string `json:"managerAddress"`
	// The backing image backup upload finished time.
	// +optional
	BackupCreatedAt string `json:"backupCreatedAt"`
	// The last time that the backing image backup was synced with the remote backup target.
	// +optional
	// +nullable
	LastSyncedAt metav1.Time `json:"lastSyncedAt"`
	// Compression method
	// +optional
	CompressionMethod BackupCompressionMethod `json:"compressionMethod"`
	// Record the secret if this backup backing image is encrypted
	// +optional
	Secret string `json:"secret"`
	// Record the secret namespace if this backup backing image is encrypted
	// +optional
	SecretNamespace string `json:"secretNamespace"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhbbi
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="BackingImage",type=string,JSONPath=`.status.backingImage`,description="The backing image name"
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.status.size`,description="The backing image size"
// +kubebuilder:printcolumn:name="BackupCreatedAt",type=string,JSONPath=`.status.backupCreatedAt`,description="The backing image backup upload finished time"
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`,description="The backing image backup state"
// +kubebuilder:printcolumn:name="LastSyncedAt",type=string,JSONPath=`.status.lastSyncedAt`,description="The last synced time"

// BackupBackingImage is where Longhorn stores backing image backup object.
type BackupBackingImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupBackingImageSpec   `json:"spec,omitempty"`
	Status BackupBackingImageStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackupBackingImageList is a list of backing image backup.
type BackupBackingImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupBackingImage `json:"items"`
}
