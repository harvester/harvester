// Copyright © 2020 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package typeoverride

import (
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Types in this package are the structural equivalent of their original Kubernetes counterparts with the difference,
// that required fields are declared as optional using `omitempty` or in certain cases some fields are left off.
//
// The purpose of this is that now these types can be embedded into CRDs and can be used to provide a convenient override
// mechanism for resources created inside an operator.
//
// For an example see tests in https://github.com/cisco-open/operator-tools/tree/master/pkg/merge

// +kubebuilder:object:generate=true

// ObjectMeta contains only a [subset of the fields included in k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#objectmeta-v1-meta).
type ObjectMeta struct {
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// Merge merges it's receivers values into the incoming ObjectMeta by overwriting values for existing keys and adding new ones.
func (override *ObjectMeta) Merge(meta metav1.ObjectMeta) metav1.ObjectMeta {
	if override == nil {
		return meta
	}
	if len(override.Annotations) > 0 {
		if meta.Annotations == nil {
			meta.Annotations = make(map[string]string)
		}
		for key, val := range override.Annotations {
			meta.Annotations[key] = val
		}
	}
	if len(override.Labels) > 0 {
		if meta.Labels == nil {
			meta.Labels = make(map[string]string)
		}
		for key, val := range override.Labels {
			meta.Labels[key] = val
		}
	}
	return meta
}

// +kubebuilder:object:generate=true

// Service is a subset of [Service in k8s.io/api/apps/v1](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#service-v1-core) for embedding.
type Service struct {
	ObjectMeta ObjectMeta `json:"metadata,omitempty"`
	// Kubernetes [Service Specification](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#servicespec-v1-core)
	Spec v1.ServiceSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:generate=true

// IngressExtensionsV1beta1 is a subset of Ingress k8s.io/api/extensions/v1beta1 but is already deprecated
type IngressExtensionsV1beta1 struct {
	ObjectMeta `json:"metadata,omitempty"`
	Spec       v1beta1.IngressSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:generate=true

// IngressExtensionsV1beta1 is a subset of [Ingress in k8s.io/api/networking/v1beta1](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#ingress-v1-networking-k8s-io).
type IngressNetworkingV1beta1 struct {
	ObjectMeta `json:"metadata,omitempty"`
	// Kubernetes [Ingress Specification](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#ingressclassspec-v1-networking-k8s-io)
	Spec networkingv1beta1.IngressSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:generate=true

// DaemonSet is a subset of [DaemonSet in k8s.io/api/apps/v1](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#daemonset-v1-apps), with [DaemonSetSpec replaced by the local variant](#daemonset-spec).
type DaemonSet struct {
	ObjectMeta `json:"metadata,omitempty"`
	// [Local DaemonSet specification](#daemonset-spec)
	Spec DaemonSetSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:generate=true

// DaemonSetSpec is a subset of [DaemonSetSpec in k8s.io/api/apps/v1](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#daemonsetspec-v1-apps) but with required fields declared as optional
// and [PodTemplateSpec replaced by the local variant](#podtemplatespec).
type DaemonSetSpec struct {
	// A label query over pods that are managed by the daemon set.
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// An object that describes the pod that will be created. Note that this is a [local PodTemplateSpec](#podtemplatespec)
	Template PodTemplateSpec `json:"template,omitempty"`

	// An update strategy to replace existing DaemonSet pods with new pods.
	UpdateStrategy appsv1.DaemonSetUpdateStrategy `json:"updateStrategy,omitempty"`

	// The minimum number of seconds for which a newly created DaemonSet pod should
	// be ready without any of its container crashing, for it to be considered
	// available. Defaults to 0 (pod will be considered available as soon as it
	// is ready). (default: 0)
	MinReadySeconds int32 `json:"minReadySeconds,omitempty"`

	// The number of old history to retain to allow rollback.
	// This is a pointer to distinguish between explicit zero and not specified.
	// Defaults to 10. (default: 10)
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// +kubebuilder:object:generate=true

// Deployment is a subset of [Deployment in k8s.io/api/apps/v1](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#deployment-v1-apps), with [DeploymentSpec replaced by the local variant](#deployment-spec).
type Deployment struct {
	ObjectMeta `json:"metadata,omitempty"`
	// The desired behavior of [this deployment](#deploymentspec).
	Spec DeploymentSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:generate=true

// DeploymentSpec is a subset of [DeploymentSpec in k8s.io/api/apps/v1](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#deploymentspec-v1-apps) but with required fields declared as optional
// and [PodTemplateSpec replaced by the local variant](#podtemplatespec).
type DeploymentSpec struct {
	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1. (default: 1)
	Replicas *int32 `json:"replicas,omitempty"`

	// Label selector for pods. Existing ReplicaSets whose pods are
	// selected by this will be the ones affected by this deployment.
	// It must match the pod template's labels.
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// An object that describes the pod that will be created. Note that this is a [local PodTemplateSpec](#podtemplatespec)
	Template PodTemplateSpec `json:"template,omitempty"`

	// The deployment strategy to use to replace existing pods with new ones.
	// +patchStrategy=retainKeys
	Strategy appsv1.DeploymentStrategy `json:"strategy,omitempty"`

	// Minimum number of seconds for which a newly created pod should be ready
	// without any of its container crashing, for it to be considered available.
	// Defaults to 0 (pod will be considered available as soon as it is ready) (default: 0)
	MinReadySeconds int32 `json:"minReadySeconds,omitempty"`

	// The number of old ReplicaSets to retain to allow rollback.
	// This is a pointer to distinguish between explicit zero and not specified.
	// Defaults to 10. (default: 10)
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// Indicates that the deployment is paused.
	Paused bool `json:"paused,omitempty"`

	// The maximum time in seconds for a deployment to make progress before it
	// is considered to be failed. The deployment controller will continue to
	// process failed deployments and a condition with a ProgressDeadlineExceeded
	// reason will be surfaced in the deployment status. Note that progress will
	// not be estimated during the time a deployment is paused.
	// Defaults to 600s. (default: 600)
	ProgressDeadlineSeconds *int32 `json:"progressDeadlineSeconds,omitempty"`
}

// +kubebuilder:object:generate=true

// StatefulSet is a subset of [StatefulSet in k8s.io/api/apps/v1](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#statefulset-v1-apps), with [StatefulSetSpec replaced by the local variant](#statefulset-spec).
type StatefulSet struct {
	ObjectMeta `json:"metadata,omitempty"`
	Spec       StatefulSetSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:generate=true

// StatefulSetSpec is a subset of [StatefulSetSpec in k8s.io/api/apps/v1](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#statefulsetspec-v1-apps) but with required fields declared as optional
// and [PodTemplateSpec](#podtemplatespec) and [PersistentVolumeClaim replaced by the local variant](#persistentvolumeclaim).
type StatefulSetSpec struct {
	// Replicas is the desired number of replicas of the given Template.
	// These are replicas in the sense that they are instantiations of the
	// same Template, but individual replicas also have a consistent identity.
	// If unspecified, defaults to 1. (default: 1)
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Selector is a label query over pods that should match the replica count.
	// It must match the pod template's labels.
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// template is the object that describes the pod that will be created if
	// insufficient replicas are detected. Each pod stamped out by the StatefulSet
	// will fulfill this Template, but have a unique identity from the rest
	// of the StatefulSet.
	Template PodTemplateSpec `json:"template,omitempty"`

	// volumeClaimTemplates is a list of claims that pods are allowed to reference.
	// The StatefulSet controller is responsible for mapping network identities to
	// claims in a way that maintains the identity of a pod. Every claim in
	// this list must have at least one matching (by name) volumeMount in one
	// container in the template. A claim in this list takes precedence over
	// any volumes in the template, with the same name.
	// +optional
	VolumeClaimTemplates []PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`

	// serviceName is the name of the service that governs this StatefulSet.
	// This service must exist before the StatefulSet, and is responsible for
	// the network identity of the set. Pods get DNS/hostnames that follow the
	// pattern: pod-specific-string.serviceName.default.svc.cluster.local
	// where "pod-specific-string" is managed by the StatefulSet controller.
	ServiceName string `json:"serviceName,omitempty"`

	// podManagementPolicy controls how pods are created during initial scale up,
	// when replacing pods on nodes, or when scaling down. The default policy is
	// `OrderedReady`, where pods are created in increasing order (pod-0, then
	// pod-1, etc) and the controller will wait until each pod is ready before
	// continuing. When scaling down, the pods are removed in the opposite order.
	// The alternative policy is `Parallel` which will create pods in parallel
	// to match the desired scale without waiting, and on scale down will delete
	// all pods at once.
	// (default: OrderedReady)
	// +optional
	PodManagementPolicy appsv1.PodManagementPolicyType `json:"podManagementPolicy,omitempty"`

	// updateStrategy indicates the StatefulSetUpdateStrategy that will be
	// employed to update Pods in the StatefulSet when a revision is made to
	// Template.
	UpdateStrategy appsv1.StatefulSetUpdateStrategy `json:"updateStrategy,omitempty"`

	// revisionHistoryLimit is the maximum number of revisions that will
	// be maintained in the StatefulSet's revision history. The revision history
	// consists of all revisions not represented by a currently applied
	// StatefulSetSpec version. The default value is 10. (default: 10)
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// +kubebuilder:object:generate=true

// PersistentVolumeClaim is a subset of [PersistentVolumeClaim in k8s.io/api/core/v1](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#persistentvolumeclaim-v1-core).
type PersistentVolumeClaim struct {
	EmbeddedPersistentVolumeClaimObjectMeta `json:"metadata,omitempty"`
	Spec                                    v1.PersistentVolumeClaimSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:generate=true

// ObjectMeta contains only a subset of the fields included in k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta
// Only fields which are relevant to embedded PVCs are included
// controller-gen discards embedded ObjectMetadata type fields, so we have to overcome this.
type EmbeddedPersistentVolumeClaimObjectMeta struct {
	Name        string            `json:"name,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// +kubebuilder:object:generate=true

// PodTemplateSpec describes the data a pod should have when created from a template
// It's the same as [PodTemplateSpec in k8s.io/api/core/v1](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#podtemplatespec-v1-core) but with the [local ObjectMeta](#objectmeta) and [PodSpec](#podspec) types embedded.
type PodTemplateSpec struct {
	ObjectMeta `json:"metadata,omitempty"`
	Spec       PodSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:generate=true

// PodSpec is a subset of [PodSpec in k8s.io/api/corev1](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#podspec-v1-core). It's the same as the original PodSpec expect it allows for containers to be missing.
type PodSpec struct {
	// List of volumes that can be mounted by containers belonging to the pod.
	// +patchMergeKey=name
	// +patchStrategy=merge,retainKeys
	Volumes []v1.Volume `json:"volumes,omitempty" patchStrategy:"merge,retainKeys" patchMergeKey:"name"`
	// List of initialization containers belonging to the pod.
	// +patchMergeKey=name
	// +patchStrategy=merge
	InitContainers []v1.Container `json:"initContainers,omitempty" patchStrategy:"merge" patchMergeKey:"name"`
	// List of containers belonging to the pod.
	// +patchMergeKey=name
	// +patchStrategy=merge
	Containers []v1.Container `json:"containers,omitempty" patchStrategy:"merge" patchMergeKey:"name" `
	// List of ephemeral containers run in this pod.
	// +patchMergeKey=name
	// +patchStrategy=merge
	EphemeralContainers []v1.EphemeralContainer `json:"ephemeralContainers,omitempty" patchStrategy:"merge" patchMergeKey:"name"`
	// Restart policy for all containers within the pod.
	// One of Always, OnFailure, Never.
	// Default to Always. (default: Always)
	RestartPolicy v1.RestartPolicy `json:"restartPolicy,omitempty"`
	// Optional duration in seconds the pod needs to terminate gracefully.
	// Defaults to 30 seconds. (default: 30)
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`
	// Optional duration in seconds the pod may be active on the node relative to
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`
	// Set DNS policy for the pod.
	// Defaults to "ClusterFirst". (default: ClusterFirst)
	// Valid values are 'ClusterFirstWithHostNet', 'ClusterFirst', 'Default' or 'None'.
	DNSPolicy v1.DNSPolicy `json:"dnsPolicy,omitempty"`
	// NodeSelector is a selector which must be true for the pod to fit on a node.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// ServiceAccountName is the name of the ServiceAccount to use to run this pod.
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// AutomountServiceAccountToken indicates whether a service account token should be automatically mounted.
	AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty"`
	// NodeName is a request to schedule this pod onto a specific node.
	NodeName string `json:"nodeName,omitempty"`
	// Host networking requested for this pod. Use the host's network namespace.
	// If this option is set, the ports that will be used must be specified.
	// Default to false. (default: false)
	HostNetwork bool `json:"hostNetwork,omitempty"`
	// Use the host's pid namespace.
	// Optional: Default to false. (default: false)
	HostPID bool `json:"hostPID,omitempty"`
	// Use the host's ipc namespace.
	// Optional: Default to false. (default: false)
	HostIPC bool `json:"hostIPC,omitempty"`
	// Share a single process namespace between all of the containers in a pod.
	// HostPID and ShareProcessNamespace cannot both be set.
	// Optional: Default to false. (default: false)
	ShareProcessNamespace *bool `json:"shareProcessNamespace,omitempty"`
	// SecurityContext holds pod-level security attributes and common container settings.
	// Optional: Defaults to empty.  See type description for default values of each field.
	SecurityContext *v1.PodSecurityContext `json:"securityContext,omitempty"`
	// ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the images used by this PodSpec.
	// +patchMergeKey=name
	// +patchStrategy=merge
	ImagePullSecrets []v1.LocalObjectReference `json:"imagePullSecrets,omitempty" patchStrategy:"merge" patchMergeKey:"name"`
	// Specifies the hostname of the Pod
	// If not specified, the pod's hostname will be set to a system-defined value.
	Hostname string `json:"hostname,omitempty"`
	// If specified, the fully qualified Pod hostname will be "<hostname>.<subdomain>.<pod namespace>.svc.<cluster domain>".
	// If not specified, the pod will not have a domainname at all.
	Subdomain string `json:"subdomain,omitempty"`
	// If specified, the pod's scheduling constraints
	Affinity *v1.Affinity `json:"affinity,omitempty"`
	// If specified, the pod will be dispatched by specified scheduler.
	// If not specified, the pod will be dispatched by default scheduler.
	SchedulerName string `json:"schedulerName,omitempty"`
	// If specified, the pod's tolerations.
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
	// HostAliases is an optional list of hosts and IPs that will be injected into the pod's hosts
	// file if specified. This is only valid for non-hostNetwork pods.
	// +patchMergeKey=ip
	// +patchStrategy=merge
	HostAliases []v1.HostAlias `json:"hostAliases,omitempty" patchStrategy:"merge" patchMergeKey:"ip"`
	// If specified, indicates the pod's priority.
	PriorityClassName string `json:"priorityClassName,omitempty"`
	// The priority value. Various system components use this field to find the
	// priority of the pod.
	Priority *int32 `json:"priority,omitempty"`
	// Specifies the DNS parameters of a pod.
	DNSConfig *v1.PodDNSConfig `json:"dnsConfig,omitempty"`
	// If specified, all readiness gates will be evaluated for pod readiness.
	ReadinessGates []v1.PodReadinessGate `json:"readinessGates,omitempty"`
	// RuntimeClassName refers to a RuntimeClass object in the node.k8s.io group, which should be used
	// to run this pod.
	RuntimeClassName *string `json:"runtimeClassName,omitempty"`
	// EnableServiceLinks indicates whether information about services should be injected into pod's
	// environment variables, matching the syntax of Docker links.
	// Optional: Defaults to true. (default: true)
	EnableServiceLinks *bool `json:"enableServiceLinks,omitempty"`
	// PreemptionPolicy is the Policy for preempting pods with lower priority.
	// One of Never, PreemptLowerPriority.
	// Defaults to PreemptLowerPriority if unset. (default: PreemptLowerPriority)
	PreemptionPolicy *v1.PreemptionPolicy `json:"preemptionPolicy,omitempty"`
	// Overhead represents the resource overhead associated with running a pod for a given RuntimeClass.
	Overhead v1.ResourceList `json:"overhead,omitempty"`
	// TopologySpreadConstraints describes how a group of pods ought to spread across topology
	// domains.
	// +patchMergeKey=topologyKey
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=topologyKey
	// +listMapKey=whenUnsatisfiable
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty" patchStrategy:"merge" patchMergeKey:"topologyKey"`
	// If true the pod's hostname will be configured as the pod's FQDN, rather than the leaf name (the default).
	// Default to false. (default: false)
	// +optional
	SetHostnameAsFQDN *bool `json:"setHostnameAsFQDN,omitempty"`
}

// +kubebuilder:object:generate=true

// ServiceAccount is a subset of [ServiceAccount in k8s.io/api/core/v1](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#serviceaccount-v1-core).
type ServiceAccount struct {
	// +optional
	ObjectMeta `json:"metadata,omitempty"`

	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	Secrets []v1.ObjectReference `json:"secrets,omitempty" patchStrategy:"merge" patchMergeKey:"name"`

	// +optional
	ImagePullSecrets []v1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// +optional
	AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty"`
}
