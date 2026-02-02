package v1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type IptablesEIPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []IptablesEIP `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +resourceName=iptables-eips
type IptablesEIP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   IptablesEIPSpec   `json:"spec"`
	Status IptablesEIPStatus `json:"status"`
}
type IptablesEIPSpec struct {
	V4ip           string `json:"v4ip"`
	V6ip           string `json:"v6ip"`
	MacAddress     string `json:"macAddress"`
	NatGwDp        string `json:"natGwDp"`
	QoSPolicy      string `json:"qosPolicy"`
	ExternalSubnet string `json:"externalSubnet"`
}

type IptablesEIPStatus struct {
	// +optional
	// +patchStrategy=merge
	Ready     bool   `json:"ready" patchStrategy:"merge"`
	IP        string `json:"ip" patchStrategy:"merge"`
	Redo      string `json:"redo" patchStrategy:"merge"`
	Nat       string `json:"nat" patchStrategy:"merge"`
	QoSPolicy string `json:"qosPolicy" patchStrategy:"merge"`
	// Conditions represents the latest state of the object
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

func (s *IptablesEIPStatus) Bytes() ([]byte, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	newStr := fmt.Sprintf(`{"status": %s}`, string(bytes))
	klog.V(5).Info("status body", newStr)
	return []byte(newStr), nil
}
