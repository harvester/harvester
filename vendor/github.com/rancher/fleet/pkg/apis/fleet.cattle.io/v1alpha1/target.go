package v1alpha1

import (
	"github.com/rancher/wrangler/pkg/genericcondition"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ClusterConditionReady = "Ready"
	// ClusterNamespaceAnnotation used on a cluster namespace to refer to
	// the cluster registration namespace, which contains the cluster
	// resource.
	ClusterNamespaceAnnotation = "fleet.cattle.io/cluster-namespace"
	// ClusterAnnotation used on a cluster namespace to refer to the
	// cluster name for that namespace.
	ClusterAnnotation                      = "fleet.cattle.io/cluster"
	ClusterRegistrationAnnotation          = "fleet.cattle.io/cluster-registration"
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

type ClusterGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterGroupSpec   `json:"spec"`
	Status ClusterGroupStatus `json:"status"`
}

type ClusterGroupSpec struct {
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

type ClusterGroupStatus struct {
	ClusterCount         int                                 `json:"clusterCount"`
	NonReadyClusterCount int                                 `json:"nonReadyClusterCount"`
	NonReadyClusters     []string                            `json:"nonReadyClusters,omitempty"`
	Conditions           []genericcondition.GenericCondition `json:"conditions,omitempty"`
	Summary              BundleSummary                       `json:"summary,omitempty"`
	Display              ClusterGroupDisplay                 `json:"display,omitempty"`
	ResourceCounts       GitRepoResourceCounts               `json:"resourceCounts,omitempty"`
}

type ClusterGroupDisplay struct {
	ReadyClusters string `json:"readyClusters,omitempty"`
	ReadyBundles  string `json:"readyBundles,omitempty"`
	State         string `json:"state,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

type ClusterSpec struct {
	// Paused if set to true, will stop any BundleDeployments from being updated.
	Paused bool `json:"paused,omitempty"`

	// ClientID is a unique string that will identify the cluster. It can
	// either be predefined, or generated when importing the cluster.
	ClientID string `json:"clientID,omitempty"`

	// KubeConfigSecret is the name of the secret containing the kubeconfig for the downstream cluster.
	KubeConfigSecret string `json:"kubeConfigSecret,omitempty"`

	// RedeployAgentGeneration can be used to force redeploying the agent.
	RedeployAgentGeneration int64 `json:"redeployAgentGeneration,omitempty"`

	// AgentEnvVars are extra environment variables to be added to the agent deployment.
	AgentEnvVars []v1.EnvVar `json:"agentEnvVars,omitempty"`

	// AgentNamespace defaults to the system namespace, e.g. cattle-fleet-system.
	AgentNamespace string `json:"agentNamespace,omitempty"`

	// PrivateRepoURL prefixes the image name and overrides a global repo URL from the agents config.
	PrivateRepoURL string `json:"privateRepoURL,omitempty"`

	// TemplateValues defines a cluster specific mapping of values to be sent to fleet.yaml values templating.
	TemplateValues *GenericMap `json:"templateValues,omitempty"`

	// AgentTolerations defines an extra set of Tolerations to be added to the Agent deployment.
	AgentTolerations []v1.Toleration `json:"agentTolerations,omitempty"`

	// AgentAffinity overrides the default affinity for the cluster's agent
	// deployment. If this value is nil the default affinity is used.
	AgentAffinity *v1.Affinity `json:"agentAffinity,omitempty"`

	// AgentResources sets the resources for the cluster's agent deployment.
	AgentResources *v1.ResourceRequirements `json:"agentResources,omitempty"`
}

type ClusterStatus struct {
	Conditions []genericcondition.GenericCondition `json:"conditions,omitempty"`

	// Namespace is the cluster namespace, it contains the clusters service
	// account as well as any bundledeployments. Example:
	// "cluster-fleet-local-cluster-294db1acfa77-d9ccf852678f"
	Namespace string `json:"namespace,omitempty"`

	Summary              BundleSummary         `json:"summary,omitempty"`
	ResourceCounts       GitRepoResourceCounts `json:"resourceCounts,omitempty"`
	ReadyGitRepos        int                   `json:"readyGitRepos"`
	DesiredReadyGitRepos int                   `json:"desiredReadyGitRepos"`

	AgentEnvVarsHash        string `json:"agentEnvVarsHash,omitempty"`
	AgentPrivateRepoURL     string `json:"agentPrivateRepoURL,omitempty"`
	AgentDeployedGeneration *int64 `json:"agentDeployedGeneration,omitempty"`
	AgentMigrated           bool   `json:"agentMigrated,omitempty"`
	AgentNamespaceMigrated  bool   `json:"agentNamespaceMigrated,omitempty"`
	CattleNamespaceMigrated bool   `json:"cattleNamespaceMigrated,omitempty"`

	AgentAffinityHash    string `json:"agentAffinityHash,omitempty"`
	AgentResourcesHash   string `json:"agentResourcesHash,omitempty"`
	AgentTolerationsHash string `json:"agentTolerationsHash,omitempty"`
	AgentConfigChanged   bool   `json:"agentConfigChanged,omitempty"`

	APIServerURL    string `json:"apiServerURL,omitempty"`
	APIServerCAHash string `json:"apiServerCAHash,omitempty"`

	Display ClusterDisplay `json:"display,omitempty"`
	Agent   AgentStatus    `json:"agent,omitempty"`
}

type ClusterDisplay struct {
	ReadyBundles string `json:"readyBundles,omitempty"`
	ReadyNodes   string `json:"readyNodes,omitempty"`
	SampleNode   string `json:"sampleNode,omitempty"`
	State        string `json:"state,omitempty"`
}

type AgentStatus struct {
	LastSeen      metav1.Time `json:"lastSeen"`
	Namespace     string      `json:"namespace"`
	NonReadyNodes int         `json:"nonReadyNodes"`
	ReadyNodes    int         `json:"readyNodes"`
	// At most 3 nodes
	NonReadyNodeNames []string `json:"nonReadyNodeNames"`
	// At most 3 nodes
	ReadyNodeNames []string `json:"readyNodeNames"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ClusterRegistration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterRegistrationSpec   `json:"spec,omitempty"`
	Status ClusterRegistrationStatus `json:"status,omitempty"`
}

type ClusterRegistrationSpec struct {
	ClientID      string            `json:"clientID,omitempty"`
	ClientRandom  string            `json:"clientRandom,omitempty"`
	ClusterLabels map[string]string `json:"clusterLabels,omitempty"`
}

type ClusterRegistrationStatus struct {
	// ClusterName is only set after the registration is being processed by
	// fleet-controller.
	ClusterName string `json:"clusterName,omitempty"`
	// Granted is set to true, if the request service account is present
	// and its token secret exists. This happens directly before creating
	// the registration secret, roles and rolebindings.
	Granted bool `json:"granted,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ClusterRegistrationToken struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterRegistrationTokenSpec   `json:"spec,omitempty"`
	Status ClusterRegistrationTokenStatus `json:"status,omitempty"`
}

type ClusterRegistrationTokenSpec struct {
	TTL *metav1.Duration `json:"ttl,omitempty"`
}

type ClusterRegistrationTokenStatus struct {
	Expires    *metav1.Time `json:"expires,omitempty"`
	SecretName string       `json:"secretName,omitempty"`
}
