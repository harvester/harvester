package v1alpha1

import (
	"fmt"
	"strings"

	"github.com/rancher/wrangler/v3/pkg/genericcondition"
	"github.com/rancher/wrangler/v3/pkg/summary"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const BundleDeploymentResourceNamePlural = "bundledeployments"

func init() {
	InternalSchemeBuilder.Register(&BundleDeployment{}, &BundleDeploymentList{})
}

// MaxHelmReleaseNameLen is the maximum length of a Helm release name.
// See https://github.com/helm/helm/blob/293b50c65d4d56187cd4e2f390f0ada46b4c4737/pkg/chartutil/validate_name.go#L54-L61
const MaxHelmReleaseNameLen = 53

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Deployed",type=string,JSONPath=`.status.display.deployed`
// +kubebuilder:printcolumn:name="Monitored",type=string,JSONPath=`.status.display.monitored`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`

// BundleDeployment is used internally by Fleet and should not be used directly.
// When a Bundle is deployed to a cluster an instance of a Bundle is called a
// BundleDeployment. A BundleDeployment represents the state of that Bundle on
// a specific cluster with its cluster-specific customizations. The Fleet agent
// is only aware of BundleDeployment resources that are created for the cluster
// the agent is managing.
type BundleDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BundleDeploymentSpec   `json:"spec,omitempty"`
	Status BundleDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BundleDeploymentList contains a list of BundleDeployment
type BundleDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BundleDeployment `json:"items"`
}

type BundleDeploymentOptions struct {
	// DefaultNamespace is the namespace to use for resources that do not
	// specify a namespace. This field is not used to enforce or lock down
	// the deployment to a specific namespace.
	// +nullable
	DefaultNamespace string `json:"defaultNamespace,omitempty"`

	// TargetNamespace if present will assign all resource to this
	// namespace and if any cluster scoped resource exists the deployment
	// will fail.
	// +nullable
	TargetNamespace string `json:"namespace,omitempty"`

	// Kustomize options for the deployment, like the dir containing the
	// kustomization.yaml file.
	// +nullable
	Kustomize *KustomizeOptions `json:"kustomize,omitempty"`

	// Helm options for the deployment, like the chart name, repo and values.
	// +nullable
	Helm *HelmOptions `json:"helm,omitempty"`

	// ServiceAccount which will be used to perform this deployment.
	// +nullable
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// ForceSyncGeneration is used to force a redeployment
	ForceSyncGeneration int64 `json:"forceSyncGeneration,omitempty"`

	// YAML options, if using raw YAML these are names that map to
	// overlays/{name} files that will be used to replace or patch a resource.
	// +nullable
	YAML *YAMLOptions `json:"yaml,omitempty"`

	// Diff can be used to ignore the modified state of objects which are amended at runtime.
	// +nullable
	Diff *DiffOptions `json:"diff,omitempty"`

	// KeepResources can be used to keep the deployed resources when removing the bundle
	KeepResources bool `json:"keepResources,omitempty"`

	// DeleteNamespace can be used to delete the deployed namespace when removing the bundle
	DeleteNamespace bool `json:"deleteNamespace,omitempty"`

	//IgnoreOptions can be used to ignore fields when monitoring the bundle.
	IgnoreOptions `json:"ignore,omitempty"`

	// CorrectDrift specifies how drift correction should work.
	CorrectDrift *CorrectDrift `json:"correctDrift,omitempty"`

	// NamespaceLabels are labels that will be appended to the namespace created by Fleet.
	// +nullable
	NamespaceLabels *map[string]string `json:"namespaceLabels,omitempty"`

	// NamespaceAnnotations are annotations that will be appended to the namespace created by Fleet.
	// +nullable
	NamespaceAnnotations *map[string]string `json:"namespaceAnnotations,omitempty"`

	// DeleteCRDResources deletes CRDs. Warning! this will also delete all your Custom Resources.
	DeleteCRDResources bool `json:"deleteCRDResources,omitempty"`
}

type DiffOptions struct {
	// ComparePatches match a resource and remove fields from the check for modifications.
	// +nullable
	ComparePatches []ComparePatch `json:"comparePatches,omitempty"`
}

// ComparePatch matches a resource and removes fields from the check for modifications.
type ComparePatch struct {
	// Kind is the kind of the resource to match.
	// +nullable
	Kind string `json:"kind,omitempty"`
	// APIVersion is the apiVersion of the resource to match.
	// +nullable
	APIVersion string `json:"apiVersion,omitempty"`
	// Namespace is the namespace of the resource to match.
	// +nullable
	Namespace string `json:"namespace,omitempty"`
	// Name is the name of the resource to match.
	// +nullable
	Name string `json:"name,omitempty"`
	// Operations remove a JSON path from the resource.
	// +nullable
	Operations []Operation `json:"operations,omitempty"`
	// JSONPointers ignore diffs at a certain JSON path.
	// +nullable
	JsonPointers []string `json:"jsonPointers,omitempty"`
}

// Operation of a ComparePatch, usually "remove".
type Operation struct {
	// Op is usually "remove"
	// +nullable
	Op string `json:"op,omitempty"`
	// Path is the JSON path to remove.
	// +nullable
	Path string `json:"path,omitempty"`
	// Value is usually empty.
	// +nullable
	Value string `json:"value,omitempty"`
}

// YAMLOptions, if using raw YAML these are names that map to
// overlays/{name} files that will be used to replace or patch a resource.
type YAMLOptions struct {
	// Overlays is a list of names that maps to folders in "overlays/".
	// If you wish to customize the file ./subdir/resource.yaml then a file
	// ./overlays/myoverlay/subdir/resource.yaml will replace the base
	// file.
	// A file named ./overlays/myoverlay/subdir/resource_patch.yaml will patch the base file.
	// +nullable
	Overlays []string `json:"overlays,omitempty"`
}

// KustomizeOptions for a deployment.
type KustomizeOptions struct {
	// Dir points to a custom folder for kustomize resources. This folder must contain
	// a kustomization.yaml file.
	// +nullable
	Dir string `json:"dir,omitempty"`
}

// HelmOptions for the deployment. For Helm-based bundles, all options can be
// used, otherwise some options are ignored. For example ReleaseName works with
// all bundle types.
type HelmOptions struct {
	// Chart can refer to any go-getter URL or OCI registry based helm
	// chart URL. The chart will be downloaded.
	// +nullable
	Chart string `json:"chart,omitempty"`

	// +nullable
	// Repo is the name of the HTTPS helm repo to download the chart from.
	Repo string `json:"repo,omitempty"`

	// ReleaseName sets a custom release name to deploy the chart as. If
	// not specified a release name will be generated by combining the
	// invoking GitRepo.name + GitRepo.path.
	// +nullable
	// +kubebuilder:validation:MaxLength=53
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$
	ReleaseName string `json:"releaseName,omitempty"`

	// +nullable
	// Version of the chart to download
	Version string `json:"version,omitempty"`

	// TimeoutSeconds is the time to wait for Helm operations.
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`

	// Values passed to Helm. It is possible to specify the keys and values
	// as go template strings.
	// +nullable
	// +kubebuilder:validation:XPreserveUnknownFields
	Values *GenericMap `json:"values,omitempty"`

	// +nullable
	// ValuesFrom loads the values from configmaps and secrets.
	ValuesFrom []ValuesFrom `json:"valuesFrom,omitempty"`

	// Force allows to override immutable resources. This could be dangerous.
	Force bool `json:"force,omitempty"`

	// TakeOwnership makes helm skip the check for its own annotations
	TakeOwnership bool `json:"takeOwnership,omitempty"`

	// MaxHistory limits the maximum number of revisions saved per release by Helm.
	MaxHistory int `json:"maxHistory,omitempty"`

	// ValuesFiles is a list of files to load values from.
	// +nullable
	ValuesFiles []string `json:"valuesFiles,omitempty"`

	// WaitForJobs if set and timeoutSeconds provided, will wait until all
	// Jobs have been completed before marking the GitRepo as ready. It
	// will wait for as long as timeoutSeconds
	WaitForJobs bool `json:"waitForJobs,omitempty"`

	// Atomic sets the --atomic flag when Helm is performing an upgrade
	Atomic bool `json:"atomic,omitempty"`

	// DisablePreProcess disables template processing in values
	DisablePreProcess bool `json:"disablePreProcess,omitempty"`

	// DisableDNS can be used to customize Helm's EnableDNS option, which Fleet sets to `true` by default.
	DisableDNS bool `json:"disableDNS,omitempty"`

	// SkipSchemaValidation allows skipping schema validation against the chart values
	SkipSchemaValidation bool `json:"skipSchemaValidation,omitempty"`

	// DisableDependencyUpdate allows skipping chart dependencies update
	DisableDependencyUpdate bool `json:"disableDependencyUpdate,omitempty"`
}

// IgnoreOptions defines conditions to be ignored when monitoring the Bundle.
type IgnoreOptions struct {
	// Conditions is a list of conditions to be ignored when monitoring the Bundle.
	// +nullable
	Conditions []map[string]string `json:"conditions,omitempty"`
}

// Define helm values that can come from configmap, secret or external. Credit: https://github.com/fluxcd/helm-operator/blob/0cfea875b5d44bea995abe7324819432070dfbdc/pkg/apis/helm.fluxcd.io/v1/types_helmrelease.go#L439
type ValuesFrom struct {
	// The reference to a config map with release values.
	// +optional
	// +nullable
	ConfigMapKeyRef *ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
	// The reference to a secret with release values.
	// +optional
	// +nullable
	SecretKeyRef *SecretKeySelector `json:"secretKeyRef,omitempty"`
}

type ConfigMapKeySelector struct {
	LocalObjectReference `json:",inline"`
	// +optional
	// +nullable
	Namespace string `json:"namespace,omitempty"`
	// +optional
	// +nullable
	Key string `json:"key,omitempty"`
}

type SecretKeySelector struct {
	LocalObjectReference `json:",inline"`
	// +optional
	// +nullable
	Namespace string `json:"namespace,omitempty"`
	// +optional
	// +nullable
	Key string `json:"key,omitempty"`
}

type LocalObjectReference struct {
	// Name of a resource in the same namespace as the referent.
	// +nullable
	// +optional
	Name string `json:"name"`
}

type BundleDeploymentSpec struct {
	// Paused if set to true, will stop any BundleDeployments from being
	// updated. If true, BundleDeployments will be marked as out of sync
	// when changes are detected.
	Paused bool `json:"paused,omitempty"`
	// StagedOptions are the deployment options, that are staged for
	// the next deployment.
	StagedOptions BundleDeploymentOptions `json:"stagedOptions,omitempty"`
	// StagedDeploymentID is the ID of the staged deployment.
	// +nullable
	StagedDeploymentID string `json:"stagedDeploymentID,omitempty"`
	// Options are the deployment options, that are currently applied.
	Options BundleDeploymentOptions `json:"options,omitempty"`
	// DeploymentID is the ID of the currently applied deployment.
	// +nullable
	DeploymentID string `json:"deploymentID,omitempty"`
	// DependsOn refers to the bundles which must be ready before this bundle can be deployed.
	// +nullable
	DependsOn []BundleRef `json:"dependsOn,omitempty"`
	// CorrectDrift specifies how drift correction should work.
	CorrectDrift *CorrectDrift `json:"correctDrift,omitempty"`
	// OCIContents is true when this deployment's contents is stored in an oci registry
	OCIContents bool `json:"ociContents,omitempty"`
}

// BundleDeploymentResource contains the metadata of a deployed resource.
type BundleDeploymentResource struct {
	// +nullable
	Kind string `json:"kind,omitempty"`
	// +nullable
	APIVersion string `json:"apiVersion,omitempty"`
	// +nullable
	Namespace string `json:"namespace,omitempty"`
	// +nullable
	Name string `json:"name,omitempty"`
	// +nullable
	CreatedAt metav1.Time `json:"createdAt,omitempty"`
}

type BundleDeploymentStatus struct {
	// +nullable
	Conditions []genericcondition.GenericCondition `json:"conditions,omitempty"`
	// +nullable
	AppliedDeploymentID string `json:"appliedDeploymentID,omitempty"`
	// +nullable
	Release     string `json:"release,omitempty"`
	Ready       bool   `json:"ready,omitempty"`
	NonModified bool   `json:"nonModified,omitempty"`
	// +nullable
	NonReadyStatus []NonReadyStatus `json:"nonReadyStatus,omitempty"`
	// +nullable
	ModifiedStatus []ModifiedStatus `json:"modifiedStatus,omitempty"`
	// +nullable
	Display BundleDeploymentDisplay `json:"display,omitempty"`
	// +nullable
	SyncGeneration *int64 `json:"syncGeneration,omitempty"`
	// Resources lists the metadata of resources that were deployed
	// according to the helm release history.
	// +nullable
	Resources []BundleDeploymentResource `json:"resources,omitempty"`
}

type BundleDeploymentDisplay struct {
	// +nullable
	Deployed string `json:"deployed,omitempty"`
	// +nullable
	Monitored string `json:"monitored,omitempty"`
	// +nullable
	State string `json:"state,omitempty"`
}

// NonReadyStatus is used to report the status of a resource that is not ready. It includes a summary.
type NonReadyStatus struct {
	// +nullable
	UID types.UID `json:"uid,omitempty"`
	// +nullable
	Kind string `json:"kind,omitempty"`
	// +nullable
	APIVersion string `json:"apiVersion,omitempty"`
	// +nullable
	Namespace string `json:"namespace,omitempty"`
	// +nullable
	Name    string          `json:"name,omitempty"`
	Summary summary.Summary `json:"summary,omitempty"`
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

// ModifiedStatus is used to report the status of a resource that is modified.
// It indicates if the modification was a create, a delete or a patch.
type ModifiedStatus struct {
	// +nullable
	Kind string `json:"kind,omitempty"`
	// +nullable
	APIVersion string `json:"apiVersion,omitempty"`
	// +nullable
	Namespace string `json:"namespace,omitempty"`
	// +nullable
	Name   string `json:"name,omitempty"`
	Create bool   `json:"missing,omitempty"`
	Delete bool   `json:"delete,omitempty"`
	// +nullable
	Patch string `json:"patch,omitempty"`
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
