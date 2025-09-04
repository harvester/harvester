package util

import (
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_AddOrUpdateConditionToNode_Add(t *testing.T) {
	node := &corev1.Node{
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{},
		},
	}
	AddOrUpdateConditionToNode(node, corev1.NodeCondition{
		Type:               NodeConditionTypeMaintenanceMode,
		Status:             corev1.ConditionTrue,
		LastHeartbeatTime:  metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             NodeConditionReasonCompleted,
		Message:            "Draining the node is completed",
	})
	assert.Len(t, node.Status.Conditions, 1)
}

func Test_AddOrUpdateConditionToNode_Update(t *testing.T) {
	node := &corev1.Node{
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:               NodeConditionTypeMaintenanceMode,
					Status:             corev1.ConditionTrue,
					LastHeartbeatTime:  metav1.Now(),
					LastTransitionTime: metav1.Now(),
					Reason:             NodeConditionReasonRunning,
					Message:            "Draining the node is running",
				},
			},
		},
	}
	AddOrUpdateConditionToNode(node, corev1.NodeCondition{
		Type:               NodeConditionTypeMaintenanceMode,
		Status:             corev1.ConditionTrue,
		LastHeartbeatTime:  metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             NodeConditionReasonCompleted,
		Message:            "Draining the node is completed",
	})
	assert.Len(t, node.Status.Conditions, 1)
	assert.Equal(t, node.Status.Conditions[0].Reason, NodeConditionReasonCompleted)
}
