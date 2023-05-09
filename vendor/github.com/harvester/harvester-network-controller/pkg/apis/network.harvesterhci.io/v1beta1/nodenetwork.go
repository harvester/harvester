package v1beta1

import (
	"github.com/rancher/wrangler/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeNetwork is deprecated from Harvester v1.1.0 and only keep it for upgrade compatibility purpose

const GroupName = "network.harvesterhci.io"

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=nn;nns,scope=Cluster
// +kubebuilder:deprecatedversion
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,JSONPath=`.spec.description`
// +kubebuilder:printcolumn:name="NODENAME",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="TYPE",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="NetworkInterface",type=string,JSONPath=`.spec.nic`

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
	NetworkInterface string `json:"nic,omitempty"`
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
	NetworkInterfaces []NetworkInterface `json:"nics,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

type NetworkInterface struct {
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
	// Specify whether used by VLAN network or not
	UsedByVlanNetwork bool `json:"usedByVlanNetwork,omitempty"`
}

type NetworkID int

type NodeNetworkLinkStatus struct {
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

var (
	NodeNetworkReady condition.Cond = "Ready"
)
