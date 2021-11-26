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

type BackingImageManagerSpec struct {
	Image         string            `json:"image"`
	NodeID        string            `json:"nodeID"`
	DiskUUID      string            `json:"diskUUID"`
	DiskPath      string            `json:"diskPath"`
	BackingImages map[string]string `json:"backingImages"`
}

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

type BackingImageManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              BackingImageManagerSpec   `json:"spec"`
	Status            BackingImageManagerStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BackingImageManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []BackingImageManager `json:"items"`
}
