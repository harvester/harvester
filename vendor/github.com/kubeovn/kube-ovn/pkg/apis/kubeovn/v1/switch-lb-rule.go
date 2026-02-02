package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type SwitchLBRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []SwitchLBRule `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced
// +resourceName=switch-lb-rules
type SwitchLBRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   SwitchLBRuleSpec   `json:"spec"`
	Status SwitchLBRuleStatus `json:"status"`
}

type SwitchLBRuleSpec struct {
	Vip             string             `json:"vip"`
	Namespace       string             `json:"namespace"`
	Selector        []string           `json:"selector"`
	Endpoints       []string           `json:"endpoints"`
	SessionAffinity string             `json:"sessionAffinity,omitempty"`
	Ports           []SwitchLBRulePort `json:"ports"`
}

type SwitchLBRulePort struct {
	Name       string `json:"name"`
	Port       int32  `json:"port"`
	TargetPort int32  `json:"targetPort,omitempty"`
	Protocol   string `json:"protocol"`
}

type SwitchLBRuleStatus struct {
	// Conditions represents the latest state of the object
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	Ports   string `json:"ports" patchStrategy:"merge"`
	Service string `json:"service" patchStrategy:"merge"`
}
