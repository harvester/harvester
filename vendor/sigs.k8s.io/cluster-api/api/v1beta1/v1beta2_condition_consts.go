/*
Copyright 2024 The Kubernetes Authors.

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

// Conditions types that are used across different objects.
const (
	// AvailableV1Beta2Condition reports if an object is available.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	AvailableV1Beta2Condition = "Available"

	// ReadyV1Beta2Condition reports if an object is ready.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	ReadyV1Beta2Condition = "Ready"

	// BootstrapConfigReadyV1Beta2Condition reports if an object's bootstrap config is ready.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	BootstrapConfigReadyV1Beta2Condition = "BootstrapConfigReady"

	// InfrastructureReadyV1Beta2Condition reports if an object's infrastructure is ready.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	InfrastructureReadyV1Beta2Condition = "InfrastructureReady"

	// MachinesReadyV1Beta2Condition surfaces detail of issues on the controlled machines, if any.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	MachinesReadyV1Beta2Condition = "MachinesReady"

	// MachinesUpToDateV1Beta2Condition surfaces details of controlled machines not up to date, if any.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	MachinesUpToDateV1Beta2Condition = "MachinesUpToDate"

	// RollingOutV1Beta2Condition reports if an object is rolling out changes to machines; Cluster API usually
	// rolls out changes to machines by replacing not up-to-date machines with new ones.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	RollingOutV1Beta2Condition = "RollingOut"

	// ScalingUpV1Beta2Condition reports if an object is scaling up.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	ScalingUpV1Beta2Condition = "ScalingUp"

	// ScalingDownV1Beta2Condition reports if an object is scaling down.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	ScalingDownV1Beta2Condition = "ScalingDown"

	// RemediatingV1Beta2Condition surfaces details about ongoing remediation of the controlled machines, if any.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	RemediatingV1Beta2Condition = "Remediating"

	// DeletingV1Beta2Condition surfaces details about progress of the object deletion workflow.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	DeletingV1Beta2Condition = "Deleting"

	// PausedV1Beta2Condition reports if reconciliation for an object or the cluster is paused.
	// Note: This condition type is defined to ensure consistent naming of conditions across objects.
	// Please use object specific variants of this condition which provides more details for each context where
	// the same condition type exists.
	PausedV1Beta2Condition = "Paused"
)

// Reasons that are used across different objects.
const (
	// AvailableV1Beta2Reason applies to a condition surfacing object availability.
	AvailableV1Beta2Reason = "Available"

	// NotAvailableV1Beta2Reason applies to a condition surfacing object not satisfying availability criteria.
	NotAvailableV1Beta2Reason = "NotAvailable"

	// AvailableUnknownV1Beta2Reason applies to a condition surfacing object availability unknown.
	AvailableUnknownV1Beta2Reason = "AvailableUnknown"

	// ReadyV1Beta2Reason applies to a condition surfacing object readiness.
	ReadyV1Beta2Reason = "Ready"

	// NotReadyV1Beta2Reason applies to a condition surfacing object not satisfying readiness criteria.
	NotReadyV1Beta2Reason = "NotReady"

	// ReadyUnknownV1Beta2Reason applies to a condition surfacing object readiness unknown.
	ReadyUnknownV1Beta2Reason = "ReadyUnknown"

	// UpToDateV1Beta2Reason applies to a condition surfacing object up-tp-date.
	UpToDateV1Beta2Reason = "UpToDate"

	// NotUpToDateV1Beta2Reason applies to a condition surfacing object not up-tp-date.
	NotUpToDateV1Beta2Reason = "NotUpToDate"

	// UpToDateUnknownV1Beta2Reason applies to a condition surfacing object up-tp-date unknown.
	UpToDateUnknownV1Beta2Reason = "UpToDateUnknown"

	// RollingOutV1Beta2Reason surfaces when an object is rolling out.
	RollingOutV1Beta2Reason = "RollingOut"

	// NotRollingOutV1Beta2Reason surfaces when an object is not rolling out.
	NotRollingOutV1Beta2Reason = "NotRollingOut"

	// ScalingUpV1Beta2Reason surfaces when an object is scaling up.
	ScalingUpV1Beta2Reason = "ScalingUp"

	// NotScalingUpV1Beta2Reason surfaces when an object is not scaling up.
	NotScalingUpV1Beta2Reason = "NotScalingUp"

	// ScalingDownV1Beta2Reason surfaces when an object is scaling down.
	ScalingDownV1Beta2Reason = "ScalingDown"

	// NotScalingDownV1Beta2Reason surfaces when an object is not scaling down.
	NotScalingDownV1Beta2Reason = "NotScalingDown"

	// RemediatingV1Beta2Reason surfaces when an object owns at least one machine with HealthCheckSucceeded
	// set to false and with the OwnerRemediated condition set to false by the MachineHealthCheck controller.
	RemediatingV1Beta2Reason = "Remediating"

	// NotRemediatingV1Beta2Reason surfaces when an object does not own any machines with HealthCheckSucceeded
	// set to false and with the OwnerRemediated condition set to false by the MachineHealthCheck controller.
	NotRemediatingV1Beta2Reason = "NotRemediating"

	// NoReplicasV1Beta2Reason surfaces when an object that manage replicas does not have any.
	NoReplicasV1Beta2Reason = "NoReplicas"

	// WaitingForReplicasSetV1Beta2Reason surfaces when the replica field of an object is not set.
	WaitingForReplicasSetV1Beta2Reason = "WaitingForReplicasSet"

	// InvalidConditionReportedV1Beta2Reason applies to a condition, usually read from an external object, that is invalid
	// (e.g. its status is missing).
	InvalidConditionReportedV1Beta2Reason = "InvalidConditionReported"

	// NoReasonReportedV1Beta2Reason applies to a condition, usually read from an external object, that reports no reason.
	// Note: this could happen e.g. when an external object still uses Cluster API v1beta1 Conditions.
	NoReasonReportedV1Beta2Reason = "NoReasonReported"

	// InternalErrorV1Beta2Reason surfaces unexpected errors reporting by controllers.
	// In most cases, it will be required to look at controllers logs to properly triage those issues.
	InternalErrorV1Beta2Reason = "InternalError"

	// ObjectDoesNotExistV1Beta2Reason surfaces when a referenced object does not exist.
	ObjectDoesNotExistV1Beta2Reason = "ObjectDoesNotExist"

	// ObjectDeletedV1Beta2Reason surfaces when a referenced object has been deleted.
	// Note: controllers can't identify if the object was deleted by the controller itself, e.g.
	// during the deletion workflow, or by a users.
	ObjectDeletedV1Beta2Reason = "ObjectDeleted"

	// NotPausedV1Beta2Reason surfaces when an object is not paused.
	NotPausedV1Beta2Reason = "NotPaused"

	// PausedV1Beta2Reason surfaces when an object is paused.
	PausedV1Beta2Reason = "Paused"

	// ConnectionDownV1Beta2Reason surfaces that the connection to the workload cluster is down.
	ConnectionDownV1Beta2Reason = "ConnectionDown"

	// NotDeletingV1Beta2Reason surfaces when an object is not deleting because the
	// DeletionTimestamp is not set.
	NotDeletingV1Beta2Reason = "NotDeleting"

	// DeletingV1Beta2Reason surfaces when an object is deleting because the
	// DeletionTimestamp is set. This reason is used if none of the more specific reasons apply.
	DeletingV1Beta2Reason = "Deleting"

	// DeletionCompletedV1Beta2Reason surfaces when the deletion process has been completed.
	// This reason is set right after the corresponding finalizer is removed.
	// This means that the object will go away (i.e. be removed from etcd), except if there are other
	// finalizers on the object.
	DeletionCompletedV1Beta2Reason = "DeletionCompleted"

	// InspectionFailedV1Beta2Reason applies to a condition when inspection of the underlying object failed.
	InspectionFailedV1Beta2Reason = "InspectionFailed"
)
