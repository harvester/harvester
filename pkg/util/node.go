package util

import (
	corev1 "k8s.io/api/core/v1"
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

func AddOrUpdateConditionToNode(node *corev1.Node, newCondition corev1.NodeCondition) {
	found := false
	for i := range node.Status.Conditions {
		c := &node.Status.Conditions[i]
		if c.Type == newCondition.Type {
			found = true
			c.Status = newCondition.Status
			c.LastHeartbeatTime = newCondition.LastHeartbeatTime
			c.LastTransitionTime = newCondition.LastTransitionTime
			c.Reason = newCondition.Reason
			c.Message = newCondition.Message
		}
	}
	if !found {
		node.Status.Conditions = append(node.Status.Conditions, newCondition)
	}
}

func RemoveConditionFromNode(node *corev1.Node, condType corev1.NodeConditionType) {
	newConditions := make([]corev1.NodeCondition, 0, len(node.Status.Conditions))
	for _, c := range node.Status.Conditions {
		if c.Type != condType {
			newConditions = append(newConditions, c)
		}
	}
	node.Status.Conditions = newConditions
}

func GetConditionFromNode(node *corev1.Node, condType corev1.NodeConditionType) *corev1.NodeCondition {
	for _, c := range node.Status.Conditions {
		if c.Type == condType {
			return &c
		}
	}
	return nil
}
