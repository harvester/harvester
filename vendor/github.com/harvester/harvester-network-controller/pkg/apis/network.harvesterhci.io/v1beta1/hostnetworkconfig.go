package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=hnc;hncs,scope=Cluster
// +kubebuilder:subresource:status

type HostNetworkConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:validation:XValidation:rule="self.mode != 'static' || (has(self.ips) && size(self.ips) > 0)",message="spec.ips must be specified and non-empty when mode is static"
	Spec HostNetworkConfigSpec `json:"spec"`
	// +optional
	Status HostNetworkConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:validation:MaxLength=50
// +kubebuilder:validation:XValidation:rule="isCIDR(self)",message="Invalid CIDR format"
type IPAddr string

type HostNetworkConfigSpec struct {
	// +optional
	Description string `json:"description,omitempty"`

	// Optional: select target nodes by labels
	// If empty, applies to all nodes
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	// Required, non-empty
	// +kubebuilder:validation:MinLength=1
	ClusterNetwork string `json:"clusterNetwork"`

	// +optional
	Underlay bool `json:"underlay"`

	// Required, must be 1-4094
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4094
	VlanID uint16 `json:"vlanID"`

	// Required, non-empty, only 'static' or 'dhcp'
	// +kubebuilder:validation:Enum=static;dhcp
	// +kubebuilder:validation:MinLength=1
	Mode string `json:"mode"`

	// Optional map, validated if mode=static
	// +optional
	// +kubebuilder:validation:MaxProperties=500
	HostIPs map[string]IPAddr `json:"ips,omitempty"`
}

type HostNetworkConfigStatus struct {
	//global observed state
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
	// Per-node observed state
	// key = node name
	// +optional
	NodeStatus map[string]HostNetworkConfigNodeStatus `json:"nodeStatus,omitempty"`
}

type HostNetworkConfigNodeStatus struct {
	//cluster network
	ClusterNetwork string `json:"clusterNetwork"`

	//vlan id
	VlanID uint16 `json:"vlanID"`

	//mode static or dhcp
	Mode string `json:"mode"`

	// Node-specific conditions
	Conditions []Condition `json:"conditions,omitempty"`
}
