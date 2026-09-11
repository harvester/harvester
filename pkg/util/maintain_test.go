package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetMaintenanceModeCondition(t *testing.T) {
	initialTime := metav1.NewTime(time.Now().Add(-time.Minute))
	node := &corev1.Node{Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
		{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
		{Type: NodeConditionTypeMaintenanceMode, Status: corev1.ConditionTrue, Reason: NodeConditionReasonValidating, Message: "validating", LastTransitionTime: initialTime},
	}}}

	assert.False(t, SetMaintenanceModeCondition(node, corev1.ConditionTrue, NodeConditionReasonValidating, "validating"))
	assert.True(t, node.Status.Conditions[1].LastTransitionTime.Equal(&initialTime))

	assert.True(t, SetMaintenanceModeCondition(node, corev1.ConditionTrue, NodeConditionReasonDraining, "draining"))
	assert.True(t, node.Status.Conditions[1].LastTransitionTime.After(initialTime.Time))
	assert.Equal(t, NodeConditionReasonDraining, node.Status.Conditions[1].Reason)
	assert.Equal(t, corev1.NodeReady, node.Status.Conditions[0].Type)
}

func TestMaintenanceModeConditionHelpers(t *testing.T) {
	node := &corev1.Node{}
	assert.Nil(t, GetMaintenanceModeCondition(node))
	assert.False(t, IsMaintenanceModeEngaged(node))
	assert.False(t, RemoveMaintenanceModeCondition(node))

	assert.True(t, SetMaintenanceModeCondition(node, corev1.ConditionFalse, NodeConditionReasonError, "failed"))
	condition := GetMaintenanceModeCondition(node)
	assert.False(t, IsMaintenanceModeEngaged(node))
	assert.True(t, IsMaintenanceModeCondition(condition, corev1.ConditionFalse, NodeConditionReasonError))
	assert.False(t, CanDisableMaintenanceMode(condition))

	assert.True(t, SetMaintenanceModeCondition(node, corev1.ConditionTrue, NodeConditionReasonError, "timed out"))
	condition = GetMaintenanceModeCondition(node)
	assert.True(t, IsMaintenanceModeEngaged(node))
	assert.True(t, CanDisableMaintenanceMode(condition))
	assert.False(t, IsMaintenanceModeDrainComplete(condition))

	assert.True(t, SetMaintenanceModeCondition(node, corev1.ConditionTrue, NodeConditionReasonCompleted, "complete"))
	assert.True(t, IsMaintenanceModeDrainComplete(GetMaintenanceModeCondition(node)))
	assert.True(t, RemoveMaintenanceModeCondition(node))
	assert.Nil(t, GetMaintenanceModeCondition(node))
}

type fakeNodeStatusClient struct {
	node *corev1.Node
}

func (f *fakeNodeStatusClient) Get(_ string, _ metav1.GetOptions) (*corev1.Node, error) {
	return f.node.DeepCopy(), nil
}

func (f *fakeNodeStatusClient) UpdateStatus(node *corev1.Node) (*corev1.Node, error) {
	f.node = node.DeepCopy()
	return f.node, nil
}

func TestUpdateAndTransitionMaintenanceModeCondition(t *testing.T) {
	client := &fakeNodeStatusClient{
		node: &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		},
	}

	// Initial transition from None -> Validating
	updated, err := UpdateMaintenanceModeCondition(client, "node1", corev1.ConditionTrue, NodeConditionReasonValidating, MaintenanceModeMessageValidating)
	assert.NoError(t, err)
	assert.True(t, IsMaintenanceModePhase(updated, corev1.ConditionTrue, NodeConditionReasonValidating))

	// Transition from Validating -> Draining
	updated, err = TransitionMaintenanceModeCondition(client, "node1",
		corev1.ConditionTrue, NodeConditionReasonValidating,
		corev1.ConditionTrue, NodeConditionReasonDraining, MaintenanceModeMessageDraining)
	assert.NoError(t, err)
	assert.True(t, IsMaintenanceModePhase(updated, corev1.ConditionTrue, NodeConditionReasonDraining))

	// Stale transition should not apply
	updated, err = TransitionMaintenanceModeCondition(client, "node1",
		corev1.ConditionTrue, NodeConditionReasonValidating,
		corev1.ConditionTrue, NodeConditionReasonEvacuating, MaintenanceModeMessageEvacuating)
	assert.NoError(t, err)
	assert.True(t, IsMaintenanceModePhase(updated, corev1.ConditionTrue, NodeConditionReasonDraining))

	// Expected transition Draining -> Evacuating
	updated, err = TransitionMaintenanceModeCondition(client, "node1",
		corev1.ConditionTrue, NodeConditionReasonDraining,
		corev1.ConditionTrue, NodeConditionReasonEvacuating, MaintenanceModeMessageEvacuating)
	assert.NoError(t, err)
	assert.True(t, IsMaintenanceModePhase(updated, corev1.ConditionTrue, NodeConditionReasonEvacuating))

	// Evacuating -> Completed
	updated, err = TransitionMaintenanceModeCondition(client, "node1",
		corev1.ConditionTrue, NodeConditionReasonEvacuating,
		corev1.ConditionTrue, NodeConditionReasonCompleted, MaintenanceModeMessageCompleted)
	assert.NoError(t, err)
	assert.True(t, IsMaintenanceModePhase(updated, corev1.ConditionTrue, NodeConditionReasonCompleted))
}
