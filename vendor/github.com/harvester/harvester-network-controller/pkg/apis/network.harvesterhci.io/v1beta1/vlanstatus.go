package v1beta1

import (
	"github.com/rancher/wrangler/pkg/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=vs;vss,scope=Cluster
// +kubebuilder:printcolumn:name="CLUSTERNETWORK",type=string,JSONPath=`.status.clusterNetwork`
// +kubebuilder:printcolumn:name="VLANCONFIG",type=string,JSONPath=`.status.vlanConfig`
// +kubebuilder:printcolumn:name="NODE",type=string,JSONPath=`.status.node`
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,JSONPath=`.spec.description`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`

type VlanStatus struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status VlStatus `json:"status"`
}

type VlStatus struct {
	ClusterNetwork string `json:"clusterNetwork"`

	VlanConfig string `json:"vlanConfig"`

	LinkMonitor string `json:"linkMonitor"`

	Node string `json:"node"`
	// +optional
	LocalAreas []LocalArea `json:"localAreas,omitempty"`
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

type LocalArea struct {
	VID  uint16 `json:"vlanID"`
	CIDR string `json:"cidr,omitempty"`
}

type Condition struct {
	// Type of the condition.
	Type condition.Cond `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	LastUpdateTime string `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// Human-readable message indicating details about last transition
	Message string `json:"message,omitempty"`
}

var (
	Ready condition.Cond = "ready"
)
