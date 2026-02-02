package v1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OvnDnatRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []OvnDnatRule `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +resourceName=ovn-dnat-rules
type OvnDnatRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   OvnDnatRuleSpec   `json:"spec"`
	Status OvnDnatRuleStatus `json:"status"`
}

type OvnDnatRuleSpec struct {
	OvnEip       string `json:"ovnEip"`
	IPType       string `json:"ipType"` // vip, ip
	IPName       string `json:"ipName"` // vip, ip crd name
	InternalPort string `json:"internalPort"`
	ExternalPort string `json:"externalPort"`
	Protocol     string `json:"protocol,omitempty"`
	Vpc          string `json:"vpc"`
	V4Ip         string `json:"v4Ip"`
	V6Ip         string `json:"v6Ip"`
}

type OvnDnatRuleStatus struct {
	// +optional
	// +patchStrategy=merge
	Vpc          string `json:"vpc" patchStrategy:"merge"`
	V4Eip        string `json:"v4Eip" patchStrategy:"merge"`
	V6Eip        string `json:"v6Eip" patchStrategy:"merge"`
	ExternalPort string `json:"externalPort"`
	V4Ip         string `json:"v4Ip" patchStrategy:"merge"`
	V6Ip         string `json:"v6Ip" patchStrategy:"merge"`
	InternalPort string `json:"internalPort"`
	Protocol     string `json:"protocol,omitempty"`
	IPName       string `json:"ipName"`
	Ready        bool   `json:"ready" patchStrategy:"merge"`

	// Conditions represents the latest state of the object
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

func (s *OvnDnatRuleStatus) Bytes() ([]byte, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	newStr := fmt.Sprintf(`{"status": %s}`, string(bytes))
	klog.V(5).Info("status body", newStr)
	return []byte(newStr), nil
}
