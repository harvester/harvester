package v1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/kubeovn/kube-ovn/pkg/ovsdb/ovnnb"
)

type SgRemoteType string

const (
	SgRemoteTypeAddress SgRemoteType = "address"
	SgRemoteTypeSg      SgRemoteType = "securityGroup"
)

type SgProtocol string

const (
	SgProtocolALL  SgProtocol = "all"
	SgProtocolICMP SgProtocol = "icmp"
	SgProtocolTCP  SgProtocol = "tcp"
	SgProtocolUDP  SgProtocol = "udp"
)

type SgPolicy string

var (
	SgPolicyAllow = SgPolicy(ovnnb.ACLActionAllow)
	SgPolicyDrop  = SgPolicy(ovnnb.ACLActionDrop)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type SecurityGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []SecurityGroup `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +resourceName=security-groups
type SecurityGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   SecurityGroupSpec   `json:"spec"`
	Status SecurityGroupStatus `json:"status"`
}

type SecurityGroupSpec struct {
	IngressRules          []SecurityGroupRule `json:"ingressRules,omitempty"`
	EgressRules           []SecurityGroupRule `json:"egressRules,omitempty"`
	AllowSameGroupTraffic bool                `json:"allowSameGroupTraffic,omitempty"`
}

type SecurityGroupRule struct {
	IPVersion           string       `json:"ipVersion"`
	Protocol            SgProtocol   `json:"protocol,omitempty"`
	Priority            int          `json:"priority,omitempty"`
	RemoteType          SgRemoteType `json:"remoteType"`
	RemoteAddress       string       `json:"remoteAddress,omitempty"`
	RemoteSecurityGroup string       `json:"remoteSecurityGroup,omitempty"`
	PortRangeMin        int          `json:"portRangeMin,omitempty"`
	PortRangeMax        int          `json:"portRangeMax,omitempty"`
	Policy              SgPolicy     `json:"policy"`
}

type SecurityGroupStatus struct {
	PortGroup              string `json:"portGroup"`
	AllowSameGroupTraffic  bool   `json:"allowSameGroupTraffic"`
	IngressMd5             string `json:"ingressMd5"`
	EgressMd5              string `json:"egressMd5"`
	IngressLastSyncSuccess bool   `json:"ingressLastSyncSuccess"`
	EgressLastSyncSuccess  bool   `json:"egressLastSyncSuccess"`
}

func (s *SecurityGroupStatus) Bytes() ([]byte, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	newStr := fmt.Sprintf(`{"status": %s}`, string(bytes))
	klog.V(5).Info("status body", newStr)
	return []byte(newStr), nil
}
