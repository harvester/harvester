package util

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewNodes(nodeNames ...string) []*corev1.Node {
	var nodes []*corev1.Node
	for _, nn := range nodeNames {
		nodes = append(nodes, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nn}})
	}
	return nodes
}
