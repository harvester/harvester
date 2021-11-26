package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	DataSourceTypeDownloadParameterURL = "url"
)

type BackingImageDataSourceType string

const (
	BackingImageDataSourceTypeDownload         = BackingImageDataSourceType("download")
	BackingImageDataSourceTypeUpload           = BackingImageDataSourceType("upload")
	BackingImageDataSourceTypeExportFromVolume = BackingImageDataSourceType("export-from-volume")
)

type BackingImageDataSourceSpec struct {
	NodeID          string                     `json:"nodeID"`
	DiskUUID        string                     `json:"diskUUID"`
	DiskPath        string                     `json:"diskPath"`
	Checksum        string                     `json:"checksum"`
	SourceType      BackingImageDataSourceType `json:"sourceType"`
	Parameters      map[string]string          `json:"parameters"`
	FileTransferred bool                       `json:"fileTransferred"`
}

type BackingImageDataSourceStatus struct {
	OwnerID           string            `json:"ownerID"`
	RunningParameters map[string]string `json:"runningParameters"`
	CurrentState      BackingImageState `json:"currentState"`
	Size              int64             `json:"size"`
	Progress          int               `json:"progress"`
	Checksum          string            `json:"checksum"`
	Message           string            `json:"message"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BackingImageDataSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              BackingImageDataSourceSpec   `json:"spec"`
	Status            BackingImageDataSourceStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BackingImageDataSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []BackingImageDataSource `json:"items"`
}
