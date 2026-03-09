package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	InternalSchemeBuilder.Register(&GitRepo{}, &GitRepoList{})
}

var (
	CommitLabel          = "fleet.cattle.io/commit"
	RepoLabel            = "fleet.cattle.io/repo-name"
	BundleLabel          = "fleet.cattle.io/bundle-name"
	BundleNamespaceLabel = "fleet.cattle.io/bundle-namespace"
)

const (
	GitRepoAcceptedCondition = "Accepted"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=fleet,path=gitrepos
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Repo",type=string,JSONPath=`.spec.repo`
// +kubebuilder:printcolumn:name="Commit",type=string,JSONPath=`.status.commit`
// +kubebuilder:printcolumn:name="BundleDeployments-Ready",type=string,JSONPath=`.status.display.readyBundleDeployments`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`

// GitRepo describes a git repository that is watched by Fleet.
// The resource contains the necessary information to deploy the repo, or parts
// of it, to target clusters.
type GitRepo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitRepoSpec   `json:"spec,omitempty"`
	Status GitRepoStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitRepoList contains a list of GitRepo
type GitRepoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitRepo `json:"items"`
}

type GitRepoSpec struct {
	// Repo is a URL to a git repo to clone and index.
	// +nullable
	Repo string `json:"repo,omitempty"`

	// Branch The git branch to follow.
	// +nullable
	Branch string `json:"branch,omitempty"`

	// Revision A specific commit or tag to operate on.
	// +nullable
	Revision string `json:"revision,omitempty"`

	// Ensure that all resources are created in this namespace
	// Any cluster scoped resource will be rejected if this is set
	// Additionally this namespace will be created on demand.
	// +nullable
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// ClientSecretName is the name of the client secret to be used to connect to the repo
	// It is expected the secret be of type "kubernetes.io/basic-auth" or "kubernetes.io/ssh-auth".
	// +nullable
	ClientSecretName string `json:"clientSecretName,omitempty"`

	// HelmSecretName contains the auth secret for a private Helm repository.
	// +nullable
	HelmSecretName string `json:"helmSecretName,omitempty"`

	// HelmSecretNameForPaths contains the auth secret for private Helm repository for each path.
	// +nullable
	HelmSecretNameForPaths string `json:"helmSecretNameForPaths,omitempty"`

	// HelmRepoURLRegex Helm credentials will be used if the helm repo matches this regex
	// Credentials will always be used if this is empty or not provided.
	// +nullable
	HelmRepoURLRegex string `json:"helmRepoURLRegex,omitempty"`

	// CABundle is a PEM encoded CA bundle which will be used to validate the repo's certificate.
	// +nullable
	CABundle []byte `json:"caBundle,omitempty"`

	// InsecureSkipTLSverify will use insecure HTTPS to clone the repo.
	InsecureSkipTLSverify bool `json:"insecureSkipTLSVerify,omitempty"`

	// Paths is the directories relative to the git repo root that contain resources to be applied.
	// Path globbing is supported, for example ["charts/*"] will match all folders as a subdirectory of charts/
	// If empty, "/" is the default.
	// +nullable
	Paths []string `json:"paths,omitempty"`

	// Paused, when true, causes changes in Git not to be propagated down to the clusters but instead to mark
	// resources as OutOfSync.
	Paused bool `json:"paused,omitempty"`

	// ServiceAccount used in the downstream cluster for deployment.
	// +nullable
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// Targets is a list of targets this repo will deploy to.
	Targets []GitTarget `json:"targets,omitempty"`

	// PollingInterval is how often to check git for new updates.
	// +nullable
	PollingInterval *metav1.Duration `json:"pollingInterval,omitempty"`

	// Increment this number to force a redeployment of contents from Git.
	ForceSyncGeneration int64 `json:"forceSyncGeneration,omitempty"`

	// ImageScanInterval is the interval of syncing scanned images and writing back to git repo.
	ImageSyncInterval *metav1.Duration `json:"imageScanInterval,omitempty"`

	// Commit specifies how to commit to the git repo when a new image is scanned and written back to git repo.
	ImageScanCommit *CommitSpec `json:"imageScanCommit,omitempty"`

	// KeepResources specifies if the resources created must be kept after deleting the GitRepo.
	KeepResources bool `json:"keepResources,omitempty"`

	// DeleteNamespace specifies if the namespace created must be deleted after deleting the GitRepo.
	DeleteNamespace bool `json:"deleteNamespace,omitempty"`

	// CorrectDrift specifies how drift correction should work.
	CorrectDrift *CorrectDrift `json:"correctDrift,omitempty"`

	// Disables git polling. When enabled only webhooks will be used.
	DisablePolling bool `json:"disablePolling,omitempty"`

	// OCIRegistry specifies the OCI registry related parameters
	OCIRegistry *OCIRegistrySpec `json:"ociRegistry,omitempty"`
}

// GitTarget is a cluster or cluster group to deploy to.
type GitTarget struct {
	// Name is the name of this target.
	// +nullable
	Name string `json:"name,omitempty"`
	// ClusterName is the name of a cluster.
	// +nullable
	ClusterName string `json:"clusterName,omitempty"`
	// ClusterSelector is a label selector to select clusters.
	// +nullable
	ClusterSelector *metav1.LabelSelector `json:"clusterSelector,omitempty"`
	// ClusterGroup is the name of a cluster group in the same namespace as the clusters.
	// +nullable
	ClusterGroup string `json:"clusterGroup,omitempty"`
	// ClusterGroupSelector is a label selector to select cluster groups.
	// +nullable
	ClusterGroupSelector *metav1.LabelSelector `json:"clusterGroupSelector,omitempty"`
}

type GitRepoStatus struct {
	StatusBase `json:",inline"`
	// ObservedGeneration is the current generation of the resource in the cluster. It is copied from k8s
	// metadata.Generation. The value is incremented for all changes, except for changes to .metadata or .status.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration"`
	// Update generation is the force update generation if spec.forceSyncGeneration is set
	UpdateGeneration int64 `json:"updateGeneration,omitempty"`
	// Commit is the Git commit hash from the last git job run.
	// +optional
	Commit string `json:"commit,omitempty"`
	// WebhookCommit is the latest Git commit hash received from a webhook
	// +optional
	WebhookCommit string `json:"webhookCommit,omitempty"`
	// GitJobStatus is the status of the last Git job run, e.g. "Current" if there was no error.
	GitJobStatus string `json:"gitJobStatus,omitempty"`
	// LastSyncedImageScanTime is the time of the last image scan.
	LastSyncedImageScanTime metav1.Time `json:"lastSyncedImageScanTime,omitempty"`
	// LastPollingTime is the last time the polling check was triggered
	LastPollingTime metav1.Time `json:"lastPollingTriggered,omitempty"`
}

type GitRepoDisplay struct {
	// ReadyBundleDeployments is a string in the form "%d/%d", that describes the
	// number of ready bundledeployments over the total number of bundledeployments.
	ReadyBundleDeployments string `json:"readyBundleDeployments,omitempty"`
	// State is the state of the GitRepo, e.g. "GitUpdating" or the maximal
	// BundleState according to StateRank.
	State string `json:"state,omitempty"`
	// Message contains the relevant message from the deployment conditions.
	Message string `json:"message,omitempty"`
	// Error is true if a message is present.
	Error bool `json:"error,omitempty"`
}

// CommitSpec specifies how to commit changes to the git repository
type CommitSpec struct {
	// AuthorName gives the name to provide when making a commit
	// +optional
	AuthorName string `json:"authorName"`
	// AuthorEmail gives the email to provide when making a commit
	// +optional
	AuthorEmail string `json:"authorEmail"`
	// MessageTemplate provides a template for the commit message,
	// into which will be interpolated the details of the change made.
	// +optional
	MessageTemplate string `json:"messageTemplate,omitempty"`
}

type CorrectDrift struct {
	// Enabled correct drift if true.
	Enabled bool `json:"enabled,omitempty"`
	// Force helm rollback with --force option will be used if true. This will try to recreate all resources in the release.
	Force bool `json:"force,omitempty"`
	// KeepFailHistory keeps track of failed rollbacks in the helm history.
	KeepFailHistory bool `json:"keepFailHistory,omitempty"`
}

type OCIRegistrySpec struct {
	// Reference of the OCI Registry
	Reference string `json:"reference,omitempty"`

	// AuthSecretName contains the auth secret where the OCI registry credentials are stored.
	// +nullable
	AuthSecretName string `json:"authSecretName,omitempty"`

	// BasicHTTP uses HTTP connections to the OCI registry when enabled.
	// +optional
	// +nullable
	BasicHTTP bool `json:"basicHTTP,omitempty"`

	// InsecureSkipTLS allows connections to OCI registry without certs when enabled.
	// +optional
	// +nullable
	InsecureSkipTLS bool `json:"insecureSkipTLS,omitempty"`
}
