package v1beta1

import (
	"github.com/rancher/wrangler/v3/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	UpgradeCompleted condition.Cond = "Completed"
	// LogReady is true when logging infrastructure for is running
	LogReady condition.Cond = "LogReady"
	// ImageReady is true when upgrade image is downloaded
	ImageReady condition.Cond = "ImageReady"
	// RepoProvisioned is true when upgrade repo is provisioned
	RepoProvisioned condition.Cond = "RepoReady"
	// NodesPrepared is true when all nodes are prepared
	NodesPrepared condition.Cond = "NodesPrepared"
	// NodesUpgraded is true when all nodes are upgraded
	NodesUpgraded condition.Cond = "NodesUpgraded"
	// SystemServicesUpgraded is true when Harvester chart is upgraded
	SystemServicesUpgraded condition.Cond = "SystemServicesUpgraded"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Namespaced

type Upgrade struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UpgradeSpec   `json:"spec"`
	Status UpgradeStatus `json:"status,omitempty"`
}

type UpgradeSpec struct {
	// +optional
	Version string `json:"version"`

	// +optional
	Image string `json:"image"`

	// +optional
	// +kubebuilder:default:=true
	LogEnabled bool `json:"logEnabled" default:"true"`
}

type UpgradeStatus struct {
	// +optional
	PreviousVersion string `json:"previousVersion,omitempty"`
	// +optional
	ImageID string `json:"imageID,omitempty"`
	// +optional
	RepoInfo string `json:"repoInfo,omitempty"`
	// +optional
	SingleNode string `json:"singleNode,omitempty"`
	// +optional
	NodeStatuses map[string]NodeUpgradeStatus `json:"nodeStatuses,omitempty"`
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
	// +optional
	UpgradeLog string `json:"upgradeLog,omitempty"`
}

type NodeUpgradeStatus struct {
	State   string `json:"state,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}
