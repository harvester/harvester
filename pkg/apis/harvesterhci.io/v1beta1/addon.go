package v1beta1

import (
	"github.com/rancher/wrangler/v3/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AddonState string

const (
	AddonEnabling  AddonState = "AddonEnabling"
	AddonDeployed  AddonState = "AddonDeploySuccessful"
	AddonDisabled  AddonState = "AddonDisabled"
	AddonDisabling AddonState = "AddonDisabling"

	// after successfully updating, addon will be AddonInitState when !Spec.Enabled; AddonDeployed when Spec.Enabled
	AddonUpdating AddonState = "AddonUpdating"

	AddonInitState AddonState = "" // init status, when an addon is not enabled, or disabled successfully
)

type AddonOperation string

var (
	AddonOperationInProgress condition.Cond = "InProgress"
	AddonOperationCompleted  condition.Cond = "Completed"
	AddonOperationFailed     condition.Cond = "OperationFailed"
	DefaultJobBackOffLimit                  = int32(5)
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
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}
