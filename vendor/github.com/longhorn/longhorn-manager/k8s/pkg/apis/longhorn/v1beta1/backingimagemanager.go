package v1beta1

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
	Name                 string            `json:"name"`
	UUID                 string            `json:"uuid"`
	Size                 int64             `json:"size"`
	State                BackingImageState `json:"state"`
	CurrentChecksum      string            `json:"currentChecksum"`
	Message              string            `json:"message"`
	SendingReference     int               `json:"sendingReference"`
	SenderManagerAddress string            `json:"senderManagerAddress"`
	Progress             int               `json:"progress"`

	// Deprecated: This field is useless now. The manager of backing image files doesn't care if a file is downloaded and how.
	URL string `json:"url"`
	// Deprecated: This field is useless.
	Directory string `json:"directory"`
	// Deprecated: This field is renamed to `Progress`.
	DownloadProgress int `json:"downloadProgress"`
}

// BackingImageManagerSpec defines the desired state of the Longhorn backing image manager
type BackingImageManagerSpec struct {
	Image         string            `json:"image"`
	NodeID        string            `json:"nodeID"`
	DiskUUID      string            `json:"diskUUID"`
	DiskPath      string            `json:"diskPath"`
	BackingImages map[string]string `json:"backingImages"`
}

// BackingImageManagerStatus defines the observed state of the Longhorn backing image manager
type BackingImageManagerStatus struct {
	OwnerID             string                          `json:"ownerID"`
	CurrentState        BackingImageManagerState        `json:"currentState"`
	BackingImageFileMap map[string]BackingImageFileInfo `json:"backingImageFileMap"`
	IP                  string                          `json:"ip"`
	APIMinVersion       int                             `json:"apiMinVersion"`
	APIVersion          int                             `json:"apiVersion"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhbim
// +kubebuilder:subresource:status
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

	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Spec BackingImageManagerSpec `json:"spec,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Status BackingImageManagerStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackingImageManagerList is a list of BackingImageManagers.
type BackingImageManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackingImageManager `json:"items"`
}
