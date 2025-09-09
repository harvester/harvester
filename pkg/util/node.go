package util

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	KubeNodeRoleLabelPrefix      = "node-role.kubernetes.io/"
	KubeMasterNodeLabelKey       = KubeNodeRoleLabelPrefix + "master"
	KubeControlPlaneNodeLabelKey = KubeNodeRoleLabelPrefix + "control-plane"
	KubeEtcdNodeLabelKey         = KubeNodeRoleLabelPrefix + "etcd"

	PromoteStatusComplete = "complete"
	PromoteStatusRunning  = "running"
	PromoteStatusUnknown  = "unknown"
	PromoteStatusFailed   = "failed"
)

func ExcludeWitnessNodes(nodes []*corev1.Node) []*corev1.Node {
	nonWitnessNodes := make([]*corev1.Node, 0, len(nodes))
	for _, node := range nodes {
		if _, ok := node.Labels[HarvesterWitnessNodeLabelKey]; !ok {
			nonWitnessNodes = append(nonWitnessNodes, node)
		}
	}
	return nonWitnessNodes
}

func IsPromoteStatusIn(node *corev1.Node, statuses ...string) bool {
	status, ok := node.Annotations[HarvesterPromoteStatusAnnotationKey]
	if !ok {
		return false
	}

	for _, s := range statuses {
		if status == s {
			return true
		}
	}

	return false
}

func IsWitnessNodeWithoutPromotionStatus(node *corev1.Node) bool {
	val, found := node.Labels[HarvesterWitnessNodeLabelKey]
	if found && val == "true" {
		return true
	}

	return false
}

func IsWitnessNode(node *corev1.Node, isManagement bool) bool {
	_, found := node.Labels[HarvesterWitnessNodeLabelKey]
	if !found {
		return false
	}

	// promotion has already been run for this node
	if isManagement || IsPromoteStatusIn(node, PromoteStatusComplete, PromoteStatusRunning, PromoteStatusFailed, PromoteStatusUnknown) {
		return true
	}

	return false
}

// IsManagementRole determine whether it's an management node based on the node's label.
// Management Role included: master, control-plane, etcd
func IsManagementRole(node *corev1.Node) bool {
	if value, ok := node.Labels[KubeMasterNodeLabelKey]; ok {
		return value == "true"
	}

	// Related to https://github.com/kubernetes/kubernetes/pull/95382
	if value, ok := node.Labels[KubeControlPlaneNodeLabelKey]; ok {
		return value == "true"
	}

	// Now we have the witness node, we need to count it as a management node
	if value, ok := node.Labels[KubeEtcdNodeLabelKey]; ok {
		return value == "true"
	}

	return false
}

// CountNonWitnessNodes count the number of nodes running instance manager pod
func CountNonWitnessNodes(nodes []*corev1.Node) int {
	count := 0

	for _, node := range nodes {
		if !IsWitnessNodeWithoutPromotionStatus(node) {
			count++
		}
	}

	return count
}

// SetNodeStatusCondition sets the condition of the node.
// If the condition doesn't exist, it will be added. If the condition already
// exists, it will be updated.
func SetNodeStatusCondition(node *corev1.Node, condType corev1.NodeConditionType, status corev1.ConditionStatus, reason string, message string) {
	now := metav1.Now()
	cond := FindNodeStatusCondition(node.Status.Conditions, condType)

	if cond == nil {
		node.Status.Conditions = append(node.Status.Conditions,
			corev1.NodeCondition{
				Type:               condType,
				Status:             status,
				LastTransitionTime: now,
				LastHeartbeatTime:  now,
				Reason:             reason,
				Message:            message,
			},
		)
		return
	}

	if cond.Status != status {
		cond.Status = status
		cond.LastTransitionTime = now
	}

	cond.LastHeartbeatTime = now
	cond.Reason = reason
	cond.Message = message

	for id, c := range node.Status.Conditions {
		if c.Type == condType {
			node.Status.Conditions[id] = *cond
		}
	}
}

// FindNodeStatusCondition returns the condition of the node based on the condition type.
// If the condition doesn't exist, it will return nil.
// Note: The returned pointer references a copy of the matched condition.
func FindNodeStatusCondition(conditions []corev1.NodeCondition, conditionType corev1.NodeConditionType) *corev1.NodeCondition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			c := conditions[i]
			return &c
		}
	}
	return nil
}

// RemoveNodeStatusCondition removes the condition of the node based on the condition type.
// It returns true if the condition is removed, otherwise false.
func RemoveNodeStatusCondition(node *corev1.Node, condType corev1.NodeConditionType) bool {
	numConditions := len(node.Status.Conditions)
	if numConditions == 0 {
		return false
	}

	newConditions := make([]corev1.NodeCondition, 0, numConditions)

	for _, c := range node.Status.Conditions {
		if c.Type != condType {
			newConditions = append(newConditions, c)
		}
	}

	node.Status.Conditions = newConditions

	return numConditions != len(newConditions)
}
