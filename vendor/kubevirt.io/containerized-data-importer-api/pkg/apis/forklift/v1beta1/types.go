package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OvirtVolumePopulatorKind is the type of the CR used to populator a volume from an oVirt disk
var OvirtVolumePopulatorKind = "OvirtVolumePopulator"

// OvirtVolumePopulator is the CR used to populator a volume from an oVirt disk
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
// +kubebuilder:resource:shortName={ovvp,ovvps}
type OvirtVolumePopulator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OvirtVolumePopulatorSpec `json:"spec"`
	// +optional
	Status OvirtVolumePopulatorStatus `json:"status,omitempty"`
}

// OvirtVolumePopulatorSpec is the spec of the OvirtVolumePopulator CR
type OvirtVolumePopulatorSpec struct {
	EngineURL string `json:"engineUrl"`
	SecretRef string `json:"secretRef"`
	DiskID    string `json:"diskId"`
	// The network attachment definition that should be used for disk transfer.
	TransferNetwork *string `json:"transferNetwork,omitempty"`
}

// OvirtVolumePopulatorStatus is the status of the OvirtVolumePopulator CR
type OvirtVolumePopulatorStatus struct {
	// +optional
	Progress *string `json:"progress,omitempty"`
}

// OvirtVolumePopulatorList contains a list of OvirtVolumePopulators
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OvirtVolumePopulatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OvirtVolumePopulator `json:"items"`
}

// OpenstackVolumePopulatorKind is the type of the CR used to populator a volume from an Openstack image
var OpenstackVolumePopulatorKind = "OpenstackVolumePopulator"

// OpenstackVolumePopulator is the CR used to populator a volume from an Openstack image
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
// +kubebuilder:resource:shortName={osvp,osvps}
type OpenstackVolumePopulator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenstackVolumePopulatorSpec `json:"spec"`
	// +optional
	Status OpenstackVolumePopulatorStatus `json:"status,omitempty"`
}

// OpenstackVolumePopulatorSpec is the spec of the OpenstackVolumePopulator CR
type OpenstackVolumePopulatorSpec struct {
	IdentityURL string `json:"identityUrl"`
	SecretRef   string `json:"secretRef"`
	ImageID     string `json:"imageId"`
	// The network attachment definition that should be used for disk transfer.
	TransferNetwork *string `json:"transferNetwork,omitempty"`
}

// OpenstackVolumePopulatorStatus is the status of the OpenstackVolumePopulator CR
type OpenstackVolumePopulatorStatus struct {
	// +optional
	Progress *string `json:"progress,omitempty"`
}

// OpenstackVolumePopulatorList contains a list of OpenstackVolumePopulators
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OpenstackVolumePopulatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenstackVolumePopulator `json:"items"`
}
