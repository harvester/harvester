package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	InternalSchemeBuilder.Register(&BundleNamespaceMapping{}, &BundleNamespaceMappingList{})
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// BundleNamespaceMapping maps bundles to clusters in other namespaces.
type BundleNamespaceMapping struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +nullable
	BundleSelector *metav1.LabelSelector `json:"bundleSelector,omitempty"`
	// +nullable
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

// +kubebuilder:object:root=true

// BundleNamespaceMappingList contains a list of BundleNamespaceMapping
type BundleNamespaceMappingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BundleNamespaceMapping `json:"items"`
}
