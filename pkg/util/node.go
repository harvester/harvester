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

// IsNodeInMaintenanceMode checks if the node is going into or already in maintenance mode.
func IsNodeInMaintenanceMode(node *corev1.Node) bool {
	if node == nil {
		return false
	}
	_, ok := node.Annotations[AnnotationMaintainStatus]
	return ok
}
