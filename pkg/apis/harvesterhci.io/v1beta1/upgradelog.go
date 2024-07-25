package v1beta1

import (
	"github.com/rancher/wrangler/v3/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	LoggingOperatorDeployed condition.Cond = "LoggingOperatorReady"
	InfraReady              condition.Cond = "InfraReady"
	UpgradeLogReady         condition.Cond = "Started"
	UpgradeEnded            condition.Cond = "Stopped"
	DownloadReady           condition.Cond = "DownloadReady"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="UPGRADE",type="string",JSONPath=`.spec.upgradeName`

type UpgradeLog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UpgradeLogSpec   `json:"spec"`
	Status UpgradeLogStatus `json:"status,omitempty"`
}

type UpgradeLogSpec struct {
	// +kubebuilder:validation:Required
	UpgradeName string `json:"upgradeName,omitempty"`
}

type UpgradeLogStatus struct {
	// +optional
	Archives map[string]Archive `json:"archives,omitempty"`
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

type Archive struct {
	// +optional
	Size int64 `json:"size,omitempty"`
	// +optional
	GeneratedTime string `json:"generatedTime,omitempty"`
	// +optional
	Ready bool `json:"ready"`
	// +optional
	Reason string `json:"reason,omitempty"`
}
