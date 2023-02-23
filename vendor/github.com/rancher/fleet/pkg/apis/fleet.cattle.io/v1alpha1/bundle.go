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

	Paused             bool                      `json:"paused,omitempty"`
	RolloutStrategy    *RolloutStrategy          `json:"rolloutStrategy,omitempty"`
	Resources          []BundleResource          `json:"resources,omitempty"`
	Targets            []BundleTarget            `json:"targets,omitempty"`
	TargetRestrictions []BundleTargetRestriction `json:"targetRestrictions,omitempty"`
	DependsOn          []BundleRef               `json:"dependsOn,omitempty"`
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
	ResourceKey              []ResourceKey     `json:"resourceKey,omitempty"`
	ObservedGeneration       int64             `json:"observedGeneration"`
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
	DefaultNamespace    string            `json:"defaultNamespace,omitempty"`
	TargetNamespace     string            `json:"namespace,omitempty"`
	Kustomize           *KustomizeOptions `json:"kustomize,omitempty"`
	Helm                *HelmOptions      `json:"helm,omitempty"`
	ServiceAccount      string            `json:"serviceAccount,omitempty"`
	ForceSyncGeneration int64             `json:"forceSyncGeneration,omitempty"`
	YAML                *YAMLOptions      `json:"yaml,omitempty"`
	Diff                *DiffOptions      `json:"diff,omitempty"`
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
	Chart          string       `json:"chart,omitempty"`
	Repo           string       `json:"repo,omitempty"`
	ReleaseName    string       `json:"releaseName,omitempty"`
	Version        string       `json:"version,omitempty"`
	TimeoutSeconds int          `json:"timeoutSeconds,omitempty"`
	Values         *GenericMap  `json:"values,omitempty"`
	ValuesFrom     []ValuesFrom `json:"valuesFrom,omitempty"`
	Force          bool         `json:"force,omitempty"`
	TakeOwnership  bool         `json:"takeOwnership,omitempty"`
	MaxHistory     int          `json:"maxHistory,omitempty"`
	ValuesFiles    []string     `json:"valuesFiles,omitempty"`

	// Atomic sets the --atomic flag when Helm is performing an upgrade
	Atomic bool `json:"atomic,omitempty"`
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
	StagedOptions      BundleDeploymentOptions `json:"stagedOptions,omitempty"`
	StagedDeploymentID string                  `json:"stagedDeploymentID,omitempty"`
	Options            BundleDeploymentOptions `json:"options,omitempty"`
	DeploymentID       string                  `json:"deploymentID,omitempty"`
	DependsOn          []BundleRef             `json:"dependsOn,omitempty"`
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
