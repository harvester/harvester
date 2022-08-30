package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type BackingImageManagerState string

const (
	BackingImageManagerStateError    = BackingImageManagerState("error")
	BackingImageManagerStateRunning  = BackingImageManagerState("running")
	BackingImageManagerStateStopped  = BackingImageManagerState("stopped")
	BackingImageManagerStateStarting = BackingImageManagerState("starting")
	BackingImageManagerStateUnknown  = BackingImageManagerState("unknown")
)

type BackingImageFileInfo struct {
	// +optional
	Name string `json:"name"`
	// +optional
	UUID string `json:"uuid"`
	// +optional
	Size int64 `json:"size"`
	// +optional
	State BackingImageState `json:"state"`
	// +optional
	CurrentChecksum string `json:"currentChecksum"`
	// +optional
	Message string `json:"message"`
	// +optional
	SendingReference int `json:"sendingReference"`
	// +optional
	SenderManagerAddress string `json:"senderManagerAddress"`
	// +optional
	Progress int `json:"progress"`
	// Deprecated: This field is useless now. The manager of backing image files doesn't care if a file is downloaded and how.
	// +optional
	URL string `json:"url"`
	// Deprecated: This field is useless.
	// +optional
	Directory string `json:"directory"`
	// Deprecated: This field is renamed to `Progress`.
	// +optional
	DownloadProgress int `json:"downloadProgress"`
}

// BackingImageManagerSpec defines the desired state of the Longhorn backing image manager
type BackingImageManagerSpec struct {
	// +optional
	Image string `json:"image"`
	// +optional
	NodeID string `json:"nodeID"`
	// +optional
	DiskUUID string `json:"diskUUID"`
	// +optional
	DiskPath string `json:"diskPath"`
	// +optional
	BackingImages map[string]string `json:"backingImages"`
}

// BackingImageManagerStatus defines the observed state of the Longhorn backing image manager
type BackingImageManagerStatus struct {
	// +optional
	OwnerID string `json:"ownerID"`
	// +optional
	CurrentState BackingImageManagerState `json:"currentState"`
	// +optional
	// +nullable
	BackingImageFileMap map[string]BackingImageFileInfo `json:"backingImageFileMap"`
	// +optional
	IP string `json:"ip"`
	// +optional
	StorageIP string `json:"storageIP"`
	// +optional
	APIMinVersion int `json:"apiMinVersion"`
	// +optional
	APIVersion int `json:"apiVersion"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhbim
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.currentState`,description="The current state of the manager"
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`,description="The image the manager pod will use"
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeID`,description="The node the manager is on"
// +kubebuilder:printcolumn:name="DiskUUID",type=string,JSONPath=`.spec.diskUUID`,description="The disk the manager is responsible for"
// +kubebuilder:printcolumn:name="DiskPath",type=string,JSONPath=`.spec.diskPath`,description="The disk path the manager is using"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BackingImageManager is where Longhorn stores backing image manager object.
type BackingImageManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackingImageManagerSpec   `json:"spec,omitempty"`
	Status BackingImageManagerStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackingImageManagerList is a list of BackingImageManagers.
type BackingImageManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackingImageManager `json:"items"`
}
