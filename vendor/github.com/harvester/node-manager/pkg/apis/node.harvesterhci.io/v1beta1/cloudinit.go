package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=nci,scope=Cluster
// +kubebuilder:subresource:status

type CloudInit struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CloudInitSpec   `json:"spec"`
	Status            CloudInitStatus `json:"status,omitempty"`
}

type CloudInitSpec struct {
	MatchSelector map[string]string `json:"matchSelector"`
	Filename      string            `json:"filename"`
	Contents      string            `json:"contents"`
	Paused        bool              `json:"paused,omitempty"`
}

type CloudInitStatus struct {
	Rollouts map[string]Rollout `json:"rollouts,omitempty"`
}

type Rollout struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type ConditionTypeCloudInit string

const (
	CloudInitFilePresent ConditionTypeCloudInit = "Present"
	CloudInitApplicable  ConditionTypeCloudInit = "Applicable"
	CloudInitOutOfSync   ConditionTypeCloudInit = "OutOfSync"
)
