package util

import (
	"fmt"
	"slices"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/retry"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

// Maintenance mode conditions use True while a node is unavailable for
// maintenance. False/Error is a side-effect-free pre-check failure.
const (
	NodeConditionTypeMaintenanceMode corev1.NodeConditionType = "MaintenanceMode"

	NodeConditionReasonValidating = "Validating"
	NodeConditionReasonDraining   = "Draining"
	NodeConditionReasonEvacuating = "Evacuating"
	NodeConditionReasonCompleted  = "Completed"
	NodeConditionReasonError      = "Error"

	MaintenanceModeMessageValidating = "Checking whether the node can enter maintenance mode"
	MaintenanceModeMessageDraining   = "Draining the node"
	MaintenanceModeMessageEvacuating = "Waiting for VM migration and restart handling to complete"
	MaintenanceModeMessageCompleted  = "Maintenance mode enabled"
)

// NodeStatusClient provides the status operations needed to update a
// MaintenanceMode condition without depending on a generated controller type.
type NodeStatusClient interface {
	Get(name string, options metav1.GetOptions) (*corev1.Node, error)
	UpdateStatus(node *corev1.Node) (*corev1.Node, error)
}

// GetMaintenanceModeCondition returns the node's MaintenanceMode condition, or
// nil when the node is nil or has not entered the maintenance state machine.
func GetMaintenanceModeCondition(node *corev1.Node) *corev1.NodeCondition {
	if node == nil {
		return nil
	}
	return FindNodeStatusCondition(node.Status.Conditions, NodeConditionTypeMaintenanceMode)
}

// IsMaintenanceModeCondition reports whether condition has the requested
// status and one of the supplied lifecycle reasons.
func IsMaintenanceModeCondition(condition *corev1.NodeCondition, status corev1.ConditionStatus, reasons ...string) bool {
	return condition != nil && condition.Status == status && slices.Contains(reasons, condition.Reason)
}

// IsMaintenanceModePhase reports whether node is in the requested maintenance
// condition status and lifecycle phase.
func IsMaintenanceModePhase(node *corev1.Node, status corev1.ConditionStatus, reasons ...string) bool {
	return IsMaintenanceModeCondition(GetMaintenanceModeCondition(node), status, reasons...)
}

// IsMaintenanceModeEngaged reports whether maintenance may have made the node
// unavailable. True/Error remains engaged because drain side effects may exist.
func IsMaintenanceModeEngaged(node *corev1.Node) bool {
	condition := GetMaintenanceModeCondition(node)
	return condition != nil && condition.Status == corev1.ConditionTrue
}

// IsMaintenanceModeError reports whether condition represents a maintenance
// failure. The condition status distinguishes pre-check from drain failures.
func IsMaintenanceModeError(condition *corev1.NodeCondition) bool {
	return condition != nil && condition.Reason == NodeConditionReasonError
}

// CanDisableMaintenanceMode reports whether the maintenance phase supports an
// explicit disable operation.
func CanDisableMaintenanceMode(condition *corev1.NodeCondition) bool {
	return IsMaintenanceModeCondition(condition, corev1.ConditionTrue,
		NodeConditionReasonEvacuating, NodeConditionReasonCompleted, NodeConditionReasonError)
}

// IsMaintenanceModeDrainComplete reports whether DrainNode completed. It is
// true while post-drain VM handling runs and after maintenance is completed.
func IsMaintenanceModeDrainComplete(condition *corev1.NodeCondition) bool {
	return IsMaintenanceModeCondition(condition, corev1.ConditionTrue,
		NodeConditionReasonEvacuating, NodeConditionReasonCompleted)
}

// SetMaintenanceModeCondition updates node's MaintenanceMode condition and
// returns whether it changed. Status or lifecycle reason changes update
// LastTransitionTime; an unchanged condition does not refresh status fields.
func SetMaintenanceModeCondition(node *corev1.Node, status corev1.ConditionStatus, reason, message string) bool {
	now := metav1.Now()

	for i := range node.Status.Conditions {
		condition := &node.Status.Conditions[i]
		if condition.Type != NodeConditionTypeMaintenanceMode {
			continue
		}

		if condition.Status == status && condition.Reason == reason && condition.Message == message {
			return false
		}
		if condition.Status != status || condition.Reason != reason {
			condition.LastTransitionTime = now
		}
		condition.LastHeartbeatTime = now
		condition.Status = status
		condition.Reason = reason
		condition.Message = message
		return true
	}

	node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
		Type:               NodeConditionTypeMaintenanceMode,
		Status:             status,
		LastTransitionTime: now,
		LastHeartbeatTime:  now,
		Reason:             reason,
		Message:            message,
	})
	return true
}

// RemoveMaintenanceModeCondition removes node's MaintenanceMode condition and
// returns whether the condition was present.
func RemoveMaintenanceModeCondition(node *corev1.Node) bool {
	conditions := slices.DeleteFunc(node.Status.Conditions, func(condition corev1.NodeCondition) bool {
		return condition.Type == NodeConditionTypeMaintenanceMode
	})
	if len(conditions) == len(node.Status.Conditions) {
		return false
	}
	node.Status.Conditions = conditions
	return true
}

// UpdateMaintenanceModeCondition fetches the latest node, merges only its
// MaintenanceMode condition, and writes status with conflict retry. Fetching on
// every attempt preserves kubelet-owned conditions and unrelated status fields.
func UpdateMaintenanceModeCondition(client NodeStatusClient, nodeName string, status corev1.ConditionStatus, reason, message string) (*corev1.Node, error) {
	logrus.WithFields(logrus.Fields{
		"node":    nodeName,
		"status":  status,
		"reason":  reason,
		"message": message,
	}).Info("Updating node maintenance mode condition")

	var updated *corev1.Node

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := client.Get(nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}

		node = node.DeepCopy()
		if !SetMaintenanceModeCondition(node, status, reason, message) {
			updated = node
			return nil
		}

		updated, err = client.UpdateStatus(node)
		return err
	})
	return updated, err
}

// TransitionMaintenanceModeCondition changes a MaintenanceMode condition only
// when the latest node is still in the expected source phase. This prevents a
// stale reconcile from overwriting a concurrent disable or later transition.
func TransitionMaintenanceModeCondition(client NodeStatusClient, nodeName string, fromStatus corev1.ConditionStatus, fromReason string,
	toStatus corev1.ConditionStatus, toReason, message string) (*corev1.Node, error) {
	logrus.WithFields(logrus.Fields{
		"node":       nodeName,
		"fromStatus": fromStatus,
		"fromReason": fromReason,
		"toStatus":   toStatus,
		"toReason":   toReason,
		"message":    message,
	}).Info("Transitioning node maintenance mode condition")

	var updated *corev1.Node

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := client.Get(nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !IsMaintenanceModePhase(node, fromStatus, fromReason) {
			updated = node
			return nil
		}

		node = node.DeepCopy()

		SetMaintenanceModeCondition(node, toStatus, toReason, message)
		updated, err = client.UpdateStatus(node)
		return err
	})
	return updated, err
}

// RestartMaintenanceModeVMsOnNode restarts all VMs labeled with the given maintenance mode strategy
// that were associated with the specified node, and removes the node tracking annotation.
// If a VM is already running or not in Halted state, its run strategy is left unchanged.
func RestartMaintenanceModeVMsOnNode(vmClient ctlkubevirtv1.VirtualMachineClient, vmCache ctlkubevirtv1.VirtualMachineCache, nodeName, strategy string) error {
	selector := labels.Set{LabelMaintainModeStrategy: strategy}.AsSelector()
	vmList, err := vmCache.List(corev1.NamespaceAll, selector)
	if err != nil {
		return fmt.Errorf("failed to list VMs with labels %s: %w", selector.String(), err)
	}

	for _, vm := range vmList {
		if vm.Annotations == nil || vm.Annotations[AnnotationMaintainModeStrategyNodeName] != nodeName {
			continue
		}

		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			// Fetch the latest VM version from the API server on every attempt to ensure
			// a fresh resourceVersion and avoid overwriting concurrent modifications.
			latestVM, err := vmClient.Get(vm.Namespace, vm.Name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
				return err
			}
			if latestVM.Annotations == nil || latestVM.Annotations[AnnotationMaintainModeStrategyNodeName] != nodeName {
				return nil
			}

			toUpdate := latestVM.DeepCopy()
			delete(toUpdate.Annotations, AnnotationMaintainModeStrategyNodeName)

			currentStrategy, err := toUpdate.RunStrategy()
			if err != nil {
				return err
			}

			if currentStrategy == kubevirtv1.RunStrategyHalted {
				runStrategy := kubevirtv1.VirtualMachineRunStrategy(toUpdate.Annotations[AnnotationRunStrategy])
				if runStrategy == "" {
					runStrategy = kubevirtv1.RunStrategyRerunOnFailure
				}
				logrus.WithFields(logrus.Fields{
					"namespace":   toUpdate.Namespace,
					"name":        toUpdate.Name,
					"runStrategy": runStrategy,
				}).Info("Restarting VM that was shut down for maintenance mode")
				toUpdate.Spec.RunStrategy = new(runStrategy)
			} else {
				logrus.WithFields(logrus.Fields{
					"namespace":       toUpdate.Namespace,
					"name":            toUpdate.Name,
					"currentStrategy": currentStrategy,
				}).Info("VM is not halted, skipping restart")
			}

			_, err = vmClient.Update(toUpdate)
			return err
		})
		if err != nil {
			return fmt.Errorf("failed to process VM %s during restart: %w", GetNamespacedName(vm), err)
		}
	}

	return nil
}
