package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DownloderStatus string
type DownloaderCondsType string

var (
	ImageDownloaderStatusProgressing DownloderStatus = "Progressing"
	ImageDownloaderStatusReady       DownloderStatus = "Ready"

	DownloaderCondsReconciling DownloaderCondsType = "Reconciling"
	DownloaderCondsReady       DownloaderCondsType = "Ready"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=vmimagedownloader;vmimagedownloaders,scope=Namespaced
// +kubebuilder:printcolumn:name="VMImage",type=string,JSONPath=`.spec.imageName`
// +kubebuilder:printcolumn:name="CompressType",type="string",JSONPath=`.spec.compressType`
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=`.status.status`
// +kubebuilder:subresource:status

type VirtualMachineImageDownloader struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineImageDownloaderSpec   `json:"spec"`
	Status VirtualMachineImageDownloaderStatus `json:"status,omitempty"`
}

type VirtualMachineImageDownloaderSpec struct {
	// name of the vm image
	// +kubebuilder:validation:Required
	ImageName string `json:"imageName"`

	// compress type of the vm image
	// +kubebuilder:validation:Enum:=qcow2
	// +kubebuilder:default:=qcow2
	// +optional
	CompressType string `json:"compressType,omitempty"`
}

type VirtualMachineImageDownloaderStatus struct {
	// the conditions of the vm image downloader
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:={}
	Conditions []VirtualMachineImageDownloaderCondition `json:"conditions,omitempty"`

	// the current status of the vm image downloader
	// +kubebuilder:validation:Enum:=Progressing;Ready
	// +kubebuilder:default:=Progressing
	Status DownloderStatus `json:"status,omitempty"`

	// the url of the vm image
	// +optional
	DownloadURL string `json:"downloadUrl,omitempty"`
}

type VirtualMachineImageDownloaderCondition struct {
	Type               DownloaderCondsType    `json:"type"`
	Status             corev1.ConditionStatus `json:"status"`
	LastTransitionTime metav1.Time            `json:"lastTransitionTime"`
	Reason             string                 `json:"reason,omitempty"`
	Message            string                 `json:"message,omitempty"`
}
