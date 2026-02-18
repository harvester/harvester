package v1

import (
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

type QoSPolicyBindingType string

const (
	QoSBindingTypeEIP   QoSPolicyBindingType = "EIP"
	QoSBindingTypeNatGw QoSPolicyBindingType = "NATGW"
)

type QoSPolicyRuleDirection string

const (
	QoSDirectionIngress QoSPolicyRuleDirection = "ingress"
	QoSDirectionEgress  QoSPolicyRuleDirection = "egress"
)

type QoSPolicyRuleMatchType string

const QoSMatchTypeIP QoSPolicyRuleMatchType = "ip"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type QoSPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []QoSPolicy `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +resourceName=qos-policies
type QoSPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   QoSPolicySpec   `json:"spec"`
	Status QoSPolicyStatus `json:"status"`
}
type QoSPolicySpec struct {
	BandwidthLimitRules QoSPolicyBandwidthLimitRules `json:"bandwidthLimitRules"`
	Shared              bool                         `json:"shared"`
	BindingType         QoSPolicyBindingType         `json:"bindingType"`
}

// BandwidthLimitRule describes the rule of an bandwidth limit.
type QoSPolicyBandwidthLimitRule struct {
	Name       string                 `json:"name"`
	Interface  string                 `json:"interface,omitempty"`
	RateMax    string                 `json:"rateMax,omitempty"`
	BurstMax   string                 `json:"burstMax,omitempty"`
	Priority   int                    `json:"priority,omitempty"`
	Direction  QoSPolicyRuleDirection `json:"direction,omitempty"`
	MatchType  QoSPolicyRuleMatchType `json:"matchType,omitempty"`
	MatchValue string                 `json:"matchValue,omitempty"`
}

type QoSPolicyBandwidthLimitRules []QoSPolicyBandwidthLimitRule

func (s QoSPolicyBandwidthLimitRules) Strings() string {
	var resultNames []string
	for _, rule := range s {
		resultNames = append(resultNames, rule.Name)
	}
	return strings.Join(resultNames, ",")
}

type QoSPolicyStatus struct {
	BandwidthLimitRules QoSPolicyBandwidthLimitRules `json:"bandwidthLimitRules" patchStrategy:"merge"`
	Shared              bool                         `json:"shared" patchStrategy:"merge"`
	BindingType         QoSPolicyBindingType         `json:"bindingType"`

	// Conditions represents the latest state of the object
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

func (s *QoSPolicyStatus) Bytes() ([]byte, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	newStr := fmt.Sprintf(`{"status": %s}`, string(bytes))
	klog.V(5).Info("status body", newStr)
	return []byte(newStr), nil
}
