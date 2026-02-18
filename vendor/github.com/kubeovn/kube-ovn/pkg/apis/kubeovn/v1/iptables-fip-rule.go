package v1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type IptablesFIPRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []IptablesFIPRule `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +resourceName=iptables-fip-rules
type IptablesFIPRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   IptablesFIPRuleSpec   `json:"spec"`
	Status IptablesFIPRuleStatus `json:"status"`
}
type IptablesFIPRuleSpec struct {
	EIP        string `json:"eip"`
	InternalIP string `json:"internalIp"`
}

type IptablesFIPRuleStatus struct {
	// +optional
	// +patchStrategy=merge
	Ready      bool   `json:"ready" patchStrategy:"merge"`
	V4ip       string `json:"v4ip" patchStrategy:"merge"`
	V6ip       string `json:"v6ip" patchStrategy:"merge"`
	NatGwDp    string `json:"natGwDp" patchStrategy:"merge"`
	Redo       string `json:"redo" patchStrategy:"merge"`
	InternalIP string `json:"internalIp"  patchStrategy:"merge"`

	// Conditions represents the latest state of the object
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

func (s *IptablesFIPRuleStatus) Bytes() ([]byte, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	newStr := fmt.Sprintf(`{"status": %s}`, string(bytes))
	klog.V(5).Info("status body", newStr)
	return []byte(newStr), nil
}
