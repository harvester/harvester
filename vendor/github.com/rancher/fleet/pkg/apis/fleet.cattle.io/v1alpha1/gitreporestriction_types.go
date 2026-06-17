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
	// be set. A target namespace is allowed if it is listed here or if it
	// matches AllowedTargetNamespaceSelector (OR semantics).
	// +nullable
	AllowedTargetNamespaces []string `json:"allowedTargetNamespaces,omitempty"`

	// AllowedTargetNamespaceSelector is a label selector to match labels defined on the downstream
	// cluster's namespaces. When labels defined on the namespace resources match the selector labels,
	// the namespace on the downstream cluster will be a target namespace that bundles can be deployed to.
	// A namespace is allowed if it either matches this selector or is listed in AllowedTargetNamespaces (OR semantics).
	// +nullable
	AllowedTargetNamespaceSelector *metav1.LabelSelector `json:"allowedTargetNamespaceSelector,omitempty"`
}

// +kubebuilder:object:root=true

// GitRepoRestrictionList contains a list of GitRepoRestriction
type GitRepoRestrictionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitRepoRestriction `json:"items"`
}
