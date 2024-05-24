package v1alpha1

import (
	"fmt"
	"strings"

	"github.com/rancher/wrangler/pkg/genericcondition"
	"github.com/rancher/wrangler/pkg/summary"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	Ready       BundleState = "Ready"
	NotReady    BundleState = "NotReady"
	WaitApplied BundleState = "WaitApplied"
	ErrApplied  BundleState = "ErrApplied"
	OutOfSync   BundleState = "OutOfSync"
	Pending     BundleState = "Pending"
	Modified    BundleState = "Modified"

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

// MaxHelmReleaseNameLen is the maximum length of a Helm release name.
// See https://github.com/helm/helm/blob/293b50c65d4d56187cd4e2f390f0ada46b4c4737/pkg/chartutil/validate_name.go#L54-L61
const MaxHelmReleaseNameLen = 53

type BundleState string

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Bundle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BundleSpec   `json:"spec"`
	Status BundleStatus `json:"status"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BundleNamespaceMapping struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	BundleSelector    *metav1.LabelSelector `json:"bundleSelector,omitempty"`
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

type BundleSpec struct {
	BundleDeploymentOptions

	// Paused if set to true, will stop any BundleDeployments from being updated. It will be marked as out of sync.
	Paused bool `json:"paused,omitempty"`

	// RolloutStrategy controls the rollout of bundles, by defining
	// partitions, canaries and percentages for cluster availability.
	RolloutStrategy *RolloutStrategy `json:"rolloutStrategy,omitempty"`

	// Resources contains the resources that were read from the bundle's
	// path. This includes the content of downloaded helm charts.
	Resources []BundleResource `json:"resources,omitempty"`

	// Targets refer to the clusters which will be deployed to.
	Targets []BundleTarget `json:"targets,omitempty"`

	// TargetRestrictions restrict which clusters the bundle will be deployed to.
	TargetRestrictions []BundleTargetRestriction `json:"targetRestrictions,omitempty"`

	// DependsOn refers to the bundles which must be ready before this bundle can be deployed.
	DependsOn []BundleRef `json:"dependsOn,omitempty"`
}

type BundleRef struct {
	Name     string                `json:"name,omitempty"`
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

type BundleResource struct {
	Name     string `json:"name,omitempty"`
	Content  string `json:"content,omitempty"`
	Encoding string `json:"encoding,omitempty"`
}

type RolloutStrategy struct {
	MaxUnavailable           *intstr.IntOrString `json:"maxUnavailable,omitempty"`
	MaxUnavailablePartitions *intstr.IntOrString `json:"maxUnavailablePartitions,omitempty"`
	AutoPartitionSize        *intstr.IntOrString `json:"autoPartitionSize,omitempty"`
	Partitions               []Partition         `json:"partitions,omitempty"`
}

type Partition struct {
	Name                 string                `json:"name,omitempty"`
	MaxUnavailable       *intstr.IntOrString   `json:"maxUnavailable,omitempty"`
	ClusterName          string                `json:"clusterName,omitempty"`
	ClusterSelector      *metav1.LabelSelector `json:"clusterSelector,omitempty"`
	ClusterGroup         string                `json:"clusterGroup,omitempty"`
	ClusterGroupSelector *metav1.LabelSelector `json:"clusterGroupSelector,omitempty"`
}

type BundleTargetRestriction struct {
	Name                 string                `json:"name,omitempty"`
	ClusterName          string                `json:"clusterName,omitempty"`
	ClusterSelector      *metav1.LabelSelector `json:"clusterSelector,omitempty"`
	ClusterGroup         string                `json:"clusterGroup,omitempty"`
	ClusterGroupSelector *metav1.LabelSelector `json:"clusterGroupSelector,omitempty"`
}

type BundleTarget struct {
	BundleDeploymentOptions
	Name                 string                `json:"name,omitempty"`
	ClusterName          string                `json:"clusterName,omitempty"`
	ClusterSelector      *metav1.LabelSelector `json:"clusterSelector,omitempty"`
	ClusterGroup         string                `json:"clusterGroup,omitempty"`
	ClusterGroupSelector *metav1.LabelSelector `json:"clusterGroupSelector,omitempty"`
	DoNotDeploy          bool                  `json:"doNotDeploy,omitempty"`
}

type BundleSummary struct {
	NotReady          int                `json:"notReady,omitempty"`
	WaitApplied       int                `json:"waitApplied,omitempty"`
	ErrApplied        int                `json:"errApplied,omitempty"`
	OutOfSync         int                `json:"outOfSync,omitempty"`
	Modified          int                `json:"modified,omitempty"`
	Ready             int                `json:"ready"`
	Pending           int                `json:"pending,omitempty"`
	DesiredReady      int                `json:"desiredReady"`
	NonReadyResources []NonReadyResource `json:"nonReadyResources,omitempty"`
}

type NonReadyResource struct {
	Name           string           `json:"name,omitempty"`
	State          BundleState      `json:"bundleState,omitempty"`
	Message        string           `json:"message,omitempty"`
	ModifiedStatus []ModifiedStatus `json:"modifiedStatus,omitempty"`
	NonReadyStatus []NonReadyStatus `json:"nonReadyStatus,omitempty"`
}

var (
	BundleConditionReady               = "Ready"
	BundleDeploymentConditionReady     = "Ready"
	BundleDeploymentConditionInstalled = "Installed"
	BundleDeploymentConditionDeployed  = "Deployed"
)

type BundleStatus struct {
	Conditions []genericcondition.GenericCondition `json:"conditions,omitempty"`

	Summary                  BundleSummary     `json:"summary,omitempty"`
	NewlyCreated             int               `json:"newlyCreated,omitempty"`
	Unavailable              int               `json:"unavailable"`
	UnavailablePartitions    int               `json:"unavailablePartitions"`
	MaxUnavailable           int               `json:"maxUnavailable"`
	MaxUnavailablePartitions int               `json:"maxUnavailablePartitions"`
	MaxNew                   int               `json:"maxNew,omitempty"`
	PartitionStatus          []PartitionStatus `json:"partitions,omitempty"`
	Display                  BundleDisplay     `json:"display,omitempty"`
	// ResourceKey lists resources, which will likely be deployed. The
	// actual list of resources on a cluster might differ, depending on the
	// helm chart, value templating, etc..
	ResourceKey        []ResourceKey `json:"resourceKey,omitempty"`
	ObservedGeneration int64         `json:"observedGeneration"`
}

type ResourceKey struct {
	Kind       string `json:"kind,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
}

type BundleDisplay struct {
	ReadyClusters string `json:"readyClusters,omitempty"`
	State         string `json:"state,omitempty"`
}

type PartitionStatus struct {
	Name           string        `json:"name,omitempty"`
	Count          int           `json:"count,omitempty"`
	MaxUnavailable int           `json:"maxUnavailable,omitempty"`
	Unavailable    int           `json:"unavailable,omitempty"`
	Summary        BundleSummary `json:"summary,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BundleDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BundleDeploymentSpec   `json:"spec,omitempty"`
	Status BundleDeploymentStatus `json:"status,omitempty"`
}

type BundleDeploymentOptions struct {
	// DefaultNamespace is the namespace to use for resources that do not
	// specify a namespace. This field is not used to enforce or lock down
	// the deployment to a specific namespace.
	DefaultNamespace string `json:"defaultNamespace,omitempty"`

	// TargetNamespace if present will assign all resource to this
	// namespace and if any cluster scoped resource exists the deployment
	// will fail.
	TargetNamespace string `json:"namespace,omitempty"`

	// Kustomize options for the deployment, like the dir containing the
	// kustomization.yaml file.
	Kustomize *KustomizeOptions `json:"kustomize,omitempty"`

	// Helm options for the deployment, like the chart name, repo and values.
	Helm *HelmOptions `json:"helm,omitempty"`

	// ServiceAccount which will be used to perform this deployment.
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// ForceSyncGeneration is used to force a redeployment
	ForceSyncGeneration int64 `json:"forceSyncGeneration,omitempty"`

	// YAML options, if using raw YAML these are names that map to
	// overlays/{name} that will be used to replace or patch a resource.
	YAML *YAMLOptions `json:"yaml,omitempty"`

	// Diff can be used to ignore the modified state of objects which are amended at runtime.
	Diff *DiffOptions `json:"diff,omitempty"`

	// KeepResources can be used to keep the deployed resources when removing the bundle
	KeepResources bool `json:"keepResources,omitempty"`

	//IgnoreOptions can be used to ignore fields when monitoring the bundle.
	IgnoreOptions `json:"ignore,omitempty"`

	// CorrectDrift specifies how drift correction should work.
	CorrectDrift CorrectDrift `json:"correctDrift,omitempty"`

	// NamespaceLabels are labels that will be appended to the namespace created by Fleet.
	NamespaceLabels *map[string]string `json:"namespaceLabels,omitempty"`

	// NamespaceAnnotations are annotations that will be appended to the namespace created by Fleet.
	NamespaceAnnotations *map[string]string `json:"namespaceAnnotations,omitempty"`
}

type DiffOptions struct {
	ComparePatches []ComparePatch `json:"comparePatches,omitempty"`
}

type ComparePatch struct {
	Kind         string      `json:"kind,omitempty"`
	APIVersion   string      `json:"apiVersion,omitempty"`
	Namespace    string      `json:"namespace,omitempty"`
	Name         string      `json:"name,omitempty"`
	Operations   []Operation `json:"operations,omitempty"`
	JsonPointers []string    `json:"jsonPointers,omitempty"`
}

type Operation struct {
	Op    string `json:"op,omitempty"`
	Path  string `json:"path,omitempty"`
	Value string `json:"value,omitempty"`
}

type YAMLOptions struct {
	Overlays []string `json:"overlays,omitempty"`
}

type KustomizeOptions struct {
	Dir string `json:"dir,omitempty"`
}

type HelmOptions struct {
	// Chart can refer to any go-getter URL or OCI registry based helm
	// chart URL. The chart will be downloaded.
	Chart string `json:"chart,omitempty"`

	// Repo is the name of the HTTPS helm repo to download the chart from.
	Repo string `json:"repo,omitempty"`

	// ReleaseName sets a custom release name to deploy the chart as. If
	// not specified a release name will be generated by combining the
	// invoking GitRepo.name + GitRepo.path.
	ReleaseName string `json:"releaseName,omitempty"`

	// Version of the chart to download
	Version string `json:"version,omitempty"`

	// TimeoutSeconds is the time to wait for Helm operations.
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`

	// Values passed to Helm. It is possible to specify the keys and values
	// as go template strings.
	Values *GenericMap `json:"values,omitempty"`

	// ValuesFrom loads the values from configmaps and secrets.
	ValuesFrom []ValuesFrom `json:"valuesFrom,omitempty"`

	// Force allows to override immutable resources. This could be dangerous.
	Force bool `json:"force,omitempty"`

	// TakeOwnership makes helm skip the check for its own annotations
	TakeOwnership bool `json:"takeOwnership,omitempty"`

	// MaxHistory limits the maximum number of revisions saved per release by Helm.
	MaxHistory int `json:"maxHistory,omitempty"`

	// ValuesFiles is a list of files to load values from.
	ValuesFiles []string `json:"valuesFiles,omitempty"`

	// WaitForJobs if set and timeoutSeconds provided, will wait until all
	// Jobs have been completed before marking the GitRepo as ready. It
	// will wait for as long as timeoutSeconds
	WaitForJobs bool `json:"waitForJobs,omitempty"`

	// Atomic sets the --atomic flag when Helm is performing an upgrade
	Atomic bool `json:"atomic,omitempty"`

	// DisablePreProcess disables template processing in values
	DisablePreProcess bool `json:"disablePreProcess,omitempty"`
}

// IgnoreOptions defines conditions to be ignored when monitoring the Bundle.
type IgnoreOptions struct {
	Conditions []map[string]string `json:"conditions,omitempty"`
}

// Define helm values that can come from configmap, secret or external. Credit: https://github.com/fluxcd/helm-operator/blob/0cfea875b5d44bea995abe7324819432070dfbdc/pkg/apis/helm.fluxcd.io/v1/types_helmrelease.go#L439
type ValuesFrom struct {
	// The reference to a config map with release values.
	// +optional
	ConfigMapKeyRef *ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
	// The reference to a secret with release values.
	// +optional
	SecretKeyRef *SecretKeySelector `json:"secretKeyRef,omitempty"`
}

type ConfigMapKeySelector struct {
	LocalObjectReference `json:",inline"`
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// +optional
	Key string `json:"key,omitempty"`
}

type SecretKeySelector struct {
	LocalObjectReference `json:",inline"`
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// +optional
	Key string `json:"key,omitempty"`
}

type LocalObjectReference struct {
	Name string `json:"name"`
}

type BundleDeploymentSpec struct {
	Paused             bool                    `json:"paused,omitempty"`
	StagedOptions      BundleDeploymentOptions `json:"stagedOptions,omitempty"`
	StagedDeploymentID string                  `json:"stagedDeploymentID,omitempty"`
	Options            BundleDeploymentOptions `json:"options,omitempty"`
	DeploymentID       string                  `json:"deploymentID,omitempty"`
	DependsOn          []BundleRef             `json:"dependsOn,omitempty"`
	CorrectDrift       CorrectDrift            `json:"correctDrift,omitempty"`
}

type BundleDeploymentResource struct {
	Kind       string      `json:"kind,omitempty"`
	APIVersion string      `json:"apiVersion,omitempty"`
	Namespace  string      `json:"namespace,omitempty"`
	Name       string      `json:"name,omitempty"`
	CreatedAt  metav1.Time `json:"createdAt,omitempty"`
}

type BundleDeploymentStatus struct {
	Conditions          []genericcondition.GenericCondition `json:"conditions,omitempty"`
	AppliedDeploymentID string                              `json:"appliedDeploymentID,omitempty"`
	Release             string                              `json:"release,omitempty"`
	Ready               bool                                `json:"ready,omitempty"`
	NonModified         bool                                `json:"nonModified,omitempty"`
	NonReadyStatus      []NonReadyStatus                    `json:"nonReadyStatus,omitempty"`
	ModifiedStatus      []ModifiedStatus                    `json:"modifiedStatus,omitempty"`
	Display             BundleDeploymentDisplay             `json:"display,omitempty"`
	SyncGeneration      *int64                              `json:"syncGeneration,omitempty"`
	// Resources lists the metadata of resources that were deployed
	// according to the helm release history.
	Resources []BundleDeploymentResource `json:"resources,omitempty"`
}

type BundleDeploymentDisplay struct {
	Deployed  string `json:"deployed,omitempty"`
	Monitored string `json:"monitored,omitempty"`
	State     string `json:"state,omitempty"`
}

type NonReadyStatus struct {
	UID        types.UID       `json:"uid,omitempty"`
	Kind       string          `json:"kind,omitempty"`
	APIVersion string          `json:"apiVersion,omitempty"`
	Namespace  string          `json:"namespace,omitempty"`
	Name       string          `json:"name,omitempty"`
	Summary    summary.Summary `json:"summary,omitempty"`
}

func (in NonReadyStatus) String() string {
	return name(in.APIVersion, in.Kind, in.Namespace, in.Name) + " " + in.Summary.String()
}

func name(apiVersion, kind, namespace, name string) string {
	if apiVersion == "" {
		if namespace == "" {
			return fmt.Sprintf("%s %s", strings.ToLower(kind), name)
		}
		return fmt.Sprintf("%s %s/%s", strings.ToLower(kind), namespace, name)
	}
	if namespace == "" {
		return fmt.Sprintf("%s.%s %s", strings.ToLower(kind), strings.SplitN(apiVersion, "/", 2)[0], name)
	}
	return fmt.Sprintf("%s.%s %s/%s", strings.ToLower(kind), strings.SplitN(apiVersion, "/", 2)[0], namespace, name)
}

type ModifiedStatus struct {
	Kind       string `json:"kind,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
	Create     bool   `json:"missing,omitempty"`
	Delete     bool   `json:"delete,omitempty"`
	Patch      string `json:"patch,omitempty"`
}

func (in ModifiedStatus) String() string {
	msg := name(in.APIVersion, in.Kind, in.Namespace, in.Name)
	if in.Create {
		return msg + " missing"
	} else if in.Delete {
		return msg + " extra"
	}
	return msg + " modified " + in.Patch
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Content struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Content []byte `json:"content,omitempty"`
}
