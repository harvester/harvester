package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AddonState string

const (
	AddonEnabled  AddonState = "AddonEnabled"
	AddonDeployed AddonState = "AddonDeploySuccessful"
	AddonFailed   AddonState = "AddonDeployFailed"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:printcolumn:name="HelmRepo",type=string,JSONPath=`.spec.repo`
// +kubebuilder:printcolumn:name="ChartName",type=string,JSONPath=`.spec.chart`
// +kubebuilder:printcolumn:name="Enabled",type=boolean,JSONPath=`.spec.enabled`
// +kubebuilder:subresource:status

type Addon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AddonSpec   `json:"spec"`
	Status            AddonStatus `json:"status,omitempty"`
}

type AddonSpec struct {
	Repo          string `json:"repo"`
	Chart         string `json:"chart"`
	Version       string `json:"version"`
	Enabled       bool   `json:"enabled"`
	ValuesContent string `json:"valuesContent"`
}

type AddonStatus struct {
	Status AddonState `json:"status,omitempty"`
}
