package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:validation:Enum=up;down
type LinkState string

const (
	LinkUp   LinkState = "up"
	LinkDown LinkState = "down"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lm;lms,scope=Cluster

type LinkMonitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              LinkMonitorSpec `json:"spec"`
	// +optional
	Status LinkMonitorStatus `json:"status"`
}

type LinkMonitorSpec struct {
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// +optional
	TargetLinkRule TargetLinkRule `json:"targetLinkRule,omitempty"`
}

type LinkMonitorStatus struct {
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
	// +optional
	LinkStatus map[string][]LinkStatus `json:"linkStatus,omitempty"`
}

type TargetLinkRule struct {
	// +optional
	// Support regular expression and empty value means matching all
	TypeRule string `json:"typeRule,omitempty"`
	// +optional
	// Support regular expression and empty value means matching all
	NameRule string `json:"nameRule,omitempty"`
}

type LinkStatus struct {
	Name string `json:"name"`
	// +optional
	Index int `json:"index,omitempty"`
	// +optional
	Type string `json:"type,omitempty"`
	// +optional
	MAC string `json:"mac,omitempty"`
	// +optional
	Promiscuous bool `json:"promiscuous,omitempty"`
	// +optional
	State LinkState `json:"state,omitempty"`
	// +optional
	MasterIndex int `json:"masterIndex,omitempty"`
}
