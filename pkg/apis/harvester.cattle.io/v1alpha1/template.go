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

type VirtualMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineTemplateSpec   `json:"spec,omitempty"`
	Status VirtualMachineTemplateStatus `json:"status,omitempty"`
}

// +genclient
// +genclient:skipVerbs=update
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VirtualMachineTemplateVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineTemplateVersionSpec   `json:"spec,omitempty"`
	Status VirtualMachineTemplateVersionStatus `json:"status,omitempty"`
}

type VirtualMachineTemplateSpec struct {
	Description      string `json:"description,omitempty"`
	DefaultVersionID string `json:"defaultVersionId,omitempty"`
}

type VirtualMachineTemplateStatus struct {
	DefaultVersion int `json:"defaultVersion,omitempty"`
	LatestVersion  int `json:"latestVersion,omitempty"`
}

type VirtualMachineTemplateVersionSpec struct {
	Description string                          `json:"description,omitempty"`
	TemplateID  string                          `json:"templateId,omitempty"`
	ImageID     string                          `json:"imageId,omitempty"`
	KeyPairIDs  []string                        `json:"keyPairIds,omitempty"`
	VM          virtv1alpha3.VirtualMachineSpec `json:"vm,omitempty"`
}

type VirtualMachineTemplateVersionStatus struct {
	Version    int         `json:"version,omitempty"`
	Conditions []Condition `json:"conditions"`
}
