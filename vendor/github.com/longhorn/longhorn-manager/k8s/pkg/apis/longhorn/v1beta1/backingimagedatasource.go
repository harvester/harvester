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

// BackingImageDataSourceSpec defines the desired state of the Longhorn backing image data source
type BackingImageDataSourceSpec struct {
	NodeID          string                     `json:"nodeID"`
	DiskUUID        string                     `json:"diskUUID"`
	DiskPath        string                     `json:"diskPath"`
	Checksum        string                     `json:"checksum"`
	SourceType      BackingImageDataSourceType `json:"sourceType"`
	Parameters      map[string]string          `json:"parameters"`
	FileTransferred bool                       `json:"fileTransferred"`
}

// BackingImageDataSourceStatus defines the observed state of the Longhorn backing image data source
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
// +kubebuilder:resource:shortName=lhbids
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.currentState`,description="The current state of the pod used to provision the backing image file from source"
// +kubebuilder:printcolumn:name="SourceType",type=string,JSONPath=`.spec.sourceType`,description="The data source type"
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeID`,description="The node the backing image file will be prepared on"
// +kubebuilder:printcolumn:name="DiskUUID",type=string,JSONPath=`.spec.diskUUID`,description="The disk the backing image file will be prepared on"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// BackingImageDataSource is where Longhorn stores backing image data source object.
type BackingImageDataSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Spec BackingImageDataSourceSpec `json:"spec,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Status BackingImageDataSourceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackingImageDataSourceList is a list of BackingImageDataSources.
type BackingImageDataSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackingImageDataSource `json:"items"`
}
