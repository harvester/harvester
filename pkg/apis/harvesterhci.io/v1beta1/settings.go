package v1beta1

import (
	"github.com/rancher/wrangler/v3/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	SettingConfigured condition.Cond = "configured"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="VALUE",type="string",JSONPath=`.value`

type Setting struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Value string `json:"value,omitempty"`

	// +optional
	Default string `json:"default,omitempty"`

	// +optional
	Customized bool `json:"customized,omitempty"`

	// +optional
	Source string `json:"source,omitempty"`

	Status SettingStatus `json:"status,omitempty"`
}

type SettingStatus struct {
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}
