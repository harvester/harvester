package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetNodeCondition_NoConditions(t *testing.T) {
	cond := FindNodeStatusCondition(nil, corev1.NodeReady)
	assert.Nil(t, cond, "expected nil when no conditions provided")
}

func TestGetNodeCondition_FoundFirstMatch(t *testing.T) {
	conditions := []corev1.NodeCondition{
		{
			Type:   corev1.NodeDiskPressure,
			Status: corev1.ConditionFalse,
		},
		{
			Type:   corev1.NodeReady,
			Status: corev1.ConditionTrue,
			Reason: "ready",
		},
	}

	cond := FindNodeStatusCondition(conditions, corev1.NodeReady)
	assert.NotNil(t, cond, "expected to find NodeReady condition")
	assert.Equal(t, corev1.NodeReady, cond.Type)
	assert.Equal(t, "ready", cond.Reason)
}

func TestGetNodeCondition_MultipleSameType_ReturnsFirstAndIsPointerToSliceElement(t *testing.T) {
	conditions := []corev1.NodeCondition{
		{Type: corev1.NodeReady, Reason: "first"},
		{Type: corev1.NodeReady, Reason: "second"},
	}

	cond := FindNodeStatusCondition(conditions, corev1.NodeReady)
	assert.NotNil(t, cond, "expected to find NodeReady condition")
	assert.Equal(t, "first", cond.Reason)

	// Mutate the returned condition and ensure the original slice element was changed.
	cond.Reason = "changed"
	assert.Equal(t, "changed", conditions[0].Reason)
}

func TestSetNodeCondition_AppendsWhenMissing(t *testing.T) {
	node := &corev1.Node{}

	SetNodeStatusCondition(node, corev1.NodeReady, corev1.ConditionTrue, "reason", "message")

	assert.Len(t, node.Status.Conditions, 1, "expected one condition appended")
	c := node.Status.Conditions[0]
	assert.Equal(t, corev1.NodeReady, c.Type)
	assert.Equal(t, corev1.ConditionTrue, c.Status)
	assert.Equal(t, "reason", c.Reason)
	assert.Equal(t, "message", c.Message)
	assert.False(t, c.LastHeartbeatTime.IsZero(), "expected LastHeartbeatTime to be set")
	assert.False(t, c.LastTransitionTime.IsZero(), "expected LastTransitionTime to be set")
}

func TestSetNodeCondition_UpdatesExisting_SameStatus(t *testing.T) {
	initialTime := metav1.NewTime(time.Now())
	node := &corev1.Node{
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:               corev1.NodeReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: initialTime,
					LastHeartbeatTime:  initialTime,
					Reason:             "old-reason",
					Message:            "old-msg",
				},
			},
		},
	}

	SetNodeStatusCondition(node, corev1.NodeReady, corev1.ConditionTrue, "new-reason", "new-msg")

	assert.Len(t, node.Status.Conditions, 1)
	c := node.Status.Conditions[0]
	assert.Equal(t, corev1.ConditionTrue, c.Status)
	assert.Equal(t, "new-reason", c.Reason)
	assert.Equal(t, "new-msg", c.Message)
	// Transition time should remain the same when status unchanged.
	assert.True(t, c.LastTransitionTime.Equal(&initialTime))
	// Heartbeat should be updated to a later time.
	assert.True(t, c.LastHeartbeatTime.Time.After(initialTime.Time))
}

func TestSetNodeCondition_UpdatesExisting_StatusChanged(t *testing.T) {
	initialTime := metav1.NewTime(time.Now())
	node := &corev1.Node{
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:               corev1.NodeReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: initialTime,
					LastHeartbeatTime:  initialTime,
					Reason:             "old-reason",
					Message:            "old-msg",
				},
			},
		},
	}

	SetNodeStatusCondition(node, corev1.NodeReady, corev1.ConditionFalse, "changed-reason", "changed-msg")

	assert.Len(t, node.Status.Conditions, 1)
	c := node.Status.Conditions[0]
	assert.Equal(t, corev1.ConditionFalse, c.Status)
	assert.Equal(t, "changed-reason", c.Reason)
	assert.Equal(t, "changed-msg", c.Message)
	// Transition time should be updated when status changed.
	assert.True(t, c.LastTransitionTime.Time.After(initialTime.Time))
	// Heartbeat should also be updated.
	assert.True(t, c.LastHeartbeatTime.Time.After(initialTime.Time))
}
