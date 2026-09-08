package drainhelper

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestDrainRequestIntent(t *testing.T) {
	node := &corev1.Node{}
	assert.False(t, HasDrainRequest(node))
	assert.False(t, IsForcedDrainRequested(node))
	assert.False(t, ClearDrainRequest(node))

	SetDrainRequest(node, true)
	assert.True(t, HasDrainRequest(node))
	assert.True(t, IsForcedDrainRequested(node))

	SetDrainRequest(node, false)
	assert.True(t, HasDrainRequest(node))
	assert.False(t, IsForcedDrainRequested(node))
	assert.True(t, ClearDrainRequest(node))
	assert.False(t, HasDrainRequest(node))
	assert.False(t, IsForcedDrainRequested(node))
}

func TestDrainRequestIntentNilNode(t *testing.T) {
	assert.NotPanics(t, func() { SetDrainRequest(nil, true) })
	assert.False(t, HasDrainRequest(nil))
	assert.False(t, IsForcedDrainRequested(nil))
	assert.False(t, ClearDrainRequest(nil))
}

func TestClearDrainRequestRemovesEmptyValues(t *testing.T) {
	node := &corev1.Node{}
	node.Annotations = map[string]string{DrainAnnotation: "", ForcedDrain: "", "other": "keep"}

	assert.True(t, ClearDrainRequest(node))
	assert.Equal(t, map[string]string{"other": "keep"}, node.Annotations)
	assert.False(t, ClearDrainRequest(node))
}
