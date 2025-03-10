package v1alpha1

import (
	"github.com/rancher/wrangler/v3/pkg/genericcondition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	InternalSchemeBuilder.Register(&Cluster{}, &ClusterList{})
}

const ClusterResourceNamePlural = "clusters"

var (
	// ClusterConditionReady indicates that all bundles in this cluster
	// have been deployed and all resources are ready.
	ClusterConditionReady = "Ready"
	// ClusterConditionProcessed indicates that the status fields have been
	// processed.
	ClusterConditionProcessed = "Processed"
	// ClusterNamespaceAnnotation used on a cluster namespace to refer to
	// the cluster registration namespace, which contains the cluster
	// resource.
	ClusterNamespaceAnnotation = "fleet.cattle.io/cluster-namespace"
	// ClusterAnnotation used on a cluster namespace to refer to the
	// cluster name for that namespace.
	ClusterAnnotation = "fleet.cattle.io/cluster"
	// ClusterRegistrationAnnotation is the name of the
	// ClusterRegistration, it's added to the request service account.
	ClusterRegistrationAnnotation = "fleet.cattle.io/cluster-registration"
	// ClusterRegistrationTokenAnnotation is the namespace of the
	// clusterregistration, e.g. "fleet-local".
	ClusterRegistrationNamespaceAnnotation = "fleet.cattle.io/cluster-registration-namespace"
	// ManagedLabel is used for clean up. Cluster namespaces and other
	// resources with this label will be cleaned up. Used in Rancher to
	// identify fleet namespaces.
	ManagedLabel = "fleet.cattle.io/managed"
	// ClusterNamespaceLabel is used on a bundledeployment to refer to the
	// cluster registration namespace of the targeted cluster.
	ClusterNamespaceLabel = "fleet.cattle.io/cluster-namespace"
	// ClusterLabel is used on a bundledeployment to refer to the targeted
	// cluster
	ClusterLabel = "fleet.cattle.io/cluster"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Bundles-Ready",type=string,JSONPath=`.status.display.readyBundles`
// +kubebuilder:printcolumn:name="Last-Seen",type=string,JSONPath=`.status.agent.lastSeen`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`

// Cluster corresponds to a Kubernetes cluster. Fleet deploys bundles to targeted clusters.
// Clusters to which Fleet deploys manifests are referred to as downstream
// clusters. In the single cluster use case, the Fleet manager Kubernetes
// cluster is both the manager and downstream cluster at the same time.
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterList contains a list of Cluster
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}

type ClusterSpec struct {
	// Paused if set to true, will stop any BundleDeployments from being updated.
	Paused bool `json:"paused,omitempty"`

	// ClientID is a unique string that will identify the cluster. It can
	// either be predefined, or generated when importing the cluster.
	// +nullable
	ClientID string `json:"clientID,omitempty"`

	// KubeConfigSecret is the name of the secret containing the kubeconfig for the downstream cluster.
	// It can optionally contain a APIServerURL and CA to override the
	// values in the fleet-controller's configmap.
	// +nullable
	KubeConfigSecret string `json:"kubeConfigSecret,omitempty"`

	// KubeConfigSecretNamespace is the namespace of the secret containing the kubeconfig for the downstream cluster.
	// If unset, it will be assumed the secret can be found in the namespace that the Cluster object resides within.
	// +nullable
	KubeConfigSecretNamespace string `json:"kubeConfigSecretNamespace,omitempty"`

	// RedeployAgentGeneration can be used to force redeploying the agent.
	RedeployAgentGeneration int64 `json:"redeployAgentGeneration,omitempty"`

	// AgentEnvVars are extra environment variables to be added to the agent deployment.
	// +nullable
	AgentEnvVars []corev1.EnvVar `json:"agentEnvVars,omitempty"`

	// AgentNamespace defaults to the system namespace, e.g. cattle-fleet-system.
	// +nullable
	AgentNamespace string `json:"agentNamespace,omitempty"`

	// PrivateRepoURL prefixes the image name and overrides a global repo URL from the agents config.
	// +nullable
	PrivateRepoURL string `json:"privateRepoURL,omitempty"`

	// TemplateValues defines a cluster specific mapping of values to be sent to fleet.yaml values templating.
	// +nullable
	// +kubebuilder:validation:XPreserveUnknownFields
	TemplateValues *GenericMap `json:"templateValues,omitempty"`

	// AgentTolerations defines an extra set of Tolerations to be added to the Agent deployment.
	// +nullable
	AgentTolerations []corev1.Toleration `json:"agentTolerations,omitempty"`

	// AgentAffinity overrides the default affinity for the cluster's agent
	// deployment. If this value is nil the default affinity is used.
	// +nullable
	AgentAffinity *corev1.Affinity `json:"agentAffinity,omitempty"`

	// +nullable
	// AgentResources sets the resources for the cluster's agent deployment.
	AgentResources *corev1.ResourceRequirements `json:"agentResources,omitempty"`
}

type ClusterStatus struct {
	Conditions []genericcondition.GenericCondition `json:"conditions,omitempty"`

	// Namespace is the cluster namespace, it contains the clusters service
	// account as well as any bundledeployments. Example:
	// "cluster-fleet-local-cluster-294db1acfa77-d9ccf852678f"
	Namespace string `json:"namespace,omitempty"`

	// Summary is a summary of the bundledeployments. The resource counts
	// are copied from the gitrepo resource.
	Summary BundleSummary `json:"summary,omitempty"`
	// ResourceCounts is an aggregate over the GitRepoResourceCounts.
	ResourceCounts GitRepoResourceCounts `json:"resourceCounts,omitempty"`
	// ReadyGitRepos is the number of gitrepos for this cluster that are ready.
	// +optional
	ReadyGitRepos int `json:"readyGitRepos"`
	// DesiredReadyGitRepos is the number of gitrepos for this cluster that
	// are desired to be ready.
	// +optional
	DesiredReadyGitRepos int `json:"desiredReadyGitRepos"`

	// AgentEnvVarsHash is a hash of the agent's env vars, used to detect changes.
	// +nullable
	AgentEnvVarsHash string `json:"agentEnvVarsHash,omitempty"`
	// AgentPrivateRepoURL is the private repo URL for the agent that is currently used.
	// +nullable
	AgentPrivateRepoURL string `json:"agentPrivateRepoURL,omitempty"`
	// AgentDeployedGeneration is the generation of the agent that is currently deployed.
	// +nullable
	AgentDeployedGeneration *int64 `json:"agentDeployedGeneration,omitempty"`
	// AgentMigrated is always set to true after importing a cluster. If
	// false, it will trigger a migration. Old agents don't have
	// this in their status.
	AgentMigrated bool `json:"agentMigrated,omitempty"`
	// AgentNamespaceMigrated is always set to true after importing a
	// cluster. If false, it will trigger a migration. Old Fleet agents
	// don't have this in their status.
	AgentNamespaceMigrated bool `json:"agentNamespaceMigrated,omitempty"`
	// CattleNamespaceMigrated is always set to true after importing a
	// cluster. If false, it will trigger a migration. Old Fleet agents,
	// don't have this in their status.
	CattleNamespaceMigrated bool `json:"cattleNamespaceMigrated,omitempty"`

	// AgentAffinityHash is a hash of the agent's affinity configuration,
	// used to detect changes.
	AgentAffinityHash string `json:"agentAffinityHash,omitempty"`
	// AgentResourcesHash is a hash of the agent's resources configuration,
	// used to detect changes.
	// +nullable
	AgentResourcesHash string `json:"agentResourcesHash,omitempty"`
	// AgentTolerationsHash is a hash of the agent's tolerations
	// configuration, used to detect changes.
	// +nullable
	AgentTolerationsHash string `json:"agentTolerationsHash,omitempty"`
	// AgentConfigChanged is set to true if any of the agent configuration
	// changed, like the API server URL or CA. Setting it to true will
	// trigger a re-import of the cluster.
	AgentConfigChanged bool `json:"agentConfigChanged,omitempty"`

	// APIServerURL is the currently used URL of the API server that the
	// cluster uses to connect to upstream.
	// +nullable
	APIServerURL string `json:"apiServerURL,omitempty"`
	// APIServerCAHash is a hash of the upstream API server CA, used to detect changes.
	// +nullable
	APIServerCAHash string `json:"apiServerCAHash,omitempty"`

	// AgentTLSMode supports two values: `system-store` and `strict`. If set to
	// `system-store`, instructs the agent to trust CA bundles from the operating
	// system's store. If set to `strict`, then the agent shall only connect to a
	// server which uses the exact CA configured when creating/updating the agent.
	// +nullable
	AgentTLSMode string `json:"agentTLSMode,omitempty"`

	// Display contains the number of ready bundles, nodes and a summary state.
	Display ClusterDisplay `json:"display,omitempty"`
	// AgentStatus contains information about the agent.
	Agent AgentStatus `json:"agent,omitempty"`
}

type ClusterDisplay struct {
	// ReadyBundles is a string in the form "%d/%d", that describes the
	// number of bundles that are ready vs. the number of bundles desired
	// to be ready.
	ReadyBundles string `json:"readyBundles,omitempty"`
	// State of the cluster, either one of the bundle states, or "WaitCheckIn".
	// +nullable
	State string `json:"state,omitempty"`
}

type AgentStatus struct {
	// LastSeen is the last time the agent checked in to update the status
	// of the cluster resource.
	// +nullable
	// +optional
	LastSeen metav1.Time `json:"lastSeen"`
	// Namespace is the namespace of the agent deployment, e.g. "cattle-fleet-system".
	// +nullable
	// +optional
	Namespace string `json:"namespace"`
}
