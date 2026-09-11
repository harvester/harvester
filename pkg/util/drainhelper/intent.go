package drainhelper

import corev1 "k8s.io/api/core/v1"

// HasDrainRequest reports whether the node has an active maintenance drain request.
func HasDrainRequest(node *corev1.Node) bool {
	return node != nil && node.Annotations[DrainAnnotation] != ""
}

// IsForcedDrainRequested reports whether the node's maintenance drain request
// requires non-migratable virtual machines to be stopped.
func IsForcedDrainRequested(node *corev1.Node) bool {
	return node != nil && node.Annotations[ForcedDrain] != ""
}

// SetDrainRequest records a maintenance drain request on the node. A non-forced
// request removes force intent left by an earlier maintenance attempt.
func SetDrainRequest(node *corev1.Node, forced bool) {
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}

	node.Annotations[DrainAnnotation] = "true"

	if forced {
		node.Annotations[ForcedDrain] = "true"
	} else {
		delete(node.Annotations, ForcedDrain)
	}
}

// ClearDrainRequest removes regular and forced maintenance drain intent from
// the node. It returns true when at least one intent annotation was present.
func ClearDrainRequest(node *corev1.Node) bool {
	if node == nil || node.Annotations == nil {
		return false
	}

	requested := HasDrainRequest(node)
	forced := IsForcedDrainRequested(node)

	delete(node.Annotations, DrainAnnotation)
	delete(node.Annotations, ForcedDrain)

	return requested || forced
}
