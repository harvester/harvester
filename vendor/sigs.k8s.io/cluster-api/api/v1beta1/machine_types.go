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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	capierrors "sigs.k8s.io/cluster-api/errors"
)

const (
	// MachineFinalizer is set on PrepareForCreate callback.
	MachineFinalizer = "machine.cluster.x-k8s.io"

	// MachineControlPlaneLabel is the label set on machines or related objects that are part of a control plane.
	MachineControlPlaneLabel = "cluster.x-k8s.io/control-plane"

	// ExcludeNodeDrainingAnnotation annotation explicitly skips node draining if set.
	ExcludeNodeDrainingAnnotation = "machine.cluster.x-k8s.io/exclude-node-draining"

	// ExcludeWaitForNodeVolumeDetachAnnotation annotation explicitly skips the waiting for node volume detaching if set.
	ExcludeWaitForNodeVolumeDetachAnnotation = "machine.cluster.x-k8s.io/exclude-wait-for-node-volume-detach"

	// MachineSetNameLabel is the label set on machines if they're controlled by MachineSet.
	// Note: The value of this label may be a hash if the MachineSet name is longer than 63 characters.
	MachineSetNameLabel = "cluster.x-k8s.io/set-name"

	// MachineDeploymentNameLabel is the label set on machines if they're controlled by MachineDeployment.
	MachineDeploymentNameLabel = "cluster.x-k8s.io/deployment-name"

	// MachinePoolNameLabel is the label indicating the name of the MachinePool a Machine is controlled by.
	// Note: The value of this label may be a hash if the MachinePool name is longer than 63 characters.
	MachinePoolNameLabel = "cluster.x-k8s.io/pool-name"

	// MachineControlPlaneNameLabel is the label set on machines if they're controlled by a ControlPlane.
	// Note: The value of this label may be a hash if the control plane name is longer than 63 characters.
	MachineControlPlaneNameLabel = "cluster.x-k8s.io/control-plane-name"

	// PreDrainDeleteHookAnnotationPrefix annotation specifies the prefix we
	// search each annotation for during the pre-drain.delete lifecycle hook
	// to pause reconciliation of deletion. These hooks will prevent removal of
	// draining the associated node until all are removed.
	PreDrainDeleteHookAnnotationPrefix = "pre-drain.delete.hook.machine.cluster.x-k8s.io"

	// PreTerminateDeleteHookAnnotationPrefix annotation specifies the prefix we
	// search each annotation for during the pre-terminate.delete lifecycle hook
	// to pause reconciliation of deletion. These hooks will prevent removal of
	// an instance from an infrastructure provider until all are removed.
	//
	// Notes for Machines managed by KCP (starting with Cluster API v1.8.2):
	// * KCP adds its own pre-terminate hook on all Machines it controls. This is done to ensure it can later remove
	//   the etcd member right before Machine termination (i.e. before InfraMachine deletion).
	// * Starting with Kubernetes v1.31 the KCP pre-terminate hook will wait for all other pre-terminate hooks to finish to
	//   ensure it runs last (thus ensuring that kubelet is still working while other pre-terminate hooks run). This is only done
	//   for v1.31 or above because the kubeadm ControlPlaneKubeletLocalMode was introduced with kubeadm 1.31. This feature configures
	//   the kubelet to communicate with the local apiserver. Only because of that the kubelet immediately starts failing after the etcd
	//   member is removed. We need the ControlPlaneKubeletLocalMode feature with 1.31 to adhere to the kubelet skew policy.
	PreTerminateDeleteHookAnnotationPrefix = "pre-terminate.delete.hook.machine.cluster.x-k8s.io"

	// MachineCertificatesExpiryDateAnnotation annotation specifies the expiry date of the machine certificates in RFC3339 format.
	// This annotation can be used on control plane machines to trigger rollout before certificates expire.
	// This annotation can be set on BootstrapConfig or Machine objects. The value set on the Machine object takes precedence.
	// This annotation can only be used on Control Plane Machines.
	MachineCertificatesExpiryDateAnnotation = "machine.cluster.x-k8s.io/certificates-expiry"

	// NodeRoleLabelPrefix is one of the CAPI managed Node label prefixes.
	NodeRoleLabelPrefix = "node-role.kubernetes.io"
	// NodeRestrictionLabelDomain is one of the CAPI managed Node label domains.
	NodeRestrictionLabelDomain = "node-restriction.kubernetes.io"
	// ManagedNodeLabelDomain is one of the CAPI managed Node label domains.
	ManagedNodeLabelDomain = "node.cluster.x-k8s.io"
)

// Machine's Available condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// MachineAvailableV1Beta2Condition is true if the machine is Ready for at least MinReadySeconds, as defined by the Machine's MinReadySeconds field.
	// Note: MinReadySeconds is assumed 0 until it will be implemented in v1beta2 API.
	MachineAvailableV1Beta2Condition = AvailableV1Beta2Condition

	// MachineWaitingForMinReadySecondsV1Beta2Reason surfaces when a machine is ready for less than MinReadySeconds (and thus not yet available).
	MachineWaitingForMinReadySecondsV1Beta2Reason = "WaitingForMinReadySeconds"

	// MachineAvailableV1Beta2Reason surfaces when a machine is ready for at least MinReadySeconds.
	// Note: MinReadySeconds is assumed 0 until it will be implemented in v1beta2 API.
	MachineAvailableV1Beta2Reason = AvailableV1Beta2Reason

	// MachineAvailableInternalErrorV1Beta2Reason surfaces unexpected error when computing the Available condition.
	MachineAvailableInternalErrorV1Beta2Reason = InternalErrorV1Beta2Reason
)

// Machine's Ready condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// MachineReadyV1Beta2Condition is true if the Machine's deletionTimestamp is not set, Machine's BootstrapConfigReady, InfrastructureReady,
	// NodeHealthy and HealthCheckSucceeded (if present) conditions are true; if other conditions are defined in spec.readinessGates,
	// these conditions must be true as well.
	// Note:
	// - When summarizing the Deleting condition:
	//   - Details about Pods stuck in draining or volumes waiting for detach are dropped, in order to improve readability & reduce flickering
	//     of the condition that bubbles up to the owning resources/ to the Cluster (it also makes it more likely this condition might be aggregated with
	//     conditions reported by other machines).
	//   - If deletion is in progress for more than 15m, this surfaces on the summary condition (hint about a possible stale deletion).
	//     - if drain is in progress for more than 5 minutes, a summery of what is blocking drain also surfaces in the message.
	// - When summarizing BootstrapConfigReady, InfrastructureReady, NodeHealthy, in case the Machine is deleting, the absence of the
	//   referenced object won't be considered as an issue.
	MachineReadyV1Beta2Condition = ReadyV1Beta2Condition

	// MachineReadyV1Beta2Reason surfaces when the machine readiness criteria is met.
	MachineReadyV1Beta2Reason = ReadyV1Beta2Reason

	// MachineNotReadyV1Beta2Reason surfaces when the machine readiness criteria is not met.
	// Note: when a machine is not ready, it is also not available.
	MachineNotReadyV1Beta2Reason = NotReadyV1Beta2Reason

	// MachineReadyUnknownV1Beta2Reason surfaces when at least one machine readiness criteria is unknown
	// and no machine readiness criteria is not met.
	MachineReadyUnknownV1Beta2Reason = ReadyUnknownV1Beta2Reason

	// MachineReadyInternalErrorV1Beta2Reason surfaces unexpected error when computing the Ready condition.
	MachineReadyInternalErrorV1Beta2Reason = InternalErrorV1Beta2Reason
)

// Machine's UpToDate condition and corresponding reasons that will be used in v1Beta2 API version.
// Note: UpToDate condition is set by the controller owning the machine.
const (
	// MachineUpToDateV1Beta2Condition is true if the Machine spec matches the spec of the Machine's owner resource, e.g. KubeadmControlPlane or MachineDeployment.
	// The Machine's owner (e.g. MachineDeployment) is authoritative to set their owned Machine's UpToDate conditions based on its current spec.
	// NOTE: The Machine's owner might use this condition to surface also other use cases when Machine is considered not up to date, e.g. when MachineDeployment spec.rolloutAfter
	// is expired and the Machine needs to be rolled out.
	MachineUpToDateV1Beta2Condition = "UpToDate"

	// MachineUpToDateV1Beta2Reason surface when a Machine spec matches the spec of the Machine's owner resource, e.g. KubeadmControlPlane or MachineDeployment.
	MachineUpToDateV1Beta2Reason = "UpToDate"

	// MachineNotUpToDateV1Beta2Reason surface when a Machine spec does not match the spec of the Machine's owner resource, e.g. KubeadmControlPlane or MachineDeployment.
	MachineNotUpToDateV1Beta2Reason = "NotUpToDate"
)

// Machine's BootstrapConfigReady condition and corresponding reasons that will be used in v1Beta2 API version.
// Note: when possible, BootstrapConfigReady condition will use reasons surfaced from the underlying bootstrap config object.
const (
	// MachineBootstrapConfigReadyV1Beta2Condition condition mirrors the corresponding Ready condition from the Machine's BootstrapConfig resource.
	MachineBootstrapConfigReadyV1Beta2Condition = BootstrapConfigReadyV1Beta2Condition

	// MachineBootstrapDataSecretProvidedV1Beta2Reason surfaces when a bootstrap data secret is provided (not originated
	// from a BoostrapConfig object referenced from the machine).
	MachineBootstrapDataSecretProvidedV1Beta2Reason = "DataSecretProvided"

	// MachineBootstrapConfigReadyV1Beta2Reason surfaces when the machine bootstrap config is ready.
	MachineBootstrapConfigReadyV1Beta2Reason = ReadyV1Beta2Reason

	// MachineBootstrapConfigNotReadyV1Beta2Reason surfaces when the machine bootstrap config is not ready.
	MachineBootstrapConfigNotReadyV1Beta2Reason = NotReadyV1Beta2Reason

	// MachineBootstrapConfigInvalidConditionReportedV1Beta2Reason surfaces a BootstrapConfig Ready condition (read from a bootstrap config object) which is invalid.
	// (e.g. its status is missing).
	MachineBootstrapConfigInvalidConditionReportedV1Beta2Reason = InvalidConditionReportedV1Beta2Reason

	// MachineBootstrapConfigInternalErrorV1Beta2Reason surfaces unexpected failures when reading a BootstrapConfig object.
	MachineBootstrapConfigInternalErrorV1Beta2Reason = InternalErrorV1Beta2Reason

	// MachineBootstrapConfigDoesNotExistV1Beta2Reason surfaces when a referenced bootstrap config object does not exist.
	// Note: this could happen when creating the machine. However, this state should be treated as an error if it lasts indefinitely.
	MachineBootstrapConfigDoesNotExistV1Beta2Reason = ObjectDoesNotExistV1Beta2Reason

	// MachineBootstrapConfigDeletedV1Beta2Reason surfaces when a referenced bootstrap config object has been deleted.
	// Note: controllers can't identify if the bootstrap config object was deleted the controller itself, e.g.
	// during the deletion workflow, or by a users.
	MachineBootstrapConfigDeletedV1Beta2Reason = ObjectDeletedV1Beta2Reason
)

// Machine's InfrastructureReady condition and corresponding reasons that will be used in v1Beta2 API version.
// Note: when possible, InfrastructureReady condition will use reasons surfaced from the underlying infra machine object.
const (
	// MachineInfrastructureReadyV1Beta2Condition mirrors the corresponding Ready condition from the Machine's infrastructure resource.
	MachineInfrastructureReadyV1Beta2Condition = InfrastructureReadyV1Beta2Condition

	// MachineInfrastructureReadyV1Beta2Reason surfaces when the machine infrastructure is ready.
	MachineInfrastructureReadyV1Beta2Reason = ReadyV1Beta2Reason

	// MachineInfrastructureNotReadyV1Beta2Reason surfaces when the machine infrastructure is not ready.
	MachineInfrastructureNotReadyV1Beta2Reason = NotReadyV1Beta2Reason

	// MachineInfrastructureInvalidConditionReportedV1Beta2Reason surfaces a infrastructure Ready condition (read from an infra machine object) which is invalid.
	// (e.g. its status is missing).
	MachineInfrastructureInvalidConditionReportedV1Beta2Reason = InvalidConditionReportedV1Beta2Reason

	// MachineInfrastructureInternalErrorV1Beta2Reason surfaces unexpected failures when reading an infra machine object.
	MachineInfrastructureInternalErrorV1Beta2Reason = InternalErrorV1Beta2Reason

	// MachineInfrastructureDoesNotExistV1Beta2Reason surfaces when a referenced infrastructure object does not exist.
	// Note: this could happen when creating the machine. However, this state should be treated as an error if it lasts indefinitely.
	MachineInfrastructureDoesNotExistV1Beta2Reason = ObjectDoesNotExistV1Beta2Reason

	// MachineInfrastructureDeletedV1Beta2Reason surfaces when a referenced infrastructure object has been deleted.
	// Note: controllers can't identify if the infrastructure object was deleted by the controller itself, e.g.
	// during the deletion workflow, or by a users.
	MachineInfrastructureDeletedV1Beta2Reason = ObjectDeletedV1Beta2Reason
)

// Machine's NodeHealthy and NodeReady conditions and corresponding reasons that will be used in v1Beta2 API version.
// Note: when possible, NodeHealthy and NodeReady conditions will use reasons surfaced from the underlying node.
const (
	// MachineNodeHealthyV1Beta2Condition is true if the Machine's Node is ready and it does not report MemoryPressure, DiskPressure and PIDPressure.
	MachineNodeHealthyV1Beta2Condition = "NodeHealthy"

	// MachineNodeReadyV1Beta2Condition is true if the Machine's Node is ready.
	MachineNodeReadyV1Beta2Condition = "NodeReady"

	// MachineNodeReadyV1Beta2Reason surfaces when Machine's Node Ready condition is true.
	MachineNodeReadyV1Beta2Reason = "NodeReady"

	// MachineNodeNotReadyV1Beta2Reason surfaces when Machine's Node Ready condition is false.
	MachineNodeNotReadyV1Beta2Reason = "NodeNotReady"

	// MachineNodeReadyUnknownV1Beta2Reason surfaces when Machine's Node Ready condition is unknown.
	MachineNodeReadyUnknownV1Beta2Reason = "NodeReadyUnknown"

	// MachineNodeHealthyV1Beta2Reason surfaces when all the node conditions report healthy state.
	MachineNodeHealthyV1Beta2Reason = "NodeHealthy"

	// MachineNodeNotHealthyV1Beta2Reason surfaces when at least one node conditions report not healthy state.
	MachineNodeNotHealthyV1Beta2Reason = "NodeNotHealthy"

	// MachineNodeHealthUnknownV1Beta2Reason surfaces when at least one node conditions report healthy state unknown
	// and no node conditions report not healthy state.
	MachineNodeHealthUnknownV1Beta2Reason = "NodeHealthyUnknown"

	// MachineNodeInternalErrorV1Beta2Reason surfaces unexpected failures when reading a Node object.
	MachineNodeInternalErrorV1Beta2Reason = InternalErrorV1Beta2Reason

	// MachineNodeDoesNotExistV1Beta2Reason surfaces when the node hosted on the machine does not exist.
	// Note: this could happen when creating the machine. However, this state should be treated as an error if it lasts indefinitely.
	MachineNodeDoesNotExistV1Beta2Reason = "NodeDoesNotExist"

	// MachineNodeDeletedV1Beta2Reason surfaces when the node hosted on the machine has been deleted.
	// Note: controllers can't identify if the Node was deleted by the controller itself, e.g.
	// during the deletion workflow, or by a users.
	MachineNodeDeletedV1Beta2Reason = "NodeDeleted"

	// MachineNodeInspectionFailedV1Beta2Reason documents a failure when inspecting the status of a Node.
	MachineNodeInspectionFailedV1Beta2Reason = InspectionFailedV1Beta2Reason

	// MachineNodeConnectionDownV1Beta2Reason surfaces that the connection to the workload cluster is down.
	MachineNodeConnectionDownV1Beta2Reason = ConnectionDownV1Beta2Reason
)

// Machine's HealthCheckSucceeded condition and corresponding reasons that will be used in v1Beta2 API version.
// Note: HealthCheckSucceeded condition is set by the MachineHealthCheck controller.
const (
	// MachineHealthCheckSucceededV1Beta2Condition is true if MHC instances targeting this machine report the Machine
	// is healthy according to the definition of healthy present in the spec of the MachineHealthCheck object.
	MachineHealthCheckSucceededV1Beta2Condition = "HealthCheckSucceeded"

	// MachineHealthCheckSucceededV1Beta2Reason surfaces when a machine passes all the health checks defined by a MachineHealthCheck object.
	MachineHealthCheckSucceededV1Beta2Reason = "HealthCheckSucceeded"

	// MachineHealthCheckUnhealthyNodeV1Beta2Reason surfaces when the node hosted on the machine does not pass the health checks
	// defined by a MachineHealthCheck object.
	MachineHealthCheckUnhealthyNodeV1Beta2Reason = "UnhealthyNode"

	// MachineHealthCheckNodeStartupTimeoutV1Beta2Reason surfaces when the node hosted on the machine does not appear within
	// the timeout defined by a MachineHealthCheck object.
	MachineHealthCheckNodeStartupTimeoutV1Beta2Reason = "NodeStartupTimeout"

	// MachineHealthCheckNodeDeletedV1Beta2Reason surfaces when a MachineHealthCheck detects that the node hosted on the
	// machine has been deleted while the Machine is still running.
	MachineHealthCheckNodeDeletedV1Beta2Reason = "NodeDeleted"

	// MachineHealthCheckHasRemediateAnnotationV1Beta2Reason surfaces when a MachineHealthCheck detects that a Machine was
	// marked for remediation via the `cluster.x-k8s.io/remediate-machine` annotation.
	MachineHealthCheckHasRemediateAnnotationV1Beta2Reason = "HasRemediateAnnotation"
)

// Machine's OwnerRemediated conditions and corresponding reasons that will be used in v1Beta2 API version.
// Note: OwnerRemediated condition is initially set by the MachineHealthCheck controller; then it is up to the Machine's
// owner controller to update or delete this condition.
const (
	// MachineOwnerRemediatedV1Beta2Condition is only present if MHC instances targeting this machine
	// determine that the controller owning this machine should perform remediation.
	MachineOwnerRemediatedV1Beta2Condition = "OwnerRemediated"

	// MachineOwnerRemediatedWaitingForRemediationV1Beta2Reason surfaces the machine is waiting for the owner controller
	// to start remediation.
	MachineOwnerRemediatedWaitingForRemediationV1Beta2Reason = "WaitingForRemediation"
)

// Machine's ExternallyRemediated conditions and corresponding reasons that will be used in v1Beta2 API version.
// Note: ExternallyRemediated condition is initially set by the MachineHealthCheck controller; then it is up to the external
// remediation controller to update or delete this condition.
const (
	// MachineExternallyRemediatedV1Beta2Condition is only present if MHC instances targeting this machine
	// determine that an external controller should perform remediation.
	MachineExternallyRemediatedV1Beta2Condition = "ExternallyRemediated"

	// MachineExternallyRemediatedWaitingForRemediationV1Beta2Reason surfaces the machine is waiting for the
	// external remediation controller to start remediation.
	MachineExternallyRemediatedWaitingForRemediationV1Beta2Reason = "WaitingForRemediation"

	// MachineExternallyRemediatedRemediationTemplateNotFoundV1Beta2Reason surfaces that the MachineHealthCheck cannot
	// find the template for an external remediation request.
	MachineExternallyRemediatedRemediationTemplateNotFoundV1Beta2Reason = "RemediationTemplateNotFound"

	// MachineExternallyRemediatedRemediationRequestCreationFailedV1Beta2Reason surfaces that the MachineHealthCheck cannot
	// create a request for the external remediation controller.
	MachineExternallyRemediatedRemediationRequestCreationFailedV1Beta2Reason = "RemediationRequestCreationFailed"
)

// Machine's Deleting condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// MachineDeletingV1Beta2Condition surfaces details about progress in the machine deletion workflow.
	MachineDeletingV1Beta2Condition = DeletingV1Beta2Condition

	// MachineNotDeletingV1Beta2Reason surfaces when the Machine is not deleting because the
	// DeletionTimestamp is not set.
	MachineNotDeletingV1Beta2Reason = NotDeletingV1Beta2Reason

	// MachineDeletingV1Beta2Reason surfaces when the Machine is deleting because the
	// DeletionTimestamp is set. This reason is used if none of the more specific reasons apply.
	MachineDeletingV1Beta2Reason = DeletingV1Beta2Reason

	// MachineDeletingInternalErrorV1Beta2Reason surfaces unexpected failures when deleting a Machine.
	MachineDeletingInternalErrorV1Beta2Reason = InternalErrorV1Beta2Reason

	// MachineDeletingWaitingForPreDrainHookV1Beta2Reason surfaces when the Machine deletion
	// waits for pre-drain hooks to complete. I.e. it waits until there are no annotations
	// with the `pre-drain.delete.hook.machine.cluster.x-k8s.io` prefix on the Machine anymore.
	MachineDeletingWaitingForPreDrainHookV1Beta2Reason = "WaitingForPreDrainHook"

	// MachineDeletingDrainingNodeV1Beta2Reason surfaces when the Machine deletion is draining the Node.
	MachineDeletingDrainingNodeV1Beta2Reason = "DrainingNode"

	// MachineDeletingWaitingForVolumeDetachV1Beta2Reason surfaces when the Machine deletion is
	// waiting for volumes to detach from the Node.
	MachineDeletingWaitingForVolumeDetachV1Beta2Reason = "WaitingForVolumeDetach"

	// MachineDeletingWaitingForPreTerminateHookV1Beta2Reason surfaces when the Machine deletion
	// waits for pre-terminate hooks to complete. I.e. it waits until there are no annotations
	// with the `pre-terminate.delete.hook.machine.cluster.x-k8s.io` prefix on the Machine anymore.
	MachineDeletingWaitingForPreTerminateHookV1Beta2Reason = "WaitingForPreTerminateHook"

	// MachineDeletingWaitingForInfrastructureDeletionV1Beta2Reason surfaces when the Machine deletion
	// waits for InfraMachine deletion to complete.
	MachineDeletingWaitingForInfrastructureDeletionV1Beta2Reason = "WaitingForInfrastructureDeletion"

	// MachineDeletingWaitingForBootstrapDeletionV1Beta2Reason surfaces when the Machine deletion
	// waits for BootstrapConfig deletion to complete.
	MachineDeletingWaitingForBootstrapDeletionV1Beta2Reason = "WaitingForBootstrapDeletion"

	// MachineDeletingDeletingNodeV1Beta2Reason surfaces when the Machine deletion is
	// deleting the Node.
	MachineDeletingDeletingNodeV1Beta2Reason = "DeletingNode"

	// MachineDeletingDeletionCompletedV1Beta2Reason surfaces when the Machine deletion has been completed.
	// This reason is set right after the `machine.cluster.x-k8s.io` finalizer is removed.
	// This means that the object will go away (i.e. be removed from etcd), except if there are other
	// finalizers on the Machine object.
	MachineDeletingDeletionCompletedV1Beta2Reason = DeletionCompletedV1Beta2Reason
)

// ANCHOR: MachineSpec

// MachineSpec defines the desired state of Machine.
type MachineSpec struct {
	// clusterName is the name of the Cluster this object belongs to.
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// bootstrap is a reference to a local struct which encapsulates
	// fields to configure the Machine’s bootstrapping mechanism.
	Bootstrap Bootstrap `json:"bootstrap"`

	// infrastructureRef is a required reference to a custom resource
	// offered by an infrastructure provider.
	InfrastructureRef corev1.ObjectReference `json:"infrastructureRef"`

	// version defines the desired Kubernetes version.
	// This field is meant to be optionally used by bootstrap providers.
	// +optional
	Version *string `json:"version,omitempty"`

	// providerID is the identification ID of the machine provided by the provider.
	// This field must match the provider ID as seen on the node object corresponding to this machine.
	// This field is required by higher level consumers of cluster-api. Example use case is cluster autoscaler
	// with cluster-api as provider. Clean-up logic in the autoscaler compares machines to nodes to find out
	// machines at provider which could not get registered as Kubernetes nodes. With cluster-api as a
	// generic out-of-tree provider for autoscaler, this field is required by autoscaler to be
	// able to have a provider view of the list of machines. Another list of nodes is queried from the k8s apiserver
	// and then a comparison is done to find out unregistered machines and are marked for delete.
	// This field will be set by the actuators and consumed by higher level entities like autoscaler that will
	// be interfacing with cluster-api as generic provider.
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// failureDomain is the failure domain the machine will be created in.
	// Must match a key in the FailureDomains map stored on the cluster object.
	// +optional
	FailureDomain *string `json:"failureDomain,omitempty"`

	// The minimum number of seconds for which a Machine should be ready before considering it available.
	// Defaults to 0 (Machine will be considered available as soon as the Machine is ready)
	// NOTE: this field will be considered only for computing v1beta2 conditions.
	// +optional
	// TODO: This field will be added in the v1beta2 API, and act as a replacement of existing MinReadySeconds in
	//  MachineDeployment, MachineSet and MachinePool
	// MinReadySeconds int32 `json:"minReadySeconds,omitempty"`

	// readinessGates specifies additional conditions to include when evaluating Machine Ready condition.
	//
	// This field can be used e.g. by Cluster API control plane providers to extend the semantic of the
	// Ready condition for the Machine they control, like the kubeadm control provider adding ReadinessGates
	// for the APIServerPodHealthy, SchedulerPodHealthy conditions, etc.
	//
	// Another example are external controllers, e.g. responsible to install special software/hardware on the Machines;
	// they can include the status of those components with a new condition and add this condition to ReadinessGates.
	//
	// NOTE: This field is considered only for computing v1beta2 conditions.
	// NOTE: In case readinessGates conditions start with the APIServer, ControllerManager, Scheduler prefix, and all those
	// readiness gates condition are reporting the same message, when computing the Machine's Ready condition those
	// readinessGates will be replaced by a single entry reporting "Control plane components: " + message.
	// This helps to improve readability of conditions bubbling up to the Machine's owner resource / to the Cluster).
	// +optional
	// +listType=map
	// +listMapKey=conditionType
	// +kubebuilder:validation:MaxItems=32
	ReadinessGates []MachineReadinessGate `json:"readinessGates,omitempty"`

	// nodeDrainTimeout is the total amount of time that the controller will spend on draining a node.
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`
	// +optional
	NodeDrainTimeout *metav1.Duration `json:"nodeDrainTimeout,omitempty"`

	// nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// +optional
	NodeVolumeDetachTimeout *metav1.Duration `json:"nodeVolumeDetachTimeout,omitempty"`

	// nodeDeletionTimeout defines how long the controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// Defaults to 10 seconds.
	// +optional
	NodeDeletionTimeout *metav1.Duration `json:"nodeDeletionTimeout,omitempty"`
}

// MachineReadinessGate contains the type of a Machine condition to be used as a readiness gate.
type MachineReadinessGate struct {
	// conditionType refers to a positive polarity condition (status true means good) with matching type in the Machine's condition list.
	// If the conditions doesn't exist, it will be treated as unknown.
	// Note: Both Cluster API conditions or conditions added by 3rd party controllers can be used as readiness gates.
	// +required
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
	// +kubebuilder:validation:MaxLength=316
	// +kubebuilder:validation:MinLength=1
	ConditionType string `json:"conditionType"`
}

// ANCHOR_END: MachineSpec

// ANCHOR: MachineStatus

// MachineStatus defines the observed state of Machine.
type MachineStatus struct {
	// nodeRef will point to the corresponding Node if it exists.
	// +optional
	NodeRef *corev1.ObjectReference `json:"nodeRef,omitempty"`

	// nodeInfo is a set of ids/uuids to uniquely identify the node.
	// More info: https://kubernetes.io/docs/concepts/nodes/node/#info
	// +optional
	NodeInfo *corev1.NodeSystemInfo `json:"nodeInfo,omitempty"`

	// lastUpdated identifies when the phase of the Machine last transitioned.
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// failureReason will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a succinct value suitable
	// for machine interpretation.
	//
	// This field should not be set for transitive errors that a controller
	// faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	//
	// Deprecated: This field is deprecated and is going to be removed in the next apiVersion. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	FailureReason *capierrors.MachineStatusError `json:"failureReason,omitempty"`

	// failureMessage will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a more verbose string suitable
	// for logging and human consumption.
	//
	// This field should not be set for transitive errors that a controller
	// faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	//
	// Deprecated: This field is deprecated and is going to be removed in the next apiVersion. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// addresses is a list of addresses assigned to the machine.
	// This field is copied from the infrastructure provider reference.
	// +optional
	Addresses MachineAddresses `json:"addresses,omitempty"`

	// phase represents the current phase of machine actuation.
	// E.g. Pending, Running, Terminating, Failed etc.
	// +optional
	Phase string `json:"phase,omitempty"`

	// certificatesExpiryDate is the expiry date of the machine certificates.
	// This value is only set for control plane machines.
	// +optional
	CertificatesExpiryDate *metav1.Time `json:"certificatesExpiryDate,omitempty"`

	// bootstrapReady is the state of the bootstrap provider.
	// +optional
	BootstrapReady bool `json:"bootstrapReady"`

	// infrastructureReady is the state of the infrastructure provider.
	// +optional
	InfrastructureReady bool `json:"infrastructureReady"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions defines current service state of the Machine.
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`

	// deletion contains information relating to removal of the Machine.
	// Only present when the Machine has a deletionTimestamp and drain or wait for volume detach started.
	// +optional
	Deletion *MachineDeletionStatus `json:"deletion,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in Machine's status with the V1Beta2 version.
	// +optional
	V1Beta2 *MachineV1Beta2Status `json:"v1beta2,omitempty"`
}

// MachineV1Beta2Status groups all the fields that will be added or modified in MachineStatus with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachineV1Beta2Status struct {
	// conditions represents the observations of a Machine's current state.
	// Known condition types are Available, Ready, UpToDate, BootstrapConfigReady, InfrastructureReady, NodeReady,
	// NodeHealthy, Deleting, Paused.
	// If a MachineHealthCheck is targeting this machine, also HealthCheckSucceeded, OwnerRemediated conditions are added.
	// Additionally control plane Machines controlled by KubeadmControlPlane will have following additional conditions:
	// APIServerPodHealthy, ControllerManagerPodHealthy, SchedulerPodHealthy, EtcdPodHealthy, EtcdMemberHealthy.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ANCHOR_END: MachineStatus

// MachineDeletionStatus is the deletion state of the Machine.
type MachineDeletionStatus struct {
	// nodeDrainStartTime is the time when the drain of the node started and is used to determine
	// if the NodeDrainTimeout is exceeded.
	// Only present when the Machine has a deletionTimestamp and draining the node had been started.
	// +optional
	NodeDrainStartTime *metav1.Time `json:"nodeDrainStartTime,omitempty"`

	// waitForNodeVolumeDetachStartTime is the time when waiting for volume detachment started
	// and is used to determine if the NodeVolumeDetachTimeout is exceeded.
	// Detaching volumes from nodes is usually done by CSI implementations and the current state
	// is observed from the node's `.Status.VolumesAttached` field.
	// Only present when the Machine has a deletionTimestamp and waiting for volume detachments had been started.
	// +optional
	WaitForNodeVolumeDetachStartTime *metav1.Time `json:"waitForNodeVolumeDetachStartTime,omitempty"`
}

// SetTypedPhase sets the Phase field to the string representation of MachinePhase.
func (m *MachineStatus) SetTypedPhase(p MachinePhase) {
	m.Phase = string(p)
}

// GetTypedPhase attempts to parse the Phase field and return
// the typed MachinePhase representation as described in `machine_phase_types.go`.
func (m *MachineStatus) GetTypedPhase() MachinePhase {
	switch phase := MachinePhase(m.Phase); phase {
	case
		MachinePhasePending,
		MachinePhaseProvisioning,
		MachinePhaseProvisioned,
		MachinePhaseRunning,
		MachinePhaseDeleting,
		MachinePhaseDeleted,
		MachinePhaseFailed:
		return phase
	default:
		return MachinePhaseUnknown
	}
}

// ANCHOR: Bootstrap

// Bootstrap encapsulates fields to configure the Machine’s bootstrapping mechanism.
type Bootstrap struct {
	// configRef is a reference to a bootstrap provider-specific resource
	// that holds configuration details. The reference is optional to
	// allow users/operators to specify Bootstrap.DataSecretName without
	// the need of a controller.
	// +optional
	ConfigRef *corev1.ObjectReference `json:"configRef,omitempty"`

	// dataSecretName is the name of the secret that stores the bootstrap data script.
	// If nil, the Machine should remain in the Pending state.
	// +optional
	DataSecretName *string `json:"dataSecretName,omitempty"`
}

// ANCHOR_END: Bootstrap

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=machines,shortName=ma,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName",description="Cluster"
// +kubebuilder:printcolumn:name="NodeName",type="string",JSONPath=".status.nodeRef.name",description="Node name associated with this machine"
// +kubebuilder:printcolumn:name="ProviderID",type="string",JSONPath=".spec.providerID",description="Provider ID"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Machine status such as Terminating/Pending/Running/Failed etc"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of Machine"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.version",description="Kubernetes version associated with this Machine"

// Machine is the Schema for the machines API.
type Machine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineSpec   `json:"spec,omitempty"`
	Status MachineStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (m *Machine) GetConditions() Conditions {
	return m.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (m *Machine) SetConditions(conditions Conditions) {
	m.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (m *Machine) GetV1Beta2Conditions() []metav1.Condition {
	if m.Status.V1Beta2 == nil {
		return nil
	}
	return m.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (m *Machine) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if m.Status.V1Beta2 == nil {
		m.Status.V1Beta2 = &MachineV1Beta2Status{}
	}
	m.Status.V1Beta2.Conditions = conditions
}

// +kubebuilder:object:root=true

// MachineList contains a list of Machine.
type MachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Machine `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &Machine{}, &MachineList{})
}
