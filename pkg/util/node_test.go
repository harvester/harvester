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
		Message:            "Maintenance mode enabled",
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
					Message:            "Draining the node",
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
		Message:            "Maintenance mode enabled",
	})
	assert.Len(t, node.Status.Conditions, 1)
	assert.Equal(t, node.Status.Conditions[0].Reason, NodeConditionReasonCompleted)
}

func Test_RemoveConditionFromNode(t *testing.T) {
	node := &corev1.Node{
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:               NodeConditionTypeMaintenanceMode,
					Status:             corev1.ConditionTrue,
					LastHeartbeatTime:  metav1.Now(),
					LastTransitionTime: metav1.Now(),
					Reason:             NodeConditionReasonCompleted,
					Message:            "Maintenance mode enabled",
				},
				{
					Type:               "foo",
					Status:             corev1.ConditionFalse,
					LastHeartbeatTime:  metav1.Now(),
					LastTransitionTime: metav1.Now(),
					Reason:             "bar",
					Message:            "Qui aromata regit, universum regit",
				},
			},
		},
	}
	removed := RemoveConditionFromNode(node, NodeConditionTypeMaintenanceMode)
	assert.Len(t, node.Status.Conditions, 1)
	assert.True(t, removed)
}

func Test_GetConditionFromNode_1(t *testing.T) {
	node := &corev1.Node{
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:               NodeConditionTypeMaintenanceMode,
					Status:             corev1.ConditionTrue,
					LastHeartbeatTime:  metav1.Now(),
					LastTransitionTime: metav1.Now(),
					Reason:             NodeConditionReasonCompleted,
					Message:            "Maintenance mode enabled",
				},
			},
		},
	}
	condition := GetConditionFromNode(node, NodeConditionTypeMaintenanceMode)
	assert.NotNil(t, condition)
	assert.Equal(t, condition.Type, NodeConditionTypeMaintenanceMode)
}

func Test_GetConditionFromNode_2(t *testing.T) {
	node := &corev1.Node{
		Status: corev1.NodeStatus{},
	}
	condition := GetConditionFromNode(node, NodeConditionTypeMaintenanceMode)
	assert.Nil(t, condition)
}
