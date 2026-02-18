package v1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OvnEipList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []OvnEip `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +resourceName=ovn-eips
type OvnEip struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   OvnEipSpec   `json:"spec"`
	Status OvnEipStatus `json:"status"`
}
type OvnEipSpec struct {
	ExternalSubnet string `json:"externalSubnet"`
	V4Ip           string `json:"v4Ip"`
	V6Ip           string `json:"v6Ip"`
	MacAddress     string `json:"macAddress"`
	Type           string `json:"type"`
	// usage type: lrp, lsp, nat
	// nat: used by nat: dnat, snat, fip
	// lrp: lrp created by vpc enable external, and also could be used by nat
	// lsp: in the case of bfd session between lrp and lsp, the lsp is on the node as ecmp nexthop
}

type OvnEipStatus struct {
	// Conditions represents the latest state of the object
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	Type       string `json:"type" patchStrategy:"merge"`
	Nat        string `json:"nat" patchStrategy:"merge"`
	Ready      bool   `json:"ready" patchStrategy:"merge"`
	V4Ip       string `json:"v4Ip" patchStrategy:"merge"`
	V6Ip       string `json:"v6Ip" patchStrategy:"merge"`
	MacAddress string `json:"macAddress" patchStrategy:"merge"`
}

func (s *OvnEipStatus) Bytes() ([]byte, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	newStr := fmt.Sprintf(`{"status": %s}`, string(bytes))
	klog.V(5).Info("status body", newStr)
	return []byte(newStr), nil
}
