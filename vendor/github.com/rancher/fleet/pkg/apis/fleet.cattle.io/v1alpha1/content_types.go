package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ContentResourceNamePlural = "contents"

func init() {
	InternalSchemeBuilder.Register(&Content{}, &ContentList{})
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// Content is used internally by Fleet and should not be used directly. It
// contains the resources from a bundle for a specific target cluster.
type Content struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Content is a byte array, which contains the manifests of a bundle.
	// The bundle resources are copied into the bundledeployment's content
	// resource, so the downstream agent can deploy them.
	// +nullable
	Content []byte `json:"content,omitempty"`

	// SHA256Sum of the Content field
	SHA256Sum string `json:"sha256sum,omitempty"`
}

// +kubebuilder:object:root=true

// ContentList contains a list of Content
type ContentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Content `json:"items"`
}
