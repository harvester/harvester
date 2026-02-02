package v1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type IptablesSnatRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []IptablesSnatRule `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +resourceName=iptables-snat-rules
type IptablesSnatRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   IptablesSnatRuleSpec   `json:"spec"`
	Status IptablesSnatRuleStatus `json:"status"`
}

type IptablesSnatRuleSpec struct {
	EIP          string `json:"eip"`
	InternalCIDR string `json:"internalCIDR"`
}

type IptablesSnatRuleStatus struct {
	// +optional
	// +patchStrategy=merge
	Ready        bool   `json:"ready" patchStrategy:"merge"`
	V4ip         string `json:"v4ip" patchStrategy:"merge"`
	V6ip         string `json:"v6ip" patchStrategy:"merge"`
	NatGwDp      string `json:"natGwDp" patchStrategy:"merge"`
	Redo         string `json:"redo" patchStrategy:"merge"`
	InternalCIDR string `json:"internalCIDR" patchStrategy:"merge"`

	// Conditions represents the latest state of the object
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

func (s *IptablesSnatRuleStatus) Bytes() ([]byte, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	newStr := fmt.Sprintf(`{"status": %s}`, string(bytes))
	klog.V(5).Info("status body", newStr)
	return []byte(newStr), nil
}
