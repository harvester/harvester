package v1beta1

import (
	"github.com/rancher/wrangler/pkg/condition"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ImageInitialized        condition.Cond = "Initialized"
	ImageImported           condition.Cond = "Imported"
	ImageRetryLimitExceeded condition.Cond = "RetryLimitExceeded"
)

const (
	VirtualMachineImageSourceTypeDownload     = "download"
	VirtualMachineImageSourceTypeUpload       = "upload"
	VirtualMachineImageSourceTypeExportVolume = "export-from-volume"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=vmimage;vmimages,scope=Namespaced
// +kubebuilder:printcolumn:name="DISPLAY-NAME",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="SIZE",type=integer,JSONPath=`.status.size`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`

type VirtualMachineImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineImageSpec   `json:"spec"`
	Status VirtualMachineImageStatus `json:"status,omitempty"`
}

type VirtualMachineImageSpec struct {
	// +optional
	Description string `json:"description,omitempty"`

	// +kubebuilder:validation:Required
	DisplayName string `json:"displayName"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=download;upload;export-from-volume
	SourceType string `json:"sourceType"`

	// +optional
	PVCName string `json:"pvcName"`

	// +optional
	PVCNamespace string `json:"pvcNamespace"`

	// +optional
	URL string `json:"url"`

	// +optional
	Checksum string `json:"checksum"`

	// +optional
	StorageClassParameters map[string]string `json:"storageClassParameters"`

	// +optinoal
	// +kubebuilder:default:=3
	// +kubebuilder:validation:Minimum:=0
	// +kubebuilder:validation:Maximum:=10
	// +kubebuilder:validation:Optional
	Retry int `json:"retry" default:"3"`
}

type VirtualMachineImageStatus struct {
	// +optional
	AppliedURL string `json:"appliedUrl,omitempty"`

	// +optional
	Progress int `json:"progress,omitempty"`

	// +optional
	Size int64 `json:"size,omitempty"`

	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`

	// +optional
	// +kubebuilder:default:=0
	// +kubebuilder:validation:Minimum:=0
	Failed int `json:"failed"`

	// +optinoal
	// +kubebuilder:validation:Optional
	LastFailedTime string `json:"lastFailedTime,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

type Condition struct {
	// Type of the condition.
	Type condition.Cond `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status v1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	LastUpdateTime string `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// Human-readable message indicating details about last transition
	Message string `json:"message,omitempty"`
}
