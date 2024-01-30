package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	DataSourceTypeDownloadParameterURL      = "url"
	DataSourceTypeExportParameterExportType = "export-type"
	DataSourceTypeExportParameterVolumeName = "volume-name"
)

// +kubebuilder:validation:Enum=download;upload;export-from-volume
type BackingImageDataSourceType string

const (
	BackingImageDataSourceTypeDownload         = BackingImageDataSourceType("download")
	BackingImageDataSourceTypeUpload           = BackingImageDataSourceType("upload")
	BackingImageDataSourceTypeExportFromVolume = BackingImageDataSourceType("export-from-volume")

	DataSourceTypeExportFromVolumeParameterVolumeName    = "volume-name"
	DataSourceTypeExportFromVolumeParameterVolumeSize    = "volume-size"
	DataSourceTypeExportFromVolumeParameterSnapshotName  = "snapshot-name"
	DataSourceTypeExportFromVolumeParameterSenderAddress = "sender-address"
)

// BackingImageDataSourceSpec defines the desired state of the Longhorn backing image data source
type BackingImageDataSourceSpec struct {
	// +optional
	NodeID string `json:"nodeID"`
	// +optional
	UUID string `json:"uuid"`
	// +optional
	DiskUUID string `json:"diskUUID"`
	// +optional
	DiskPath string `json:"diskPath"`
	// +optional
	Checksum string `json:"checksum"`
	// +optional
	SourceType BackingImageDataSourceType `json:"sourceType"`
	// +optional
	Parameters map[string]string `json:"parameters"`
	// +optional
	FileTransferred bool `json:"fileTransferred"`
}

// BackingImageDataSourceStatus defines the observed state of the Longhorn backing image data source
type BackingImageDataSourceStatus struct {
	// +optional
	OwnerID string `json:"ownerID"`
	// +optional
	// +nullable
	RunningParameters map[string]string `json:"runningParameters"`
	// +optional
	CurrentState BackingImageState `json:"currentState"`
	// +optional
	IP string `json:"ip"`
	// +optional
	StorageIP string `json:"storageIP"`
	// +optional
	Size int64 `json:"size"`
	// +optional
	Progress int `json:"progress"`
	// +optional
	Checksum string `json:"checksum"`
	// +optional
	Message string `json:"message"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lhbids
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="UUID",type=string,JSONPath=`.spec.uuid`,description="The system generated UUID of the provisioned backing image file"
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.currentState`,description="The current state of the pod used to provision the backing image file from source"
// +kubebuilder:printcolumn:name="SourceType",type=string,JSONPath=`.spec.sourceType`,description="The data source type"
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.status.size`,description="The backing image file size"
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeID`,description="The node the backing image file will be prepared on"
// +kubebuilder:printcolumn:name="DiskUUID",type=string,JSONPath=`.spec.diskUUID`,description="The disk the backing image file will be prepared on"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BackingImageDataSource is where Longhorn stores backing image data source object.
type BackingImageDataSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackingImageDataSourceSpec   `json:"spec,omitempty"`
	Status BackingImageDataSourceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackingImageDataSourceList is a list of BackingImageDataSources.
type BackingImageDataSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackingImageDataSource `json:"items"`
}
