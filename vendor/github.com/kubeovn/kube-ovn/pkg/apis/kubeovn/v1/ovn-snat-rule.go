package v1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OvnSnatRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []OvnSnatRule `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +resourceName=ovn-snat-rules
type OvnSnatRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   OvnSnatRuleSpec   `json:"spec"`
	Status OvnSnatRuleStatus `json:"status"`
}

type OvnSnatRuleSpec struct {
	OvnEip    string `json:"ovnEip"`
	VpcSubnet string `json:"vpcSubnet"`
	IPName    string `json:"ipName"`
	Vpc       string `json:"vpc"`
	V4IpCidr  string `json:"v4IpCidr"` // subnet cidr or pod ip address
	V6IpCidr  string `json:"v6IpCidr"` // subnet cidr or pod ip address
}

type OvnSnatRuleStatus struct {
	// +optional
	// +patchStrategy=merge
	Vpc      string `json:"vpc" patchStrategy:"merge"`
	V4Eip    string `json:"v4Eip" patchStrategy:"merge"`
	V6Eip    string `json:"v6Eip" patchStrategy:"merge"`
	V4IpCidr string `json:"v4IpCidr" patchStrategy:"merge"`
	V6IpCidr string `json:"v6IpCidr" patchStrategy:"merge"`
	Ready    bool   `json:"ready" patchStrategy:"merge"`

	// Conditions represents the latest state of the object
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

func (s *OvnSnatRuleStatus) Bytes() ([]byte, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	newStr := fmt.Sprintf(`{"status": %s}`, string(bytes))
	klog.V(5).Info("status body", newStr)
	return []byte(newStr), nil
}
