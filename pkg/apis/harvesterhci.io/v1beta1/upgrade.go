package v1beta1

import (
	"github.com/rancher/wrangler/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	UpgradeCompleted condition.Cond = "completed"
	// NodesUpgraded is true when all nodes are upgraded
	NodesUpgraded condition.Cond = "nodesUpgraded"
	// SystemServicesUpgraded is true when Harvester chart is upgraded
	SystemServicesUpgraded condition.Cond = "systemServicesUpgraded"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Namespaced

type Upgrade struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UpgradeSpec   `json:"spec,omitempty"`
	Status UpgradeStatus `json:"status,omitempty"`
}

type UpgradeSpec struct {
	// +kubebuilder:validation:Required
	Version string `json:"version,omitempty"`
}

type UpgradeStatus struct {
	// +optional
	PreviousVersion string `json:"previousVersion,omitempty"`
	// +optional
	NodeStatuses map[string]NodeUpgradeStatus `json:"nodeStatuses,omitempty"`
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

type NodeUpgradeStatus struct {
	State   string `json:"state,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}
