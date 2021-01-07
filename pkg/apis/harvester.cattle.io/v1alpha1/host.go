package v1alpha1

import (
	"github.com/rancher/wrangler/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=host;hosts,scope=Cluster
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,JSONPath=`.description`
// +kubebuilder:printcolumn:name="NETWORK_TYPE",type=string,JSONPath=`.spec.network.type`
// +kubebuilder:printcolumn:name="NETWORK_NIC",type=string,JSONPath=`.spec.network.nic`

type Host struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostSpec   `json:"spec,omitempty"`
	Status HostStatus `json:"status,omitempty"`
}

type HostSpec struct {
	// +optional
	Description string `json:"description,omitempty"`

	// +optional
	Network HostNetwork `json:"network,omitempty"`
}

type HostNetwork struct {
	Type NetworkType `json:"type,omitempty"`
	NIC  string      `json:"nic,omitempty"`
}

// +kubebuilder:validation:Enum=vlan
type NetworkType string

const (
	NetworkTypeVLAN NetworkType = "vlan"
)

type HostStatus struct {
	// +optional
	NetworkIDs []NetworkID `json:"networkIDs,omitempty"`

	// +optional
	NetworkStatus map[string]*LinkStatus `json:"networkStatus,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
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
	State LinkState `json:"state,omitempty"`

	// +optional
	IPV4Address string `json:"ipv4Address,omitempty"`

	// +optional
	Master string `json:"master,omitempty"`

	// +optional
	Routes []string `json:"routes,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

type LinkState string

const (
	LinkStateUp   LinkState = "Up"
	LinkStateDown LinkState = "Down"
)

var (
	HostReady                  condition.Cond = "Ready"
	HostNetworkReady           condition.Cond = "NetworkReady"
	HostNetworkIDConfigured    condition.Cond = "NetworkIDConfigured"
	HostNetworkLinkReady       condition.Cond = "LinkReady"
	HostNetworkRouteConfigured condition.Cond = "RouteConfigured"
)
