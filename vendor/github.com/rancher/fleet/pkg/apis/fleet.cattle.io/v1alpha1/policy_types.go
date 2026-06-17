package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func init() {
	InternalSchemeBuilder.Register(&Policy{}, &PolicyList{})
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// Policy restricts what GitRepo, HelmOp, and Bundle resources in the same
// namespace may do. Enforced at three points in the controller stack:
//
//   - GitRepo reconciler: validates and applies defaults before producing a Bundle.
//   - HelmOp reconciler: validates and applies defaults before producing a Bundle.
//   - Bundle reconciler: validates only (fail-only) before producing BundleDeployments.
//
// Top-level fields are checked by all three reconcilers.
// Sub-object fields (gitRepo, helmOp) are only read by their respective reconciler.
// Default* fields inside sub-objects are applied before top-level validators run.
//
// Multiple Policy objects in the same namespace are aggregated with OR/union
// semantics, sorted by name for determinism.
type Policy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// RequireServiceAccount, when true, rejects any GitRepo, HelmOp, or Bundle
	// whose ServiceAccount is empty after any defaulting has been applied.
	// Combine with AllowedServiceAccounts to also restrict which account is used.
	// +optional
	RequireServiceAccount bool `json:"requireServiceAccount,omitempty"`

	// AllowedServiceAccounts lists service accounts that may be used.
	// If non-empty, the ServiceAccount must appear in this list.
	// When RequireServiceAccount is also true, an empty ServiceAccount is
	// rejected regardless of this list.
	// +optional
	// +nullable
	AllowedServiceAccounts []string `json:"allowedServiceAccounts,omitempty"`

	// GitRepo contains restrictions and defaults applied only by the GitRepo reconciler.
	// +optional
	GitRepo *GitRepoPolicySpec `json:"gitRepo,omitempty"`

	// HelmOp contains restrictions and defaults applied only by the HelmOp reconciler.
	// +optional
	HelmOp *HelmOpPolicySpec `json:"helmOp,omitempty"`
}

// GitRepoPolicySpec holds GitRepo-specific defaults and source restrictions.
type GitRepoPolicySpec struct {
	// DefaultServiceAccount is applied to GitRepo objects whose ServiceAccount
	// is empty, before the top-level RequireServiceAccount check runs.
	// +optional
	DefaultServiceAccount string `json:"defaultServiceAccount,omitempty"`

	// DefaultClientSecretName is applied to GitRepo objects whose
	// ClientSecretName is empty.
	// +optional
	DefaultClientSecretName string `json:"defaultClientSecretName,omitempty"`

	// AllowedClientSecretNames lists client secret names that GitRepo objects
	// may reference.
	// +optional
	// +nullable
	AllowedClientSecretNames []string `json:"allowedClientSecretNames,omitempty"`

	// AllowedRepoPatterns is a list of regex patterns restricting the Repo
	// field of GitRepo objects.
	// +optional
	// +nullable
	AllowedRepoPatterns []string `json:"allowedRepoPatterns,omitempty"`
}

// HelmOpPolicySpec holds HelmOp-specific defaults and source restrictions.
type HelmOpPolicySpec struct {
	// DefaultServiceAccount is applied to HelmOp objects whose ServiceAccount
	// is empty, before the top-level RequireServiceAccount check runs.
	// +optional
	DefaultServiceAccount string `json:"defaultServiceAccount,omitempty"`

	// DefaultHelmSecretName is applied to HelmOp objects whose HelmSecretName
	// is empty.
	// +optional
	DefaultHelmSecretName string `json:"defaultHelmSecretName,omitempty"`

	// AllowedHelmSecretNames lists credential secret names that HelmOp objects
	// may reference.
	// +optional
	// +nullable
	AllowedHelmSecretNames []string `json:"allowedHelmSecretNames,omitempty"`

	// AllowedHelmRepoPatterns is a list of regex patterns restricting the
	// spec.helm.repo field of HelmOp objects.
	// +optional
	// +nullable
	AllowedHelmRepoPatterns []string `json:"allowedHelmRepoPatterns,omitempty"`

	// AllowedChartPatterns is a list of regex patterns restricting the
	// spec.helm.chart field of HelmOp objects.
	// +optional
	// +nullable
	AllowedChartPatterns []string `json:"allowedChartPatterns,omitempty"`
}

// +kubebuilder:object:root=true

// PolicyList contains a list of Policy.
type PolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Policy `json:"items"`
}
