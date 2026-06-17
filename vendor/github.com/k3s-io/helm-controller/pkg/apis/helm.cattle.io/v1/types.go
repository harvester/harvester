package v1

//revive:disable:struct-tag

import (
	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/intstr"
)

// +kubebuilder:validation:Enum={"abort","reinstall"}
type FailurePolicy string

// +kubebuilder:validation:Enum={"secret","configmap"}
type HelmDriver string

// +genclient
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=hc
// +kubebuilder:printcolumn:name="Repo",type=string,JSONPath=`.spec.repo`
// +kubebuilder:printcolumn:name="Chart",type=string,JSONPath=`.spec.chart`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="TargetNamespace",type=string,JSONPath=`.spec.targetNamespace`
// +kubebuilder:printcolumn:name="Bootstrap",type=boolean,JSONPath=`.spec.bootstrap`
// +kubebuilder:printcolumn:name="Failed",type=string,JSONPath=`.status.conditions[?(@.type=='Failed')].status`
// +kubebuilder:printcolumn:name="Job",type=string,JSONPath=`.status.jobName`,priority=10
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// HelmChart represents configuration and state for the deployment of a Helm chart.
type HelmChart struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HelmChartSpec   `json:"spec,omitempty"`
	Status HelmChartStatus `json:"status,omitempty"`
}

// HelmChartSpec represents the user-configurable details for installation and upgrade of a Helm chart release.
type HelmChartSpec struct {
	// Helm Chart target namespace.
	// Helm CLI positional argument/flag: `--namespace`
	TargetNamespace string `json:"targetNamespace,omitempty"`
	// Create target namespace if not present.
	// Helm CLI positional argument/flag: `--create-namespace`
	CreateNamespace bool `json:"createNamespace,omitempty"`
	// Helm Chart name in repository, or complete HTTPS URL to chart archive (.tgz)
	// Helm CLI positional argument/flag: `CHART`
	Chart string `json:"chart,omitempty"`
	// Helm Chart version. Only used when installing from repository; ignored when .spec.chart or .spec.chartContent is used to install a specific chart archive.
	// Helm CLI positional argument/flag: `--version`
	Version string `json:"version,omitempty"`
	// Helm Chart repository URL.
	// Helm CLI positional argument/flag: `--repo`
	Repo string `json:"repo,omitempty"`
	// Verify certificates of HTTPS-enabled servers using this CA bundle. Should be a string containing one or more PEM-encoded CA Certificates.
	// Helm CLI positional argument/flag: `--ca-file`
	RepoCA string `json:"repoCA,omitempty"`
	// Reference to a ConfigMap containing CA Certificates to be be trusted by Helm. Can be used along with or instead of `.spec.repoCA`
	// Helm CLI positional argument/flag: `--ca-file`
	RepoCAConfigMap *corev1.LocalObjectReference `json:"repoCAConfigMap,omitempty"`
	// Override simple Chart values. These take precedence over options set via values or valuesContent.
	// Helm CLI positional argument/flag: `--set`, `--set-string`
	Set map[string]intstr.IntOrString `json:"set,omitempty"`
	// Override complex Chart values via structured YAML. Takes precedence over options set via valuesContent.
	// Helm CLI positional argument/flag: `--values`
	Values *apiextv1.JSON `json:"values,omitempty"`
	// Override complex Chart values via inline YAML content.
	// Helm CLI positional argument/flag: `--values`
	ValuesContent string `json:"valuesContent,omitempty"`
	// Override complex Chart values via references to external Secrets.
	// Helm CLI positional argument/flag: `--values`
	ValuesSecrets []SecretSpec `json:"valuesSecrets,omitempty"`
	// DEPRECATED. Helm version to use. Only v3 is currently supported.
	HelmVersion string `json:"helmVersion,omitempty"`
	// Set to True if this chart is needed to bootstrap the cluster (Cloud Controller Manager, CNI, etc).
	Bootstrap bool `json:"bootstrap,omitempty"`
	// Set to True if helm should take ownership of existing resources when installing/upgrading the chart.
	// Helm CLI positional argument/flag: `--take-ownership`
	TakeOwnership bool `json:"takeOwnership,omitempty"`
	// Base64-encoded chart archive .tgz; overides `.spec.chart` and `.spec.version`.
	// Helm CLI positional argument/flag: `CHART`
	ChartContent string `json:"chartContent,omitempty"`
	// Specify the image to use for tht helm job pod when installing or upgrading the helm chart.
	JobImage string `json:"jobImage,omitempty"`
	// Specify the number of retries before considering the helm job failed.
	BackOffLimit *int32 `json:"backOffLimit,omitempty"`
	// Timeout for Helm operations.
	// Helm CLI positional argument/flag: `--timeout`
	Timeout *metav1.Duration `json:"timeout,omitempty"`
	// Configures handling of failed chart installation or upgrades.
	// - `reinstall` will perform a clean uninstall and reinstall of the chart.
	// - `abort` will take no action and leave the chart in a failed state so that the administrator can manually resolve the error.
	// +kubebuilder:default=reinstall
	FailurePolicy FailurePolicy `json:"failurePolicy,omitempty"`
	// Reference to Secret of type kubernetes.io/basic-auth holding Basic auth credentials for the Chart repo.
	AuthSecret *corev1.LocalObjectReference `json:"authSecret,omitempty"`
	// Pass Basic auth credentials to all domains.
	// Helm CLI positional argument/flag: `--pass-credentials`
	AuthPassCredentials bool `json:"authPassCredentials,omitempty"`
	// Skip TLS certificate checks for the chart download.
	// Helm CLI positional argument/flag: `--insecure-skip-tls-verify`
	InsecureSkipTLSVerify bool `json:"insecureSkipTLSVerify,omitempty"`
	// Use insecure HTTP connections for the chart download.
	// Helm CLI positional argument/flag: `--plain-http`
	PlainHTTP bool `json:"plainHTTP,omitempty"`
	// Reference to Secret of type kubernetes.io/dockerconfigjson holding Docker auth credentials for the OCI-based registry acting as the Chart repo.
	DockerRegistrySecret *corev1.LocalObjectReference `json:"dockerRegistrySecret,omitempty"`
	// Custom PodSecurityContext for the helm job pod.
	PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`
	// custom SecurityContext for the helm job pod.
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
	// Helm storage driver to use for this chart's release metadata.
	// `secret` stores releases in Kubernetes Secrets (default).
	// `configmap` stores releases in ConfigMaps.
	// This field is effectively immutable after the first install; changing the storage backend is not a supported migration path.
	// Helm CLI environment variable: `HELM_DRIVER`
	// +kubebuilder:default=secret
	// +kubebuilder:validation:XValidation:rule="!oldSelf.hasValue() || self == oldSelf.value()",message="driver is immutable after creation",optionalOldSelf=true
	Driver HelmDriver `json:"driver,omitempty"`
}

// HelmChartStatus represents the resulting state from processing HelmChart events
type HelmChartStatus struct {
	// The name of the job created to install or upgrade the chart.
	JobName string `json:"jobName,omitempty"`
	// `JobCreated` indicates that a job has been created to install or upgrade the chart.
	// `Failed` indicates that the helm job has failed and the failure policy is set to `abort`.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []HelmChartCondition `json:"conditions,omitempty"`
}

// +genclient
// +kubebuilder:resource:shortName=hcc
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// HelmChartConfig represents additional configuration for the installation of Helm chart release.
// This resource is intended for use when additional configuration needs to be passed to a HelmChart
// that is managed by an external system.
type HelmChartConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec HelmChartConfigSpec `json:"spec,omitempty"`
}

// HelmChartConfigSpec represents additional user-configurable details of an installed and configured Helm chart release.
// These fields are merged with or override the corresponding fields on the related HelmChart resource.
type HelmChartConfigSpec struct {
	// Override complex Chart values via structured YAML. Takes precedence over options set via valuesContent.
	// Helm CLI positional argument/flag: `--values`
	Values *apiextv1.JSON `json:"values,omitempty"`
	// Override complex Chart values via inline YAML content.
	// Helm CLI positional argument/flag: `--values`
	ValuesContent string `json:"valuesContent,omitempty"`
	// Override complex Chart values via references to external Secrets.
	// Helm CLI positional argument/flag: `--values`
	ValuesSecrets []SecretSpec `json:"valuesSecrets,omitempty"`
	// Configures handling of failed chart installation or upgrades.
	// - `reinstall` will perform a clean uninstall and reinstall of the chart.
	// - `abort` will take no action and leave the chart in a failed state so that the administrator can manually resolve the error.
	// +kubebuilder:default=reinstall
	FailurePolicy FailurePolicy `json:"failurePolicy,omitempty"`
}

type HelmChartConditionType string

const (
	HelmChartJobCreated HelmChartConditionType = "JobCreated"
	HelmChartFailed     HelmChartConditionType = "Failed"
)

type HelmChartCondition struct {
	// Type of job condition.
	Type HelmChartConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// (brief) reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// SecretSpec describes a key in a secret to load chart values from.
type SecretSpec struct {
	// Name of the secret. Must be in the same namespace as the HelmChart resource.
	Name string `json:"name,omitempty"`
	// Keys to read values content from. If no keys are specified, the secret is not used.
	Keys []string `json:"keys,omitempty"`
	// Ignore changes to the secret, and mark the secret as optional.
	// By default, the secret must exist, and changes to the secret will trigger an upgrade of the chart to apply the updated values.
	// If `ignoreUpdates` is true, the secret is optional, and changes to the secret will not trigger an upgrade of the chart.
	IgnoreUpdates bool `json:"ignoreUpdates,omitempty"`
}
