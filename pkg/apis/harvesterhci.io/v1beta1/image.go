package v1beta1

import (
	"github.com/rancher/wrangler/pkg/condition"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ImageInitialized condition.Cond = "Initialized"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=vmimage;vmimages,scope=Namespaced
// +kubebuilder:printcolumn:name="DISPLAY_NAME",type=string,priority=8,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,priority=10,JSONPath=`.spec.description`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="SIZE",type=integer,JSONPath=`.status.size`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`

type VirtualMachineImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineImageSpec   `json:"spec,omitempty"`
	Status VirtualMachineImageStatus `json:"status,omitempty"`
}

type VirtualMachineImageSpec struct {
	// +optional
	URL string `json:"url"`

	// +kubebuilder:validation:Required
	DisplayName string `json:"displayName,omitempty"`

	// +optional
	Description string `json:"description,omitempty"`

	// +optional
	SecretRef string `json:"secretRef,omitempty"`
}

type VirtualMachineImageStatus struct {
	// +optional
	AppliedURL string `json:"appliedUrl,omitempty"`

	// +optional
	Size int64 `json:"size,omitempty"`

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
