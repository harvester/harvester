package v1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type IptablesDnatRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []IptablesDnatRule `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +resourceName=iptables-dnat-rules
type IptablesDnatRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   IptablesDnatRuleSpec   `json:"spec"`
	Status IptablesDnatRuleStatus `json:"status"`
}

type IptablesDnatRuleSpec struct {
	EIP          string `json:"eip"`
	ExternalPort string `json:"externalPort"`
	Protocol     string `json:"protocol,omitempty"`
	InternalIP   string `json:"internalIp"`
	InternalPort string `json:"internalPort"`
}

type IptablesDnatRuleStatus struct {
	// +optional
	// +patchStrategy=merge
	Ready        bool   `json:"ready" patchStrategy:"merge"`
	V4ip         string `json:"v4ip" patchStrategy:"merge"`
	V6ip         string `json:"v6ip" patchStrategy:"merge"`
	NatGwDp      string `json:"natGwDp" patchStrategy:"merge"`
	Redo         string `json:"redo" patchStrategy:"merge"`
	Protocol     string `json:"protocol"  patchStrategy:"merge"`
	InternalIP   string `json:"internalIp"  patchStrategy:"merge"`
	InternalPort string `json:"internalPort"  patchStrategy:"merge"`
	ExternalPort string `json:"externalPort"  patchStrategy:"merge"`

	// Conditions represents the latest state of the object
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

func (s *IptablesDnatRuleStatus) Bytes() ([]byte, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	newStr := fmt.Sprintf(`{"status": %s}`, string(bytes))
	klog.V(5).Info("status body", newStr)
	return []byte(newStr), nil
}
