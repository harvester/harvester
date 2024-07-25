package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ClusterRegistrationResourceNamePlural = "clusterregistrations"

func init() {
	InternalSchemeBuilder.Register(&ClusterRegistration{}, &ClusterRegistrationList{})
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster-Name",type=string,JSONPath=`.status.clusterName`
// +kubebuilder:printcolumn:name="Labels",type=string,JSONPath=`.spec.clusterLabels`

// ClusterRegistration is used internally by Fleet and should not be used directly.
type ClusterRegistration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterRegistrationSpec   `json:"spec,omitempty"`
	Status ClusterRegistrationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterRegistrationList contains a list of ClusterRegistration
type ClusterRegistrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterRegistration `json:"items"`
}

type ClusterRegistrationSpec struct {
	// ClientID is a unique string that will identify the cluster. The
	// agent either uses the configured ID or the kubeSystem.UID.
	// +nullable
	ClientID string `json:"clientID,omitempty"`
	// ClientRandom is a random string that the agent generates. When
	// fleet-controller grants a registration, it creates a registration
	// secret with this string in the name.
	// +nullable
	ClientRandom string `json:"clientRandom,omitempty"`
	// ClusterLabels are copied to the cluster resource during the registration.
	// +nullable
	ClusterLabels map[string]string `json:"clusterLabels,omitempty"`
}

type ClusterRegistrationStatus struct {
	// ClusterName is only set after the registration is being processed by
	// fleet-controller.
	// +nullable
	ClusterName string `json:"clusterName,omitempty"`
	// Granted is set to true, if the request service account is present
	// and its token secret exists. This happens directly before creating
	// the registration secret, roles and rolebindings.
	Granted bool `json:"granted,omitempty"`
}
