package v1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VipList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Vip `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
type Vip struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   VipSpec   `json:"spec"`
	Status VipStatus `json:"status"`
}

type VipSpec struct {
	Namespace string `json:"namespace"`
	Subnet    string `json:"subnet"`
	Type      string `json:"type"`
	// usage type: switch lb vip, allowed address pair vip by default
	V4ip          string   `json:"v4ip"`
	V6ip          string   `json:"v6ip"`
	MacAddress    string   `json:"macAddress"`
	Selector      []string `json:"selector"`
	AttachSubnets []string `json:"attachSubnets"`
}

type VipStatus struct {
	// Conditions represents the latest state of the object
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	Type string `json:"type"`
	V4ip string `json:"v4ip" patchStrategy:"merge"`
	V6ip string `json:"v6ip" patchStrategy:"merge"`
	Mac  string `json:"mac" patchStrategy:"merge"`
}

func (s *VipStatus) Bytes() ([]byte, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	newStr := fmt.Sprintf(`{"status": %s}`, string(bytes))
	klog.V(5).Info("status body", newStr)
	return []byte(newStr), nil
}
