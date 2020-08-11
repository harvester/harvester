package v1alpha1

import (
	"github.com/rancher/wrangler/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1alpha3 "kubevirt.io/client-go/api/v1alpha3"
)

var (
	VersionAssigned condition.Cond = "assigned" // version number was assigned to templateVersion object's status.Version
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Template struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TemplateSpec   `json:"spec,omitempty"`
	Status TemplateStatus `json:"status,omitempty"`
}

// +genclient
// +genclient:skipVerbs=update
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type TemplateVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TemplateVersionSpec   `json:"spec,omitempty"`
	Status TemplateVersionStatus `json:"status,omitempty"`
}

type TemplateSpec struct {
	Description      string `json:"description,omitempty"`
	DefaultVersionID string `json:"defaultVersionId,omitempty"`
}

type TemplateStatus struct {
	DefaultVersion int `json:"defaultVersion,omitempty"`
}

type TemplateVersionSpec struct {
	Description string                          `json:"description,omitempty"`
	TemplateID  string                          `json:"templateId,omitempty"`
	ImageID     string                          `json:"imageId,omitempty"`
	VM          virtv1alpha3.VirtualMachineSpec `json:"vm,omitempty"`
}

type TemplateVersionStatus struct {
	Version    int         `json:"version,omitempty"`
	Conditions []Condition `json:"conditions"`
}
