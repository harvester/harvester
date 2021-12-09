package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="ISO-URL",type=string,JSONPath=`.spec.isoURL`
// +kubebuilder:printcolumn:name="ReleaseDate",type=string,JSONPath=`.spec.releaseDate`
// +kubebuilder:printcolumn:name="MinUpgradableVersion",type="string",JSONPath=`.spec.minUpgradableVersion`

type Version struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VersionSpec `json:"spec"`
}

type VersionSpec struct {
	// +kubebuilder:validation:Required
	ISOURL string `json:"isoURL"`

	// +optional
	ISOChecksum string `json:"isoChecksum"`

	// +optional
	ReleaseDate string `json:"releaseDate"`

	// +optional
	MinUpgradableVersion string `json:"minUpgradableVersion,omitempty"`

	// +optional
	Tags []string `json:"tags"`
}
