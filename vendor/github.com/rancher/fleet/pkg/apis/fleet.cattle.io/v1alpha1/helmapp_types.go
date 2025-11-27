package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	InternalSchemeBuilder.Register(&HelmApp{}, &HelmAppList{})
}

var (
	HelmAppLabel = "fleet.cattle.io/fleet-helm-name"
)

const (
	HelmAppAcceptedCondition = "Accepted"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=fleet,path=helmapps
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Repo",type=string,JSONPath=`.spec.helm.repo`
// +kubebuilder:printcolumn:name="Chart",type=string,JSONPath=`.spec.helm.chart`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`
// +kubebuilder:printcolumn:name="BundleDeployments-Ready",type=string,JSONPath=`.status.display.readyBundleDeployments`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`

// HelmApp describes a helm chart information.
// The resource contains the necessary information to deploy the chart to target clusters.
type HelmApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HelmAppSpec   `json:"spec,omitempty"`
	Status HelmAppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HelmAppList contains a list of HelmApp
type HelmAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HelmApp `json:"items"`
}

type HelmAppSpec struct {
	BundleSpec `json:",inline"`
	// Labels are copied to the bundle and can be used in a
	// dependsOn.selector.
	Labels map[string]string `json:"labels,omitempty"`
	// HelmSecretName contains the auth secret with the credentials to access
	// a private Helm repository.
	// +nullable
	HelmSecretName string `json:"helmSecretName,omitempty"`
	// InsecureSkipTLSverify will use insecure HTTPS to clone the helm app resource.
	InsecureSkipTLSverify bool `json:"insecureSkipTLSVerify,omitempty"`
}

type HelmAppStatus struct {
	StatusBase `json:",inline"`
	// Version installed for the helm chart.
	// When using * or empty version in the spec we get the latest version from
	// the helm repository when possible
	Version string `json:"version,omitempty"`
}
