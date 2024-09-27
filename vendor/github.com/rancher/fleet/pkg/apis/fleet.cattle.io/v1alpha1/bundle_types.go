package v1alpha1

import (
	"github.com/rancher/wrangler/v3/pkg/genericcondition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func init() {
	InternalSchemeBuilder.Register(&Bundle{}, &BundleList{})
}

const (
	// Ready: Bundles have been deployed and all resources are ready.
	Ready BundleState = "Ready"
	// NotReady: Bundles have been deployed and some resources are not
	// ready.
	NotReady BundleState = "NotReady"
	// WaitApplied: Bundles have been synced from Fleet controller and
	// downstream cluster, but are waiting to be deployed.
	WaitApplied BundleState = "WaitApplied"
	// ErrApplied: Bundles have been synced from the Fleet controller and
	// the downstream cluster, but there were some errors when deploying
	// the Bundle.
	ErrApplied BundleState = "ErrApplied"
	// OutOfSync: Bundles have been synced from Fleet controller, but
	// downstream agent hasn't synced the change yet.
	OutOfSync BundleState = "OutOfSync"
	// Pending: Bundles are being processed by Fleet controller.
	Pending BundleState = "Pending"
	// Modified: Bundles have been deployed and all resources are ready,
	// but there are some changes that were not made from the Git
	// Repository.
	Modified BundleState = "Modified"
)

var (
	// StateRank ranks the state, e.g. so the highest ranked non-ready
	// state can be reported in a summary.
	StateRank = map[BundleState]int{
		ErrApplied:  7,
		WaitApplied: 6,
		Modified:    5,
		OutOfSync:   4,
		Pending:     3,
		NotReady:    2,
		Ready:       1,
	}
)

type BundleState string

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="BundleDeployments-Ready",type=string,JSONPath=`.status.display.readyClusters`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`

// Bundle contains the resources of an application and its deployment options.
// It will be deployed as a Helm chart to target clusters.
//
// When a GitRepo is scanned it will produce one or more bundles. Bundles are
// a collection of resources that get deployed to one or more cluster(s). Bundle is the
// fundamental deployment unit used in Fleet. The contents of a Bundle may be
// Kubernetes manifests, Kustomize configuration, or Helm charts. Regardless
// of the source the contents are dynamically rendered into a Helm chart by
// the agent and installed into the downstream cluster as a Helm release.
type Bundle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec BundleSpec `json:"spec"`
	// +optional
	Status BundleStatus `json:"status"`
}

// +kubebuilder:object:root=true

// BundleList contains a list of Bundle
type BundleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Bundle `json:"items"`
}

type BundleSpec struct {
	BundleDeploymentOptions `json:",inline"`

	// Paused if set to true, will stop any BundleDeployments from being updated. It will be marked as out of sync.
	Paused bool `json:"paused,omitempty"`

	// RolloutStrategy controls the rollout of bundles, by defining
	// partitions, canaries and percentages for cluster availability.
	// +nullable
	RolloutStrategy *RolloutStrategy `json:"rolloutStrategy,omitempty"`

	// Resources contains the resources that were read from the bundle's
	// path. This includes the content of downloaded helm charts.
	// +nullable
	Resources []BundleResource `json:"resources,omitempty"`

	// Targets refer to the clusters which will be deployed to.
	// Targets are evaluated in order and the first one to match is used.
	Targets []BundleTarget `json:"targets,omitempty"`

	// TargetRestrictions is an allow list, which controls if a bundledeployment is created for a target.
	TargetRestrictions []BundleTargetRestriction `json:"targetRestrictions,omitempty"`

	// DependsOn refers to the bundles which must be ready before this bundle can be deployed.
	// +nullable
	DependsOn []BundleRef `json:"dependsOn,omitempty"`

	// ContentsID stores the contents id when deploying contents using an OCI registry.
	// +nullable
	ContentsID string `json:"contentsId,omitempty"`
}

type BundleRef struct {
	// Name of the bundle.
	// +nullable
	Name string `json:"name,omitempty"`
	// Selector matching bundle's labels.
	// +nullable
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

// BundleResource represents the content of a single resource from the bundle, like a YAML manifest.
type BundleResource struct {
	// Name of the resource, can include the bundle's internal path.
	// +nullable
	Name string `json:"name,omitempty"`
	// The content of the resource, can be compressed.
	// +nullable
	Content string `json:"content,omitempty"`
	// Encoding is either empty or "base64+gz".
	// +nullable
	Encoding string `json:"encoding,omitempty"`
}

// RolloverStrategy controls the rollout of the bundle across clusters.
type RolloutStrategy struct {
	// A number or percentage of clusters that can be unavailable during an update
	// of a bundle. This follows the same basic approach as a deployment rollout
	// strategy. Once the number of clusters meets unavailable state update will be
	// paused. Default value is 100% which doesn't take effect on update.
	// default: 100%
	// +nullable
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
	// A number or percentage of cluster partitions that can be unavailable during
	// an update of a bundle.
	// default: 0
	// +nullable
	MaxUnavailablePartitions *intstr.IntOrString `json:"maxUnavailablePartitions,omitempty"`
	// A number or percentage of how to automatically partition clusters if no
	// specific partitioning strategy is configured.
	// default: 25%
	// +nullable
	AutoPartitionSize *intstr.IntOrString `json:"autoPartitionSize,omitempty"`
	// A list of definitions of partitions.  If any target clusters do not match
	// the configuration they are added to partitions at the end following the
	// autoPartitionSize.
	// +nullable
	Partitions []Partition `json:"partitions,omitempty"`
}

// Partition defines a separate rollout strategy for a set of clusters.
type Partition struct {
	// A user-friendly name given to the partition used for Display (optional).
	// +nullable
	Name string `json:"name,omitempty"`
	// A number or percentage of clusters that can be unavailable in this
	// partition before this partition is treated as done.
	// default: 10%
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
	// ClusterName is the name of a cluster to include in this partition
	ClusterName string `json:"clusterName,omitempty"`
	// Selector matching cluster labels to include in this partition
	ClusterSelector *metav1.LabelSelector `json:"clusterSelector,omitempty"`
	// A cluster group name to include in this partition
	ClusterGroup string `json:"clusterGroup,omitempty"`
	// Selector matching cluster group labels to include in this partition
	// +nullable
	ClusterGroupSelector *metav1.LabelSelector `json:"clusterGroupSelector,omitempty"`
}

// BundleTargetRestriction is used internally by Fleet and should not be modified.
// It acts as an allow list, to prevent the creation of BundleDeployments from
// Targets created by TargetCustomizations in fleet.yaml.
type BundleTargetRestriction struct {
	// +nullable
	Name string `json:"name,omitempty"`
	// +nullable
	ClusterName string `json:"clusterName,omitempty"`
	// +nullable
	ClusterSelector *metav1.LabelSelector `json:"clusterSelector,omitempty"`
	// +nullable
	ClusterGroup string `json:"clusterGroup,omitempty"`
	// +nullable
	ClusterGroupSelector *metav1.LabelSelector `json:"clusterGroupSelector,omitempty"`
}

// BundleTarget declares clusters to deploy to. Fleet will merge the
// BundleDeploymentOptions from customizations into this struct.
type BundleTarget struct {
	BundleDeploymentOptions `json:",inline"`
	// Name of target. This value is largely for display and logging. If
	// not specified a default name of the format "target000" will be used
	Name string `json:"name,omitempty"`
	// ClusterName to match a specific cluster by name that will be
	// selected
	// +nullable
	ClusterName string `json:"clusterName,omitempty"`
	// ClusterSelector is a selector to match clusters. The structure is
	// the standard metav1.LabelSelector format. If clusterGroupSelector or
	// clusterGroup is specified, clusterSelector will be used only to
	// further refine the selection after clusterGroupSelector and
	// clusterGroup is evaluated.
	// +nullable
	ClusterSelector *metav1.LabelSelector `json:"clusterSelector,omitempty"`
	// ClusterGroup to match a specific cluster group by name.
	// +nullable
	ClusterGroup string `json:"clusterGroup,omitempty"`
	// ClusterGroupSelector is a selector to match cluster groups.
	// +nullable
	ClusterGroupSelector *metav1.LabelSelector `json:"clusterGroupSelector,omitempty"`
	// DoNotDeploy if set to true, will not deploy to this target.
	DoNotDeploy bool `json:"doNotDeploy,omitempty"`
}

// BundleSummary contains the number of bundle deployments in each state and a
// list of non-ready resources. It is used in the bundle, clustergroup, cluster
// and gitrepo status.
type BundleSummary struct {
	// NotReady is the number of bundle deployments that have been deployed
	// where some resources are not ready.
	NotReady int `json:"notReady,omitempty"`
	// WaitApplied is the number of bundle deployments that have been
	// synced from Fleet controller and downstream cluster, but are waiting
	// to be deployed.
	WaitApplied int `json:"waitApplied,omitempty"`
	// ErrApplied is the number of bundle deployments that have been synced
	// from the Fleet controller and the downstream cluster, but with some
	// errors when deploying the bundle.
	ErrApplied int `json:"errApplied,omitempty"`
	// OutOfSync is the number of bundle deployments that have been synced
	// from Fleet controller, but not yet by the downstream agent.
	OutOfSync int `json:"outOfSync,omitempty"`
	// Modified is the number of bundle deployments that have been deployed
	// and for which all resources are ready, but where some changes from the
	// Git repository have not yet been synced.
	Modified int `json:"modified,omitempty"`
	// Ready is the number of bundle deployments that have been deployed
	// where all resources are ready.
	// +optional
	Ready int `json:"ready"`
	// Pending is the number of bundle deployments that are being processed
	// by Fleet controller.
	Pending int `json:"pending,omitempty"`
	// DesiredReady is the number of bundle deployments that should be
	// ready.
	// +optional
	DesiredReady int `json:"desiredReady"`
	// NonReadyClusters is a list of states, which is filled for a bundle
	// that is not ready.
	// +nullable
	NonReadyResources []NonReadyResource `json:"nonReadyResources,omitempty"`
}

// NonReadyResource contains information about a bundle that is not ready for a
// given state like "ErrApplied". It contains a list of non-ready or modified
// resources and their states.
type NonReadyResource struct {
	// Name is the name of the resource.
	// +nullable
	Name string `json:"name,omitempty"`
	// State is the state of the resource, like e.g. "NotReady" or "ErrApplied".
	// +nullable
	State BundleState `json:"bundleState,omitempty"`
	// Message contains information why the bundle is not ready.
	// +nullable
	Message string `json:"message,omitempty"`
	// ModifiedStatus lists the state for each modified resource.
	// +nullable
	ModifiedStatus []ModifiedStatus `json:"modifiedStatus,omitempty"`
	// NonReadyStatus lists the state for each non-ready resource.
	// +nullable
	NonReadyStatus []NonReadyStatus `json:"nonReadyStatus,omitempty"`
}

const (
	// BundleConditionReady is unused. A "Ready" condition on a bundle
	// indicates that its resources are ready and the dependencies are
	// fulfilled.
	BundleConditionReady = "Ready"
	// BundleDeploymentConditionReady is the condition that displays for
	// status in general and it is used for the readiness of resources.
	BundleDeploymentConditionReady = "Ready"
	// BundleDeploymentConditionInstalled indicates the bundledeployment
	// has been installed.
	BundleDeploymentConditionInstalled = "Installed"
	// BundleDeploymentConditionDeployed is used by the bundledeployment
	// controller. It is true if the handler returns no error and false if
	// an error is returned.
	BundleDeploymentConditionDeployed  = "Deployed"
	BundleDeploymentConditionMonitored = "Monitored"
)

type BundleStatus struct {
	// Conditions is a list of Wrangler conditions that describe the state
	// of the bundle.
	// +optional
	Conditions []genericcondition.GenericCondition `json:"conditions,omitempty"`

	// Summary contains the number of bundle deployments in each state and
	// a list of non-ready resources.
	Summary BundleSummary `json:"summary,omitempty"`
	// NewlyCreated is the number of bundle deployments that have been created,
	// not updated.
	NewlyCreated int `json:"newlyCreated,omitempty"`
	// Unavailable is the number of bundle deployments that are not ready or
	// where the AppliedDeploymentID in the status does not match the
	// DeploymentID from the spec.
	// +optional
	Unavailable int `json:"unavailable"`
	// UnavailablePartitions is the number of unavailable partitions.
	// +optional
	UnavailablePartitions int `json:"unavailablePartitions"`
	// MaxUnavailable is the maximum number of unavailable deployments. See
	// rollout configuration.
	// +optional
	MaxUnavailable int `json:"maxUnavailable"`
	// MaxUnavailablePartitions is the maximum number of unavailable
	// partitions. The rollout configuration defines a maximum number or
	// percentage of unavailable partitions.
	// +optional
	MaxUnavailablePartitions int `json:"maxUnavailablePartitions"`
	// MaxNew is always 50. A bundle change can only stage 50
	// bundledeployments at a time.
	MaxNew int `json:"maxNew,omitempty"`
	// PartitionStatus lists the status of each partition.
	PartitionStatus []PartitionStatus `json:"partitions,omitempty"`
	// Display contains the number of ready, desiredready clusters and a
	// summary state for the bundle's resources.
	Display BundleDisplay `json:"display,omitempty"`
	// ResourceKey lists resources, which will likely be deployed. The
	// actual list of resources on a cluster might differ, depending on the
	// helm chart, value templating, etc..
	// +nullable
	ResourceKey []ResourceKey `json:"resourceKey,omitempty"`
	// OCIReference is the OCI reference used to store contents, this is
	// only for informational purposes.
	OCIReference string `json:"ociReference,omitempty"`
	// ObservedGeneration is the current generation of the bundle.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration"`
	// ResourcesSHA256Sum corresponds to the JSON serialization of the .Spec.Resources field
	ResourcesSHA256Sum string `json:"resourcesSha256Sum,omitempty"`
}

// ResourceKey lists resources, which will likely be deployed.
type ResourceKey struct {
	// Kind is the k8s api kind of the resource.
	// +nullable
	Kind string `json:"kind,omitempty"`
	// APIVersion is the k8s api version of the resource.
	// +nullable
	APIVersion string `json:"apiVersion,omitempty"`
	// Namespace is the namespace of the resource.
	// +nullable
	Namespace string `json:"namespace,omitempty"`
	// Name is the name of the resource.
	// +nullable
	Name string `json:"name,omitempty"`
}

// BundleDisplay contains the number of ready, desiredready clusters and a
// summary state for the bundle.
type BundleDisplay struct {
	// ReadyClusters is a string in the form "%d/%d", that describes the
	// number of clusters that are ready vs. the number of clusters desired
	// to be ready.
	// +nullable
	ReadyClusters string `json:"readyClusters,omitempty"`
	// State is a summary state for the bundle, calculated over the non-ready resources.
	// +nullable
	State string `json:"state,omitempty"`
}

// PartitionStatus is the status of a single rollout partition.
type PartitionStatus struct {
	// Name is the name of the partition.
	// +nullable
	Name string `json:"name,omitempty"`
	// Count is the number of clusters in the partition.
	Count int `json:"count,omitempty"`
	// MaxUnavailable is the maximum number of unavailable clusters in the partition.
	MaxUnavailable int `json:"maxUnavailable,omitempty"`
	// Unavailable is the number of unavailable clusters in the partition.
	Unavailable int `json:"unavailable,omitempty"`
	// Summary is a summary state for the partition, calculated over its non-ready resources.
	Summary BundleSummary `json:"summary,omitempty"`
}
