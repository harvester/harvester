/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation/field"

	capierrors "sigs.k8s.io/cluster-api/errors"
)

const (
	// MachineSetTopologyFinalizer is the finalizer used by the topology MachineDeployment controller to
	// clean up referenced template resources if necessary when a MachineSet is being deleted.
	MachineSetTopologyFinalizer = "machineset.topology.cluster.x-k8s.io"

	// MachineSetFinalizer is the finalizer used by the MachineSet controller to
	// ensure ordered cleanup of corresponding Machines when a Machineset is being deleted.
	MachineSetFinalizer = "cluster.x-k8s.io/machineset"
)

// ANCHOR: MachineSetSpec

// MachineSetSpec defines the desired state of MachineSet.
type MachineSetSpec struct {
	// clusterName is the name of the Cluster this object belongs to.
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// replicas is the number of desired replicas.
	// This is a pointer to distinguish between explicit zero and unspecified.
	//
	// Defaults to:
	// * if the Kubernetes autoscaler min size and max size annotations are set:
	//   - if it's a new MachineSet, use min size
	//   - if the replicas field of the old MachineSet is < min size, use min size
	//   - if the replicas field of the old MachineSet is > max size, use max size
	//   - if the replicas field of the old MachineSet is in the (min size, max size) range, keep the value from the oldMS
	// * otherwise use 1
	// Note: Defaulting will be run whenever the replicas field is not set:
	// * A new MachineSet is created with replicas not set.
	// * On an existing MachineSet the replicas field was first set and is now unset.
	// Those cases are especially relevant for the following Kubernetes autoscaler use cases:
	// * A new MachineSet is created and replicas should be managed by the autoscaler
	// * An existing MachineSet which initially wasn't controlled by the autoscaler
	//   should be later controlled by the autoscaler
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// minReadySeconds is the minimum number of seconds for which a Node for a newly created machine should be ready before considering the replica available.
	// Defaults to 0 (machine will be considered available as soon as the Node is ready)
	// +optional
	MinReadySeconds int32 `json:"minReadySeconds,omitempty"`

	// deletePolicy defines the policy used to identify nodes to delete when downscaling.
	// Defaults to "Random".  Valid values are "Random, "Newest", "Oldest"
	// +kubebuilder:validation:Enum=Random;Newest;Oldest
	// +optional
	DeletePolicy string `json:"deletePolicy,omitempty"`

	// selector is a label query over machines that should match the replica count.
	// Label keys and values that must match in order to be controlled by this MachineSet.
	// It must match the machine template's labels.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	Selector metav1.LabelSelector `json:"selector"`

	// template is the object that describes the machine that will be created if
	// insufficient replicas are detected.
	// Object references to custom resources are treated as templates.
	// +optional
	Template MachineTemplateSpec `json:"template,omitempty"`
}

// MachineSet's ScalingUp condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// MachineSetScalingUpV1Beta2Condition is true if actual replicas < desired replicas.
	// Note: In case a MachineSet preflight check is preventing scale up, this will surface in the condition message.
	MachineSetScalingUpV1Beta2Condition = ScalingUpV1Beta2Condition

	// MachineSetScalingUpV1Beta2Reason surfaces when actual replicas < desired replicas.
	MachineSetScalingUpV1Beta2Reason = ScalingUpV1Beta2Reason

	// MachineSetNotScalingUpV1Beta2Reason surfaces when actual replicas >= desired replicas.
	MachineSetNotScalingUpV1Beta2Reason = NotScalingUpV1Beta2Reason

	// MachineSetScalingUpInternalErrorV1Beta2Reason surfaces unexpected failures when listing machines.
	MachineSetScalingUpInternalErrorV1Beta2Reason = InternalErrorV1Beta2Reason

	// MachineSetScalingUpWaitingForReplicasSetV1Beta2Reason surfaces when the .spec.replicas
	// field of the MachineSet is not set.
	MachineSetScalingUpWaitingForReplicasSetV1Beta2Reason = WaitingForReplicasSetV1Beta2Reason
)

// MachineSet's ScalingDown condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// MachineSetScalingDownV1Beta2Condition is true if actual replicas > desired replicas.
	MachineSetScalingDownV1Beta2Condition = ScalingDownV1Beta2Condition

	// MachineSetScalingDownV1Beta2Reason surfaces when actual replicas > desired replicas.
	MachineSetScalingDownV1Beta2Reason = ScalingDownV1Beta2Reason

	// MachineSetNotScalingDownV1Beta2Reason surfaces when actual replicas <= desired replicas.
	MachineSetNotScalingDownV1Beta2Reason = NotScalingDownV1Beta2Reason

	// MachineSetScalingDownInternalErrorV1Beta2Reason surfaces unexpected failures when listing machines.
	MachineSetScalingDownInternalErrorV1Beta2Reason = InternalErrorV1Beta2Reason

	// MachineSetScalingDownWaitingForReplicasSetV1Beta2Reason surfaces when the .spec.replicas
	// field of the MachineSet is not set.
	MachineSetScalingDownWaitingForReplicasSetV1Beta2Reason = WaitingForReplicasSetV1Beta2Reason
)

// MachineSet's MachinesReady condition and corresponding reasons that will be used in v1Beta2 API version.
// Note: Reason's could also be derived from the aggregation of machine's Ready conditions.
const (
	// MachineSetMachinesReadyV1Beta2Condition surfaces detail of issues on the controlled machines, if any.
	MachineSetMachinesReadyV1Beta2Condition = MachinesReadyV1Beta2Condition

	// MachineSetMachinesReadyV1Beta2Reason surfaces when all the controlled machine's Ready conditions are true.
	MachineSetMachinesReadyV1Beta2Reason = ReadyV1Beta2Reason

	// MachineSetMachinesNotReadyV1Beta2Reason surfaces when at least one of the controlled machine's Ready conditions is false.
	MachineSetMachinesNotReadyV1Beta2Reason = NotReadyV1Beta2Reason

	// MachineSetMachinesReadyUnknownV1Beta2Reason surfaces when at least one of the controlled machine's Ready conditions is unknown
	// and none of the controlled machine's Ready conditions is false.
	MachineSetMachinesReadyUnknownV1Beta2Reason = ReadyUnknownV1Beta2Reason

	// MachineSetMachinesReadyNoReplicasV1Beta2Reason surfaces when no machines exist for the MachineSet.
	MachineSetMachinesReadyNoReplicasV1Beta2Reason = NoReplicasV1Beta2Reason

	// MachineSetMachinesReadyInternalErrorV1Beta2Reason surfaces unexpected failures when listing machines
	// or aggregating machine's conditions.
	MachineSetMachinesReadyInternalErrorV1Beta2Reason = InternalErrorV1Beta2Reason
)

// MachineSet's MachinesUpToDate condition and corresponding reasons that will be used in v1Beta2 API version.
// Note: Reason's could also be derived from the aggregation of machine's MachinesUpToDate conditions.
const (
	// MachineSetMachinesUpToDateV1Beta2Condition surfaces details of controlled machines not up to date, if any.
	// Note: New machines are considered 10s after machine creation. This gives time to the machine's owner controller to recognize the new machine and add the UpToDate condition.
	MachineSetMachinesUpToDateV1Beta2Condition = MachinesUpToDateV1Beta2Condition

	// MachineSetMachinesUpToDateV1Beta2Reason surfaces when all the controlled machine's UpToDate conditions are true.
	MachineSetMachinesUpToDateV1Beta2Reason = UpToDateV1Beta2Reason

	// MachineSetMachinesNotUpToDateV1Beta2Reason surfaces when at least one of the controlled machine's UpToDate conditions is false.
	MachineSetMachinesNotUpToDateV1Beta2Reason = NotUpToDateV1Beta2Reason

	// MachineSetMachinesUpToDateUnknownV1Beta2Reason surfaces when at least one of the controlled machine's UpToDate conditions is unknown
	// and none of the controlled machine's UpToDate conditions is false.
	MachineSetMachinesUpToDateUnknownV1Beta2Reason = UpToDateUnknownV1Beta2Reason

	// MachineSetMachinesUpToDateNoReplicasV1Beta2Reason surfaces when no machines exist for the MachineSet.
	MachineSetMachinesUpToDateNoReplicasV1Beta2Reason = NoReplicasV1Beta2Reason

	// MachineSetMachinesUpToDateInternalErrorV1Beta2Reason surfaces unexpected failures when listing machines
	// or aggregating status.
	MachineSetMachinesUpToDateInternalErrorV1Beta2Reason = InternalErrorV1Beta2Reason
)

// MachineSet's Remediating condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// MachineSetRemediatingV1Beta2Condition surfaces details about ongoing remediation of the controlled machines, if any.
	MachineSetRemediatingV1Beta2Condition = RemediatingV1Beta2Condition

	// MachineSetRemediatingV1Beta2Reason surfaces when the MachineSet has at least one machine with HealthCheckSucceeded set to false
	// and with the OwnerRemediated condition set to false.
	MachineSetRemediatingV1Beta2Reason = RemediatingV1Beta2Reason

	// MachineSetNotRemediatingV1Beta2Reason surfaces when the MachineSet does not have any machine with HealthCheckSucceeded set to false
	// and with the OwnerRemediated condition set to false.
	MachineSetNotRemediatingV1Beta2Reason = NotRemediatingV1Beta2Reason

	// MachineSetRemediatingInternalErrorV1Beta2Reason surfaces unexpected failures when computing the Remediating condition.
	MachineSetRemediatingInternalErrorV1Beta2Reason = InternalErrorV1Beta2Reason
)

// Reasons that will be used for the OwnerRemediated condition set by MachineHealthCheck on MachineSet controlled machines
// being remediated in v1Beta2 API version.
const (
	// MachineSetMachineCannotBeRemediatedV1Beta2Reason surfaces when remediation of a MachineSet machine can't be started.
	MachineSetMachineCannotBeRemediatedV1Beta2Reason = "CannotBeRemediated"

	// MachineSetMachineRemediationDeferredV1Beta2Reason surfaces when remediation of a MachineSet machine must be deferred.
	MachineSetMachineRemediationDeferredV1Beta2Reason = "RemediationDeferred"

	// MachineSetMachineRemediationMachineDeletingV1Beta2Reason surfaces when remediation of a MachineSet machine
	// has been completed by deleting the unhealthy machine.
	// Note: After an unhealthy machine is deleted, a new one is created by the MachineSet as part of the
	// regular reconcile loop that ensures the correct number of replicas exist.
	MachineSetMachineRemediationMachineDeletingV1Beta2Reason = "MachineDeleting"
)

// MachineSet's Deleting condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// MachineSetDeletingV1Beta2Condition surfaces details about ongoing deletion of the controlled machines.
	MachineSetDeletingV1Beta2Condition = DeletingV1Beta2Condition

	// MachineSetNotDeletingV1Beta2Reason surfaces when the MachineSet is not deleting because the
	// DeletionTimestamp is not set.
	MachineSetNotDeletingV1Beta2Reason = NotDeletingV1Beta2Reason

	// MachineSetDeletingV1Beta2Reason surfaces when the MachineSet is deleting because the
	// DeletionTimestamp is set.
	MachineSetDeletingV1Beta2Reason = DeletingV1Beta2Reason

	// MachineSetDeletingInternalErrorV1Beta2Reason surfaces unexpected failures when deleting a MachineSet.
	MachineSetDeletingInternalErrorV1Beta2Reason = InternalErrorV1Beta2Reason
)

// ANCHOR_END: MachineSetSpec

// ANCHOR: MachineTemplateSpec

// MachineTemplateSpec describes the data needed to create a Machine from a template.
type MachineTemplateSpec struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the machine.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Spec MachineSpec `json:"spec,omitempty"`
}

// ANCHOR_END: MachineTemplateSpec

// MachineSetDeletePolicy defines how priority is assigned to nodes to delete when
// downscaling a MachineSet. Defaults to "Random".
type MachineSetDeletePolicy string

const (
	// RandomMachineSetDeletePolicy prioritizes both Machines that have the annotation
	// "cluster.x-k8s.io/delete-machine=yes" and Machines that are unhealthy
	// (Status.FailureReason or Status.FailureMessage are set to a non-empty value
	// or NodeHealthy type of Status.Conditions is not true).
	// Finally, it picks Machines at random to delete.
	RandomMachineSetDeletePolicy MachineSetDeletePolicy = "Random"

	// NewestMachineSetDeletePolicy prioritizes both Machines that have the annotation
	// "cluster.x-k8s.io/delete-machine=yes" and Machines that are unhealthy
	// (Status.FailureReason or Status.FailureMessage are set to a non-empty value
	// or NodeHealthy type of Status.Conditions is not true).
	// It then prioritizes the newest Machines for deletion based on the Machine's CreationTimestamp.
	NewestMachineSetDeletePolicy MachineSetDeletePolicy = "Newest"

	// OldestMachineSetDeletePolicy prioritizes both Machines that have the annotation
	// "cluster.x-k8s.io/delete-machine=yes" and Machines that are unhealthy
	// (Status.FailureReason or Status.FailureMessage are set to a non-empty value
	// or NodeHealthy type of Status.Conditions is not true).
	// It then prioritizes the oldest Machines for deletion based on the Machine's CreationTimestamp.
	OldestMachineSetDeletePolicy MachineSetDeletePolicy = "Oldest"
)

// ANCHOR: MachineSetStatus

// MachineSetStatus defines the observed state of MachineSet.
type MachineSetStatus struct {
	// selector is the same as the label selector but in the string format to avoid introspection
	// by clients. The string will be in the same format as the query-param syntax.
	// More info about label selectors: http://kubernetes.io/docs/user-guide/labels#label-selectors
	// +optional
	Selector string `json:"selector,omitempty"`

	// replicas is the most recently observed number of replicas.
	// +optional
	Replicas int32 `json:"replicas"`

	// The number of replicas that have labels matching the labels of the machine template of the MachineSet.
	//
	// Deprecated: This field is deprecated and is going to be removed in the next apiVersion. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	FullyLabeledReplicas int32 `json:"fullyLabeledReplicas"`

	// The number of ready replicas for this MachineSet. A machine is considered ready when the node has been created and is "Ready".
	// +optional
	ReadyReplicas int32 `json:"readyReplicas"`

	// The number of available replicas (ready for at least minReadySeconds) for this MachineSet.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas"`

	// observedGeneration reflects the generation of the most recently observed MachineSet.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// In the event that there is a terminal problem reconciling the
	// replicas, both FailureReason and FailureMessage will be set. FailureReason
	// will be populated with a succinct value suitable for machine
	// interpretation, while FailureMessage will contain a more verbose
	// string suitable for logging and human consumption.
	//
	// These fields should not be set for transitive errors that a
	// controller faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the MachineTemplate's spec or the configuration of
	// the machine controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the machine controller, or the
	// responsible machine controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the MachineSet object and/or logged in the
	// controller's output.
	//
	// Deprecated: This field is deprecated and is going to be removed in the next apiVersion. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	FailureReason *capierrors.MachineSetStatusError `json:"failureReason,omitempty"`
	// Deprecated: This field is deprecated and is going to be removed in the next apiVersion. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`
	// conditions defines current service state of the MachineSet.
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in MachineSet's status with the V1Beta2 version.
	// +optional
	V1Beta2 *MachineSetV1Beta2Status `json:"v1beta2,omitempty"`
}

// MachineSetV1Beta2Status groups all the fields that will be added or modified in MachineSetStatus with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachineSetV1Beta2Status struct {
	// conditions represents the observations of a MachineSet's current state.
	// Known condition types are MachinesReady, MachinesUpToDate, ScalingUp, ScalingDown, Remediating, Deleting, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// readyReplicas is the number of ready replicas for this MachineSet. A machine is considered ready when Machine's Ready condition is true.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// availableReplicas is the number of available replicas for this MachineSet. A machine is considered available when Machine's Available condition is true.
	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`

	// upToDateReplicas is the number of up-to-date replicas for this MachineSet. A machine is considered up-to-date when Machine's UpToDate condition is true.
	// +optional
	UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`
}

// ANCHOR_END: MachineSetStatus

// Validate validates the MachineSet fields.
func (m *MachineSet) Validate() field.ErrorList {
	errors := field.ErrorList{}

	// validate spec.selector and spec.template.labels
	fldPath := field.NewPath("spec")
	errors = append(errors, metav1validation.ValidateLabelSelector(&m.Spec.Selector, metav1validation.LabelSelectorValidationOptions{}, fldPath.Child("selector"))...)
	if len(m.Spec.Selector.MatchLabels)+len(m.Spec.Selector.MatchExpressions) == 0 {
		errors = append(errors, field.Invalid(fldPath.Child("selector"), m.Spec.Selector, "empty selector is not valid for MachineSet."))
	}
	selector, err := metav1.LabelSelectorAsSelector(&m.Spec.Selector)
	if err != nil {
		errors = append(errors, field.Invalid(fldPath.Child("selector"), m.Spec.Selector, "invalid label selector."))
	} else {
		labels := labels.Set(m.Spec.Template.Labels)
		if !selector.Matches(labels) {
			errors = append(errors, field.Invalid(fldPath.Child("template", "metadata", "labels"), m.Spec.Template.Labels, "`selector` does not match template `labels`"))
		}
	}

	return errors
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=machinesets,shortName=ms,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName",description="Cluster"
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=".spec.replicas",description="Total number of machines desired by this machineset",priority=10
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas",description="Total number of non-terminated machines targeted by this machineset"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas",description="Total number of ready machines targeted by this machineset."
// +kubebuilder:printcolumn:name="Available",type="integer",JSONPath=".status.availableReplicas",description="Total number of available machines (ready for at least minReadySeconds)"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of MachineSet"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.template.spec.version",description="Kubernetes version associated with this MachineSet"

// MachineSet is the Schema for the machinesets API.
type MachineSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineSetSpec   `json:"spec,omitempty"`
	Status MachineSetStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for the MachineSet.
func (m *MachineSet) GetConditions() Conditions {
	return m.Status.Conditions
}

// SetConditions updates the set of conditions on the MachineSet.
func (m *MachineSet) SetConditions(conditions Conditions) {
	m.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (m *MachineSet) GetV1Beta2Conditions() []metav1.Condition {
	if m.Status.V1Beta2 == nil {
		return nil
	}
	return m.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (m *MachineSet) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if m.Status.V1Beta2 == nil {
		m.Status.V1Beta2 = &MachineSetV1Beta2Status{}
	}
	m.Status.V1Beta2.Conditions = conditions
}

// +kubebuilder:object:root=true

// MachineSetList contains a list of MachineSet.
type MachineSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineSet `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &MachineSet{}, &MachineSetList{})
}
