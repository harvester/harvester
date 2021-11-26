package v1beta1

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
	State                   BackingImageState `json:"state"`
	Progress                int               `json:"progress"`
	Message                 string            `json:"message"`
	LastStateTransitionTime string            `json:"lastStateTransitionTime"`
}

type BackingImageSpec struct {
	Disks            map[string]struct{}        `json:"disks"`
	Checksum         string                     `json:"checksum"`
	SourceType       BackingImageDataSourceType `json:"sourceType"`
	SourceParameters map[string]string          `json:"sourceParameters"`

	// Deprecated: This kind of info will be included in the related BackingImageDataSource.
	ImageURL string `json:"imageURL"`
}

type BackingImageStatus struct {
	OwnerID           string                                 `json:"ownerID"`
	UUID              string                                 `json:"uuid"`
	Size              int64                                  `json:"size"`
	Checksum          string                                 `json:"checksum"`
	DiskFileStatusMap map[string]*BackingImageDiskFileStatus `json:"diskFileStatusMap"`
	DiskLastRefAtMap  map[string]string                      `json:"diskLastRefAtMap"`

	// Deprecated: Replaced by field `State` in `DiskFileStatusMap`.
	DiskDownloadStateMap map[string]BackingImageDownloadState `json:"diskDownloadStateMap"`
	// Deprecated: Replaced by field `Progress` in `DiskFileStatusMap`.
	DiskDownloadProgressMap map[string]int `json:"diskDownloadProgressMap"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BackingImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              BackingImageSpec   `json:"spec"`
	Status            BackingImageStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BackingImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []BackingImage `json:"items"`
}
