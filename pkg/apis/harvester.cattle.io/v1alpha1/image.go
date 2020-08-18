package v1alpha1

import (
	"github.com/rancher/wrangler/pkg/condition"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ImageImported condition.Cond = "imported"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VirtualMachineImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineImageSpec   `json:"spec"`
	Status VirtualMachineImageStatus `json:"status"`
}

type VirtualMachineImageSpec struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	URL         string `json:"url"`
	SecretRef   string `json:"secretRef"`
}

type VirtualMachineImageStatus struct {
	AppliedURL  string      `json:"appliedUrl"`
	DownloadURL string      `json:"downloadUrl"`
	Progress    int         `json:"progress"`
	Conditions  []Condition `json:"conditions"`
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
