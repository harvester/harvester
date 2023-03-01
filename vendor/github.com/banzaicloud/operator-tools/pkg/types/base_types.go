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

package types

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/banzaicloud/operator-tools/pkg/utils"
)

const (
	NameLabel      = "app.kubernetes.io/name"
	InstanceLabel  = "app.kubernetes.io/instance"
	VersionLabel   = "app.kubernetes.io/version"
	ComponentLabel = "app.kubernetes.io/component"
	ManagedByLabel = "app.kubernetes.io/managed-by"

	BanzaiCloudManagedComponent    = "banzaicloud.io/managed-component"
	BanzaiCloudOwnedBy             = "banzaicloud.io/owned-by"
	BanzaiCloudRelatedTo           = "banzaicloud.io/related-to"
	BanzaiCloudDesiredStateCreated = "banzaicloud.io/desired-state-created"
)

type ObjectKey struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type ReconcileStatus string

const (
	// Used for components and for aggregated status
	ReconcileStatusFailed ReconcileStatus = "Failed"

	// Used for components and for aggregated status
	ReconcileStatusReconciling ReconcileStatus = "Reconciling"

	// Used for components
	ReconcileStatusAvailable ReconcileStatus = "Available"
	ReconcileStatusUnmanaged ReconcileStatus = "Unmanaged"
	ReconcileStatusRemoved   ReconcileStatus = "Removed"

	// Used for aggregated status if all the components are stableized (Available, Unmanaged or Removed)
	ReconcileStatusSucceeded ReconcileStatus = "Succeeded"

	// Used to trigger reconciliation for a resource that otherwise ignores status changes, but listens to the Pending state
	// See PendingStatusPredicate in pkg/reconciler
	ReconcileStatusPending ReconcileStatus = "Pending"
)

func (s ReconcileStatus) Stable() bool {
	return s == ReconcileStatusUnmanaged || s == ReconcileStatusRemoved || s == ReconcileStatusAvailable
}

func (s ReconcileStatus) Available() bool {
	return s == ReconcileStatusAvailable || s == ReconcileStatusSucceeded
}

func (s ReconcileStatus) Failed() bool {
	return s == ReconcileStatusFailed
}

func (s ReconcileStatus) Pending() bool {
	return s == ReconcileStatusReconciling || s == ReconcileStatusPending
}

// Computes an aggregated state based on component statuses
func AggregatedState(componentStatuses []ReconcileStatus) ReconcileStatus {
	overallStatus := ReconcileStatusReconciling
	statusMap := make(map[ReconcileStatus]bool)
	hasUnstable := false
	for _, cs := range componentStatuses {
		if cs != "" {
			statusMap[cs] = true
		}
		if !(cs == "" || cs.Stable()) {
			hasUnstable = true
		}
	}

	if statusMap[ReconcileStatusFailed] {
		overallStatus = ReconcileStatusFailed
	} else if statusMap[ReconcileStatusReconciling] {
		overallStatus = ReconcileStatusReconciling
	}

	if !hasUnstable {
		overallStatus = ReconcileStatusSucceeded
	}
	return overallStatus
}

// +kubebuilder:object:generate=true

// EnabledComponent implements the "enabled component" pattern
// Embed this type into other component types to avoid unnecessary code duplication
// NOTE: Don't forget to annotate the embedded field with `json:",inline"` tag for controller-gen
type EnabledComponent struct {
	Enabled *bool `json:"enabled,omitempty"`
}

// IsDisabled returns true iff the component is explicitly disabled
func (ec EnabledComponent) IsDisabled() bool {
	return ec.Enabled != nil && !*ec.Enabled
}

// IsEnabled returns true iff the component is explicitly enabled
func (ec EnabledComponent) IsEnabled() bool {
	return utils.PointerToBool(ec.Enabled)
}

// IsSkipped returns true iff the component is neither enabled nor disabled explicitly
func (ec EnabledComponent) IsSkipped() bool {
	return ec.Enabled == nil
}

// +kubebuilder:object:generate=true

// Deprecated
// Consider using ObjectMeta in the typeoverrides package combined with the merge package
type MetaBase struct {
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

func (base *MetaBase) Merge(meta metav1.ObjectMeta) metav1.ObjectMeta {
	if base == nil {
		return meta
	}
	if len(base.Annotations) > 0 {
		if meta.Annotations == nil {
			meta.Annotations = make(map[string]string)
		}
		for key, val := range base.Annotations {
			meta.Annotations[key] = val
		}
	}
	if len(base.Labels) > 0 {
		if meta.Labels == nil {
			meta.Labels = make(map[string]string)
		}
		for key, val := range base.Labels {
			meta.Labels[key] = val
		}
	}
	return meta
}

// +kubebuilder:object:generate=true

// Deprecated
// Consider using PodTemplateSpec in the typeoverrides package combined with the merge package
type PodTemplateBase struct {
	Metadata *MetaBase    `json:"metadata,omitempty"`
	PodSpec  *PodSpecBase `json:"spec,omitempty"`
}

func (base *PodTemplateBase) Override(template corev1.PodTemplateSpec) corev1.PodTemplateSpec {
	if base == nil {
		return template
	}
	if base.Metadata != nil {
		template.ObjectMeta = base.Metadata.Merge(template.ObjectMeta)
	}
	if base.PodSpec != nil {
		template.Spec = base.PodSpec.Override(template.Spec)
	}
	return template
}

// +kubebuilder:object:generate=true

// Deprecated
// Consider using Container in the typeoverrides package combined with the merge package
type ContainerBase struct {
	Name            string                       `json:"name,omitempty"`
	Resources       *corev1.ResourceRequirements `json:"resources,omitempty"`
	Image           string                       `json:"image,omitempty"`
	PullPolicy      corev1.PullPolicy            `json:"pullPolicy,omitempty"`
	Command         []string                     `json:"command,omitempty"`
	VolumeMounts    []corev1.VolumeMount         `json:"volumeMounts,omitempty" patchStrategy:"merge" patchMergeKey:"mountPath" `
	SecurityContext *corev1.SecurityContext      `json:"securityContext,omitempty"`
	LivenessProbe   *corev1.Probe                `json:"livenessProbe,omitempty"`
	ReadinessProbe  *corev1.Probe                `json:"readinessProbe,omitempty"`
}

func (base *ContainerBase) Override(container corev1.Container) corev1.Container {
	if base == nil {
		return container
	}
	if base.Resources != nil {
		container.Resources = *base.Resources
	}
	if base.Image != "" {
		container.Image = base.Image
	}
	if base.PullPolicy != "" {
		container.ImagePullPolicy = base.PullPolicy
	}
	if len(base.Command) > 0 {
		container.Command = base.Command
	}
	if len(base.VolumeMounts) > 0 {
		container.VolumeMounts = base.VolumeMounts
	}
	if base.SecurityContext != nil {
		container.SecurityContext = base.SecurityContext
	}
	if base.LivenessProbe != nil {
		container.LivenessProbe = base.LivenessProbe
	}
	if base.ReadinessProbe != nil {
		container.LivenessProbe = base.LivenessProbe
	}
	return container
}

// +kubebuilder:object:generate=true

// Deprecated
// Consider using PodSpec in the typeoverrides package combined with the merge package
type PodSpecBase struct {
	Tolerations        []corev1.Toleration           `json:"tolerations,omitempty"`
	NodeSelector       map[string]string             `json:"nodeSelector,omitempty"`
	ServiceAccountName string                        `json:"serviceAccountName,omitempty"`
	Affinity           *corev1.Affinity              `json:"affinity,omitempty"`
	SecurityContext    *corev1.PodSecurityContext    `json:"securityContext,omitempty"`
	Volumes            []corev1.Volume               `json:"volumes,omitempty" patchStrategy:"merge" patchMergeKey:"name"`
	PriorityClassName  string                        `json:"priorityClassName,omitempty"`
	Containers         []ContainerBase               `json:"containers,omitempty" patchStrategy:"merge" patchMergeKey:"name"`
	InitContainers     []ContainerBase               `json:"initContainers,omitempty" patchStrategy:"merge" patchMergeKey:"name"`
	ImagePullSecrets   []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

func (base *PodSpecBase) Override(spec corev1.PodSpec) corev1.PodSpec {
	if base == nil {
		return spec
	}
	if base.SecurityContext != nil {
		spec.SecurityContext = base.SecurityContext
	}
	if base.Tolerations != nil {
		spec.Tolerations = base.Tolerations
	}
	if base.NodeSelector != nil {
		spec.NodeSelector = base.NodeSelector
	}
	if base.ServiceAccountName != "" {
		spec.ServiceAccountName = base.ServiceAccountName
	}
	if base.Affinity != nil {
		spec.Affinity = base.Affinity
	}
	if len(base.Volumes) > 0 {
		spec.Volumes = base.Volumes
	}
	if base.PriorityClassName != "" {
		spec.PriorityClassName = base.PriorityClassName
	}
	if len(base.Containers) > 0 {
		for _, baseContainer := range base.Containers {
			for o, originalContainer := range spec.Containers {
				if baseContainer.Name == originalContainer.Name {
					spec.Containers[o] = baseContainer.Override(originalContainer)
					break
				}
			}
		}
	}
	if len(base.InitContainers) > 0 {
		for _, baseContainer := range base.InitContainers {
			for o, originalContainer := range spec.InitContainers {
				if baseContainer.Name == originalContainer.Name {
					spec.InitContainers[o] = baseContainer.Override(originalContainer)
					break
				}
			}
		}
	}

	spec.ImagePullSecrets = append(([]corev1.LocalObjectReference)(nil), spec.ImagePullSecrets...)
	spec.ImagePullSecrets = append(spec.ImagePullSecrets, base.ImagePullSecrets...)

	return spec
}

// +kubebuilder:object:generate=true

// Deprecated
// Consider using Deployment in the typeoverrides package combined with the merge package
type DeploymentBase struct {
	*MetaBase `json:",inline"`
	Spec      *DeploymentSpecBase `json:"spec,omitempty"`
}

func (base *DeploymentBase) Override(deployment appsv1.Deployment) appsv1.Deployment {
	if base == nil {
		return deployment
	}
	if base.MetaBase != nil {
		deployment.ObjectMeta = base.MetaBase.Merge(deployment.ObjectMeta)
	}
	if base.Spec != nil {
		deployment.Spec = base.Spec.Override(deployment.Spec)
	}
	return deployment
}

// +kubebuilder:object:generate=true

// Deprecated
// Consider using DeploymentSpec in the typeoverrides package combined with the merge package
type DeploymentSpecBase struct {
	Replicas *int32                     `json:"replicas,omitempty"`
	Selector *metav1.LabelSelector      `json:"selector,omitempty"`
	Strategy *appsv1.DeploymentStrategy `json:"strategy,omitempty"`
	Template *PodTemplateBase           `json:"template,omitempty"`
}

func (base *DeploymentSpecBase) Override(spec appsv1.DeploymentSpec) appsv1.DeploymentSpec {
	if base == nil {
		return spec
	}
	if base.Replicas != nil {
		spec.Replicas = base.Replicas
	}
	spec.Selector = mergeSelectors(base.Selector, spec.Selector)
	if base.Strategy != nil {
		spec.Strategy = *base.Strategy
	}
	if base.Template != nil {
		spec.Template = base.Template.Override(spec.Template)
	}
	return spec
}

// +kubebuilder:object:generate=true

// Deprecated
// Consider using StatefulSet in the typeoverrides package combined with the merge package
type StatefulSetBase struct {
	*MetaBase `json:",inline"`
	Spec      *StatefulsetSpecBase `json:"spec,omitempty"`
}

func (base *StatefulSetBase) Override(statefulSet appsv1.StatefulSet) appsv1.StatefulSet {
	if base == nil {
		return statefulSet
	}
	if base.MetaBase != nil {
		statefulSet.ObjectMeta = base.MetaBase.Merge(statefulSet.ObjectMeta)
	}
	if base.Spec != nil {
		statefulSet.Spec = base.Spec.Override(statefulSet.Spec)
	}
	return statefulSet
}

// +kubebuilder:object:generate=true

// Deprecated
// Consider using StatefulSetSpec in the typeoverrides package combined with the merge package
type StatefulsetSpecBase struct {
	Replicas            *int32                            `json:"replicas,omitempty"`
	Selector            *metav1.LabelSelector             `json:"selector,omitempty"`
	PodManagementPolicy appsv1.PodManagementPolicyType    `json:"podManagementPolicy,omitempty"`
	UpdateStrategy      *appsv1.StatefulSetUpdateStrategy `json:"updateStrategy,omitempty"`
	Template            *PodTemplateBase                  `json:"template,omitempty"`
}

func (base *StatefulsetSpecBase) Override(spec appsv1.StatefulSetSpec) appsv1.StatefulSetSpec {
	if base == nil {
		return spec
	}
	if base.Replicas != nil {
		spec.Replicas = base.Replicas
	}
	spec.Selector = mergeSelectors(base.Selector, spec.Selector)
	if base.PodManagementPolicy != "" {
		spec.PodManagementPolicy = base.PodManagementPolicy
	}
	if base.UpdateStrategy != nil {
		spec.UpdateStrategy = *base.UpdateStrategy
	}
	if base.Template != nil {
		spec.Template = base.Template.Override(spec.Template)
	}
	return spec
}

// +kubebuilder:object:generate=true

// Deprecated
// Consider using DaemonSet in the typeoverrides package combined with the merge package
type DaemonSetBase struct {
	*MetaBase `json:",inline"`
	Spec      *DaemonSetSpecBase `json:"spec,omitempty"`
}

func (base *DaemonSetBase) Override(daemonset appsv1.DaemonSet) appsv1.DaemonSet {
	if base == nil {
		return daemonset
	}
	if base.MetaBase != nil {
		daemonset.ObjectMeta = base.MetaBase.Merge(daemonset.ObjectMeta)
	}
	if base.Spec != nil {
		daemonset.Spec = base.Spec.Override(daemonset.Spec)
	}
	return daemonset
}

// +kubebuilder:object:generate=true

// Deprecated
// Consider using DaemonSetSpec in the typeoverrides package combined with the merge package
type DaemonSetSpecBase struct {
	Selector             *metav1.LabelSelector           `json:"selector,omitempty"`
	UpdateStrategy       *appsv1.DaemonSetUpdateStrategy `json:"updateStrategy,omitempty"`
	MinReadySeconds      int32                           `json:"minReadySeconds,omitempty"`
	RevisionHistoryLimit *int32                          `json:"revisionHistoryLimit,omitempty"`
	Template             *PodTemplateBase                `json:"template,omitempty"`
}

func (base *DaemonSetSpecBase) Override(spec appsv1.DaemonSetSpec) appsv1.DaemonSetSpec {
	if base == nil {
		return spec
	}
	spec.Selector = mergeSelectors(base.Selector, spec.Selector)
	if base.UpdateStrategy != nil {
		spec.UpdateStrategy = *base.UpdateStrategy
	}
	if base.MinReadySeconds != 0 {
		spec.MinReadySeconds = base.MinReadySeconds
	}
	if base.RevisionHistoryLimit != nil {
		spec.RevisionHistoryLimit = base.RevisionHistoryLimit
	}
	if base.Template != nil {
		spec.Template = base.Template.Override(spec.Template)
	}
	return spec
}

func mergeSelectors(base, spec *metav1.LabelSelector) *metav1.LabelSelector {
	if base == nil {
		return spec
	}

	if base.MatchLabels != nil {
		if spec == nil {
			spec = &metav1.LabelSelector{}
		}
		if spec.MatchLabels == nil {
			spec.MatchLabels = make(map[string]string)
		}
		for k, v := range base.MatchLabels {
			spec.MatchLabels[k] = v
		}
	}
	if base.MatchExpressions != nil {
		spec.MatchExpressions = append(spec.MatchExpressions, base.MatchExpressions...)
	}

	return spec
}
