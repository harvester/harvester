package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BackingImageDownloadState is replaced by BackingImageState.
type BackingImageDownloadState string

type BackingImageState string

const (
	BackingImageStatePending          = BackingImageState("pending")
	BackingImageStateStarting         = BackingImageState("starting")
	BackingImageStateReadyForTransfer = BackingImageState("ready-for-transfer")
	BackingImageStateReady            = BackingImageState("ready")
	BackingImageStateInProgress       = BackingImageState("in-progress")
	BackingImageStateFailed           = BackingImageState("failed")
	BackingImageStateUnknown          = BackingImageState("unknown")
)

type BackingImageDiskFileStatus struct {
	// +optional
	State BackingImageState `json:"state"`
	// +optional
	Progress int `json:"progress"`
	// +optional
	Message string `json:"message"`
	// +optional
	LastStateTransitionTime string `json:"lastStateTransitionTime"`
}

// BackingImageSpec defines the desired state of the Longhorn backing image
type BackingImageSpec struct {
	// +optional
	Disks map[string]string `json:"disks"`
	// +optional
	Checksum string `json:"checksum"`
	// +optional
	SourceType BackingImageDataSourceType `json:"sourceType"`
	// +optional
	SourceParameters map[string]string `json:"sourceParameters"`
	// Deprecated: This kind of info will be included in the related BackingImageDataSource.
	// +optional
	ImageURL string `json:"imageURL"`
}

// BackingImageStatus defines the observed state of the Longhorn backing image status
type BackingImageStatus struct {
	// +optional
	OwnerID string `json:"ownerID"`
	// +optional
	UUID string `json:"uuid"`
	// +optional
	Size int64 `json:"size"`
	// +optional
	Checksum string `json:"checksum"`
	// +optional
	// +nullable
	DiskFileStatusMap map[string]*BackingImageDiskFileStatus `json:"diskFileStatusMap"`
	// +optional
	// +nullable
	DiskLastRefAtMap map[string]string `json:"diskLastRefAtMap"`
	// Deprecated: Replaced by field `State` in `DiskFileStatusMap`.
	// +optional
	// +nullable
	DiskDownloadStateMap map[string]BackingImageDownloadState `json:"diskDownloadStateMap"`
	// Deprecated: Replaced by field `Progress` in `DiskFileStatusMap`.
	// +optional
	// +nullable
	DiskDownloadProgressMap map[string]int `json:"diskDownloadProgressMap"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhbi
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="UUID",type=string,JSONPath=`.status.uuid`,description="The system generated UUID"
// +kubebuilder:printcolumn:name="SourceType",type=string,JSONPath=`.spec.sourceType`,description="The source of the backing image file data"
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.status.size`,description="The backing image file size in each disk"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BackingImage is where Longhorn stores backing image object.
type BackingImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackingImageSpec   `json:"spec,omitempty"`
	Status BackingImageStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackingImageList is a list of BackingImages.
type BackingImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackingImage `json:"items"`
}

// Hub defines the current version (v1beta2) is the storage version
// so mark this as Hub
func (bi *BackingImage) Hub() {}
