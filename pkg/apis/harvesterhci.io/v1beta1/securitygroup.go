package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=sg,scope=Namespaced

type SecurityGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              []IngressRules `json:"allowIngressSpec,omitempty"`
}

type IngressRules struct {
	SourceAddress   string   `json:"sourceAddress"`
	SourcePortRange []uint32 `json:"ports,omitempty"`
	// +kubebuilder:validation:Enum={"tcp","udp","icmp","icmpv6"}
	IpProtocol string `json:"ipProtocol"`
}

const (
	SecurityGroupPrefix = "harvesterhci.io/attachedSecurityGroup"
)
