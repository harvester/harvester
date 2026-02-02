package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VpcDnsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []VpcDns `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +resourceName=vpc-dnses
type VpcDns struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   VpcDNSSpec   `json:"spec"`
	Status VpcDNSStatus `json:"status"`
}

type VpcDNSSpec struct {
	Replicas int32  `json:"replicas"`
	Vpc      string `json:"vpc"`
	Subnet   string `json:"subnet"`
}

type VpcDNSStatus struct {
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	Active bool `json:"active" patchStrategy:"merge"`
}
