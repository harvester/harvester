package v1beta1

import (
	"github.com/rancher/wrangler/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kv1 "kubevirt.io/client-go/api/v1"
)

var (
	VersionAssigned condition.Cond = "assigned" // version number was assigned to templateVersion object's status.Version
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=vmtemplate;vmtemplates,scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="VERSION_ID",type=string,JSONPath=`.spec.defaultVersionId`
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,priority=10,JSONPath=`.spec.description`
// +kubebuilder:printcolumn:name="VERSION_DEFAULT",type=integer,priority=8,JSONPath=`.status.defaultVersion`
// +kubebuilder:printcolumn:name="VERSION_LATEST",type=integer,priority=8,JSONPath=`.status.latestVersion`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`

type VirtualMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineTemplateSpec   `json:"spec,omitempty"`
	Status VirtualMachineTemplateStatus `json:"status,omitempty"`
}

type VirtualMachineTemplateSpec struct {
	// +optional
	DefaultVersionID string `json:"defaultVersionId"`

	// +optional
	Description string `json:"description,omitempty"`
}

type VirtualMachineTemplateStatus struct {
	// +optional
	DefaultVersion int `json:"defaultVersion,omitempty"`

	// +optional
	LatestVersion int `json:"latestVersion,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=vmtemplateversion;vmtemplateversions,scope=Namespaced
// +kubebuilder:printcolumn:name="TEMPLATE_ID",type=string,JSONPath=`.spec.templatedId`
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,priority=10,JSONPath=`.spec.description`
// +kubebuilder:printcolumn:name="VERSION",type=integer,JSONPath=`.status.version`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`

type VirtualMachineTemplateVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineTemplateVersionSpec   `json:"spec,omitempty"`
	Status VirtualMachineTemplateVersionStatus `json:"status,omitempty"`
}

type VirtualMachineTemplateVersionSpec struct {
	// +kubebuilder:validation:Required
	TemplateID string `json:"templateId"`

	// +optional
	Description string `json:"description,omitempty"`

	// +optional
	ImageID string `json:"imageId,omitempty"`

	// +optional
	KeyPairIDs []string `json:"keyPairIds,omitempty"`

	// +optional
	VM kv1.VirtualMachineSpec `json:"vm,omitempty"`
}

type VirtualMachineTemplateVersionStatus struct {
	// +optional
	Version int `json:"version,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}
