package v1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OvnFipList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []OvnFip `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +resourceName=ovn-fips
type OvnFip struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   OvnFipSpec   `json:"spec"`
	Status OvnFipStatus `json:"status"`
}
type OvnFipSpec struct {
	OvnEip string `json:"ovnEip"`
	IPType string `json:"ipType"` // vip, ip
	IPName string `json:"ipName"` // vip, ip crd name
	Vpc    string `json:"vpc"`
	V4Ip   string `json:"v4Ip"`
	V6Ip   string `json:"v6Ip"`
	Type   string `json:"type"` // distributed, centralized
}

type OvnFipStatus struct {
	// +optional
	// +patchStrategy=merge
	Vpc   string `json:"vpc" patchStrategy:"merge"`
	V4Eip string `json:"v4Eip" patchStrategy:"merge"`
	V6Eip string `json:"v6Eip" patchStrategy:"merge"`
	V4Ip  string `json:"v4Ip" patchStrategy:"merge"`
	V6Ip  string `json:"v6Ip" patchStrategy:"merge"`
	Ready bool   `json:"ready" patchStrategy:"merge"`

	// Conditions represents the latest state of the object
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

func (s *OvnFipStatus) Bytes() ([]byte, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	newStr := fmt.Sprintf(`{"status": %s}`, string(bytes))
	klog.V(5).Info("status body", newStr)
	return []byte(newStr), nil
}
