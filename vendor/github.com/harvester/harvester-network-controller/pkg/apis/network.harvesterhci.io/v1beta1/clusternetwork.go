package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=cn;cns,scope=Cluster

type ClusterNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +optional
	Status ClusterNetworkStatus `json:"status"`
}

type ClusterNetworkStatus struct {
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}
