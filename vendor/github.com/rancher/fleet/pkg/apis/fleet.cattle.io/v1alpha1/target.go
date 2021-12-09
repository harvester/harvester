package v1alpha1

import (
	"github.com/rancher/wrangler/pkg/genericcondition"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ClusterConditionReady                  = "Ready"
	ClusterGroupAnnotation                 = "fleet.cattle.io/cluster-group"
	ClusterNamespaceAnnotation             = "fleet.cattle.io/cluster-namespace"
	ClusterAnnotation                      = "fleet.cattle.io/cluster"
	ClusterRegistrationAnnotation          = "fleet.cattle.io/cluster-registration"
	ClusterRegistrationNamespaceAnnotation = "fleet.cattle.io/cluster-registration-namespace"
	ManagedLabel                           = "fleet.cattle.io/managed"

	BootstrapToken = "fleet.cattle.io/bootstrap-token"
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
	Paused                  bool        `json:"paused,omitempty"`
	ClientID                string      `json:"clientID,omitempty"`
	KubeConfigSecret        string      `json:"kubeConfigSecret,omitempty"`
	RedeployAgentGeneration int64       `json:"redeployAgentGeneration,omitempty"`
	AgentEnvVars            []v1.EnvVar `json:"agentEnvVars,omitempty"`
	AgentNamespace          string      `json:"agentNamespace,omitempty"`
}

type ClusterStatus struct {
	Conditions           []genericcondition.GenericCondition `json:"conditions,omitempty"`
	Namespace            string                              `json:"namespace,omitempty"`
	Summary              BundleSummary                       `json:"summary,omitempty"`
	ResourceCounts       GitRepoResourceCounts               `json:"resourceCounts,omitempty"`
	ReadyGitRepos        int                                 `json:"readyGitRepos"`
	DesiredReadyGitRepos int                                 `json:"desiredReadyGitRepos"`

	AgentEnvVarsHash        string `json:"agentEnvVarsHash,omitempty"`
	AgentDeployedGeneration *int64 `json:"agentDeployedGeneration,omitempty"`
	AgentMigrated           bool   `json:"agentMigrated,omitempty"`
	AgentNamespaceMigrated  bool   `json:"agentNamespaceMigrated,omitempty"`
	CattleNamespaceMigrated bool   `json:"cattleNamespaceMigrated,omitempty"`

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
	ClusterName string `json:"clusterName,omitempty"`
	Granted     bool   `json:"granted,omitempty"`
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
