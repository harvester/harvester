package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	InternalSchemeBuilder.Register(&GitRepoRestriction{}, &GitRepoRestrictionList{})
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Default-ServiceAccount",type=string,JSONPath=`.defaultServiceAccount`
// +kubebuilder:printcolumn:name="Allowed-ServiceAccounts",type=string,JSONPath=`.allowedServiceAccounts`

// GitRepoRestriction is a resource that can optionally be used to restrict
// the options of GitRepos in the same namespace.
type GitRepoRestriction struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// DefaultServiceAccount overrides the GitRepo's default service account.
	// +nullable
	DefaultServiceAccount string `json:"defaultServiceAccount,omitempty"`
	// AllowedServiceAccounts is a list of service accounts that GitRepos are allowed to use.
	// +nullable
	AllowedServiceAccounts []string `json:"allowedServiceAccounts,omitempty"`
	// AllowedRepoPatterns is a list of regex patterns that restrict the
	// valid values of the Repo field of a GitRepo.
	// +nullable
	AllowedRepoPatterns []string `json:"allowedRepoPatterns,omitempty"`

	// DefaultClientSecretName overrides the GitRepo's default client secret.
	// +nullable
	DefaultClientSecretName string `json:"defaultClientSecretName,omitempty"`
	// AllowedClientSecretNames is a list of client secret names that GitRepos are allowed to use.
	// +nullable
	AllowedClientSecretNames []string `json:"allowedClientSecretNames,omitempty"`

	// AllowedTargetNamespaces restricts TargetNamespace to the given
	// namespaces. If AllowedTargetNamespaces is set, TargetNamespace must
	// be set.
	// +nullable
	AllowedTargetNamespaces []string `json:"allowedTargetNamespaces,omitempty"`
}

// +kubebuilder:object:root=true

// GitRepoRestrictionList contains a list of GitRepoRestriction
type GitRepoRestrictionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitRepoRestriction `json:"items"`
}
