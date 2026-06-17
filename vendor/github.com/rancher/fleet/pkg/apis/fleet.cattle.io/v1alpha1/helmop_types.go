package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	InternalSchemeBuilder.Register(&HelmOp{}, &HelmOpList{})
}

var (
	HelmOpLabel = "fleet.cattle.io/fleet-helm-name"
)

const (
	HelmOpAcceptedCondition = "Accepted"
	HelmOpPolledCondition   = "Polled"

	// SecretTypeHelmOpsAccess is the secret type used to access Helm registries for HelmOps bundles.
	SecretTypeHelmOpsAccess = "fleet.cattle.io/bundle-helmops-access/v1alpha1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=fleet,path=helmops
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Repo",type=string,JSONPath=`.spec.helm.repo`
// +kubebuilder:printcolumn:name="Chart",type=string,JSONPath=`.spec.helm.chart`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`
// +kubebuilder:printcolumn:name="BundleDeployments-Ready",type=string,JSONPath=`.status.display.readyBundleDeployments`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`

// HelmOp describes a helm chart information.
// The resource contains the necessary information to deploy the chart to target clusters.
type HelmOp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HelmOpSpec   `json:"spec,omitempty"`
	Status HelmOpStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HelmOpList contains a list of HelmOp
type HelmOpList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HelmOp `json:"items"`
}

type HelmOpSpec struct {
	BundleSpec `json:",inline"`

	// Labels are copied to the bundle and can be used in a
	// dependsOn.selector.
	Labels map[string]string `json:"labels,omitempty"`

	// PollingInterval is how often to check the Helm repository for new updates.
	// +nullable
	PollingInterval *metav1.Duration `json:"pollingInterval,omitempty"`

	// HelmSecretName contains the auth secret with the credentials to access
	// a private Helm repository.
	// +nullable
	HelmSecretName string `json:"helmSecretName,omitempty"`

	// InsecureSkipTLSverify will use insecure HTTPS to clone the helm app resource.
	InsecureSkipTLSverify bool `json:"insecureSkipTLSVerify,omitempty"`
}

type HelmOpStatus struct {
	StatusBase `json:",inline"`

	// LastPollingTime is the last time the polling check was triggered
	LastPollingTime metav1.Time `json:"lastPollingTriggered,omitempty"`

	// Version installed for the helm chart.
	// When using * or empty version in the spec we get the latest version from
	// the helm repository when possible
	Version string `json:"version,omitempty"`
}
