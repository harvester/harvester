package v1

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VpcNatGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []VpcNatGateway `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +resourceName=vpc-nat-gateways
type VpcNatGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   VpcNatGatewaySpec   `json:"spec"`
	Status VpcNatGatewayStatus `json:"status"`
}

type VpcNatGatewaySpec struct {
	Vpc             string              `json:"vpc"`
	Subnet          string              `json:"subnet"`
	ExternalSubnets []string            `json:"externalSubnets"`
	LanIP           string              `json:"lanIp"`
	Selector        []string            `json:"selector"`
	Tolerations     []corev1.Toleration `json:"tolerations"`
	Affinity        corev1.Affinity     `json:"affinity"`
	QoSPolicy       string              `json:"qosPolicy"`
	BgpSpeaker      VpcBgpSpeaker       `json:"bgpSpeaker"`
}

type VpcBgpSpeaker struct {
	Enabled               bool            `json:"enabled"`
	ASN                   uint32          `json:"asn"`
	RemoteASN             uint32          `json:"remoteAsn"`
	Neighbors             []string        `json:"neighbors"`
	HoldTime              metav1.Duration `json:"holdTime"`
	RouterID              string          `json:"routerId"`
	Password              string          `json:"password"`
	EnableGracefulRestart bool            `json:"enableGracefulRestart"`
	ExtraArgs             []string        `json:"extraArgs"`
}

type VpcNatGatewayStatus struct {
	QoSPolicy       string              `json:"qosPolicy" patchStrategy:"merge"`
	ExternalSubnets []string            `json:"externalSubnets" patchStrategy:"merge"`
	Selector        []string            `json:"selector" patchStrategy:"merge"`
	Tolerations     []corev1.Toleration `json:"tolerations" patchStrategy:"merge"`
	Affinity        corev1.Affinity     `json:"affinity" patchStrategy:"merge"`
}

func (s *VpcNatGatewayStatus) Bytes() ([]byte, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	newStr := fmt.Sprintf(`{"status": %s}`, string(bytes))
	klog.V(5).Info("status body", newStr)
	return []byte(newStr), nil
}
