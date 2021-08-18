package v1beta1

import (
	"github.com/rancher/wrangler/pkg/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=nn;nns,scope=Cluster
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,JSONPath=`.spec.description`
// +kubebuilder:printcolumn:name="NODENAME",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="TYPE",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="NIC",type=string,JSONPath=`.spec.nic`

type NodeNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeNetworkSpec   `json:"spec,omitempty"`
	Status NodeNetworkStatus `json:"status,omitempty"`
}

type NodeNetworkSpec struct {
	// +optional
	Description string `json:"description,omitempty"`

	NodeName string `json:"nodeName"`

	// +kubebuilder:validation:Required
	Type NetworkType `json:"type,omitempty"`

	// +optional
	NIC string `json:"nic,omitempty"`
}

// +kubebuilder:validation:Enum=vlan
type NetworkType string

const (
	NetworkTypeVLAN NetworkType = "vlan"
)

type NodeNetworkStatus struct {
	// +optional
	NetworkIDs []NetworkID `json:"networkIDs,omitempty"`

	// +optional
	NetworkLinkStatus map[string]*LinkStatus `json:"networkLinkStatus,omitempty"`

	// +optional
	NICs []NIC `json:"nics,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

type NIC struct {
	// Index of the NIC
	Index int `json:"index"`
	// Index of the NIC's master
	MasterIndex int `json:"masterIndex,omitempty"`
	// Name of the NIC
	Name string `json:"name"`
	// Interface type of the NIC
	Type string `json:"type"`
	// State of the NIC, up/down/unknown
	State string `json:"state"`
	// Specify whether used by management network or not
	UsedByMgmtNetwork bool `json:"usedByManagementNetwork,omitempty"`
}

type NetworkID int

type LinkStatus struct {
	// +optional
	Index int `json:"index,omitempty"`

	// +optional
	Type string `json:"type,omitempty"`

	// +optional
	MAC string `json:"mac,omitempty"`

	// +optional
	Promiscuous bool `json:"promiscuous,omitempty"`

	// +optional
	State string `json:"state,omitempty"`

	// +optional
	IPV4Address []string `json:"ipv4Address,omitempty"`

	// +optional
	MasterIndex int `json:"masterIndex,omitempty"`

	// +optional
	Routes []string `json:"routes,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

type Condition struct {
	// Type of the condition.
	Type condition.Cond `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	LastUpdateTime string `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// Human-readable message indicating details about last transition
	Message string `json:"message,omitempty"`
}

var (
	NodeNetworkReady condition.Cond = "Ready"
)
