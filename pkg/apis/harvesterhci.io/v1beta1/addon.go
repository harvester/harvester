package v1beta1

import (
	"github.com/rancher/wrangler/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AddonState string

const (
	AddonEnabled       AddonState = "AddonEnabled"
	AddonDeployed      AddonState = "AddonDeploySuccessful"
	AddonFailed        AddonState = "AddonDeployFailed"
	AddonDisabling     AddonState = "AddonDisabling"
	AddonDisableFailed AddonState = "AddonDisableFailed"

	AddonUpdating    AddonState = "AddonUpdating"
	AddonUpdateFaild AddonState = "AddonUpdateFailed"

	AddonInitState AddonState = "" // init status, when an addon is not enabled, or disabled successfully
)

type AddonOperation string

const (
	AddonUpdateOperation  AddonOperation = "update"
	AddonEnableOperation  AddonOperation = "enable"
	AddonDisableOperation AddonOperation = "disable"
	AddonNullOperation    AddonOperation = "null"
)

var (
	AddonUpdateCondition  condition.Cond = "Update"
	AddonEnableCondition  condition.Cond = "Enable"
	AddonDisableCondition condition.Cond = "Disable"
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
