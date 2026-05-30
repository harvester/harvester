package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterfake "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

// buildNode builds a minimal Node with the given maintenance annotation value
// (pass "" to omit the annotation) and optional existing status conditions.
func buildNode(maintainAnnotation string, conditions []corev1.NodeCondition) *corev1.Node {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-node",
			Annotations: map[string]string{},
		},
		Status: corev1.NodeStatus{
			Conditions: conditions,
		},
	}
	if maintainAnnotation != "" {
		node.Annotations[util.MaintainStatusAnnotation] = maintainAnnotation
	}
	return node
}

// maintenanceModeCondition returns a NodeCondition for MaintenanceMode with the
// given reason.
func maintenanceModeCondition(reason string) corev1.NodeCondition {
	return corev1.NodeCondition{
		Type:   util.NodeConditionTypeMaintenanceMode,
		Status: corev1.ConditionTrue,
		Reason: reason,
	}
}

// newHandler creates a minimal maintainNodeHandler wired up to a fake
// clientset that contains the given node.
func newHandler(node *corev1.Node) *maintainNodeHandler {
	cs := harvesterfake.NewSimpleClientset(node)
	return &maintainNodeHandler{
		nodes:                       fakeclients.NodeClient(cs.CoreV1().Nodes),
		virtualMachineInstanceCache: fakeclients.VirtualMachineInstanceCache(cs.KubevirtV1().VirtualMachineInstances),
		virtualMachineCache:         fakeclients.VirtualMachineCache(cs.KubevirtV1().VirtualMachines),
		virtualMachineClient:        fakeclients.VirtualMachineClient(cs.KubevirtV1().VirtualMachines),
	}
}

// Test_OnNodeChanged_NoAnnotation_NoCondition: annotation absent, no
// MaintenanceMode condition present → handler returns without calling
// UpdateStatus (no-op).
func Test_OnNodeChanged_NoAnnotation_NoCondition(t *testing.T) {
	node := buildNode("", nil)
	h := newHandler(node)

	result, err := h.OnNodeChanged("", node)
	require.NoError(t, err)
	// No condition to remove → original node returned unchanged.
	assert.Equal(t, node.Name, result.Name)
	cond := util.FindNodeStatusCondition(result.Status.Conditions, util.NodeConditionTypeMaintenanceMode)
	assert.Nil(t, cond, "expected no MaintenanceMode condition")
}

// Test_OnNodeChanged_NoAnnotation_WithNonErrorCondition: annotation absent but
// a MaintenanceMode/Completed condition is present → condition should be
// removed (UpdateStatus called).
func Test_OnNodeChanged_NoAnnotation_WithNonErrorCondition(t *testing.T) {
	node := buildNode("", []corev1.NodeCondition{
		maintenanceModeCondition(util.NodeConditionReasonCompleted),
	})
	h := newHandler(node)

	result, err := h.OnNodeChanged("", node)
	require.NoError(t, err)
	cond := util.FindNodeStatusCondition(result.Status.Conditions, util.NodeConditionTypeMaintenanceMode)
	assert.Nil(t, cond, "expected MaintenanceMode condition to be removed")
}

// Test_OnNodeChanged_NoAnnotation_WithErrorCondition: annotation absent but a
// MaintenanceMode/Error condition present → error condition must NOT be
// removed.
func Test_OnNodeChanged_NoAnnotation_WithErrorCondition(t *testing.T) {
	node := buildNode("", []corev1.NodeCondition{
		maintenanceModeCondition(util.NodeConditionReasonError),
	})
	h := newHandler(node)

	result, err := h.OnNodeChanged("", node)
	require.NoError(t, err)
	cond := util.FindNodeStatusCondition(result.Status.Conditions, util.NodeConditionTypeMaintenanceMode)
	require.NotNil(t, cond, "expected error condition to be preserved")
	assert.Equal(t, util.NodeConditionReasonError, cond.Reason)
}

// Test_OnNodeChanged_Running_NoCondition: annotation=running, no condition set
// yet → handler must set the Running condition via UpdateStatus.
func Test_OnNodeChanged_Running_NoCondition(t *testing.T) {
	node := buildNode(util.MaintainStatusRunning, nil)
	h := newHandler(node)

	result, err := h.OnNodeChanged("", node)
	require.NoError(t, err)
	cond := util.FindNodeStatusCondition(result.Status.Conditions, util.NodeConditionTypeMaintenanceMode)
	require.NotNil(t, cond, "expected MaintenanceMode condition to be set")
	assert.Equal(t, util.NodeConditionReasonRunning, cond.Reason)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, util.MaintainStatusRunning, result.Annotations[util.MaintainStatusAnnotation])
}

// Test_OnNodeChanged_Running_ConditionAlreadyRunning: annotation=running,
// Running condition already in sync → UpdateStatus should NOT be called again;
// handler continues to the VMI-wait logic (which returns early because there
// are no VMIs in the fake store).
func Test_OnNodeChanged_Running_ConditionAlreadyRunning(t *testing.T) {
	node := buildNode(util.MaintainStatusRunning, []corev1.NodeCondition{
		maintenanceModeCondition(util.NodeConditionReasonRunning),
	})
	h := newHandler(node)

	// With no VMIs available the handler will mark the node as Complete.
	result, err := h.OnNodeChanged("", node)
	require.NoError(t, err)
	assert.Equal(t, util.MaintainStatusComplete, result.Annotations[util.MaintainStatusAnnotation],
		"expected annotation to be updated to Complete after no VMIs remaining")
}

// Test_OnNodeChanged_Complete_NoCondition: annotation=completed, no condition
// set yet → handler sets Completed condition via UpdateStatus.
func Test_OnNodeChanged_Complete_NoCondition(t *testing.T) {
	node := buildNode(util.MaintainStatusComplete, nil)
	h := newHandler(node)

	result, err := h.OnNodeChanged("", node)
	require.NoError(t, err)
	cond := util.FindNodeStatusCondition(result.Status.Conditions, util.NodeConditionTypeMaintenanceMode)
	require.NotNil(t, cond, "expected MaintenanceMode condition to be set")
	assert.Equal(t, util.NodeConditionReasonCompleted, cond.Reason)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, util.MaintainStatusComplete, result.Annotations[util.MaintainStatusAnnotation])
}

// Test_OnNodeChanged_Complete_WrongCondition: annotation=completed but
// condition has wrong reason → handler must update condition to Completed.
func Test_OnNodeChanged_Complete_WrongCondition(t *testing.T) {
	node := buildNode(util.MaintainStatusComplete, []corev1.NodeCondition{
		maintenanceModeCondition(util.NodeConditionReasonRunning),
	})
	h := newHandler(node)

	result, err := h.OnNodeChanged("", node)
	require.NoError(t, err)
	cond := util.FindNodeStatusCondition(result.Status.Conditions, util.NodeConditionTypeMaintenanceMode)
	require.NotNil(t, cond, "expected MaintenanceMode condition to be present")
	assert.Equal(t, util.NodeConditionReasonCompleted, cond.Reason,
		"expected condition reason to be updated to Completed")
	assert.Equal(t, util.MaintainStatusComplete, result.Annotations[util.MaintainStatusAnnotation])
}

// Test_OnNodeChanged_Complete_ConditionAlreadyCompleted: annotation=completed,
// Completed condition already in sync → handler returns node unchanged (no
// further UpdateStatus call).
func Test_OnNodeChanged_Complete_ConditionAlreadyCompleted(t *testing.T) {
	node := buildNode(util.MaintainStatusComplete, []corev1.NodeCondition{
		maintenanceModeCondition(util.NodeConditionReasonCompleted),
	})
	h := newHandler(node)

	result, err := h.OnNodeChanged("", node)
	require.NoError(t, err)
	// The same node object should be returned (pointer equality), since no
	// update was needed.
	assert.Equal(t, node.Name, result.Name)
	cond := util.FindNodeStatusCondition(result.Status.Conditions, util.NodeConditionTypeMaintenanceMode)
	require.NotNil(t, cond)
	assert.Equal(t, util.NodeConditionReasonCompleted, cond.Reason)
	assert.Equal(t, util.MaintainStatusComplete, result.Annotations[util.MaintainStatusAnnotation])
}

// Test_OnNodeChanged_NilNode: nil node → handler returns immediately without
// error.
func Test_OnNodeChanged_NilNode(t *testing.T) {
	h := &maintainNodeHandler{}
	result, err := h.OnNodeChanged("", nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

// Test_OnNodeChanged_DeletedNode: node with DeletionTimestamp → handler
// returns immediately without error.
func Test_OnNodeChanged_DeletedNode(t *testing.T) {
	node := buildNode("", nil)
	now := metav1.Now()
	node.DeletionTimestamp = &now
	h := &maintainNodeHandler{}

	result, err := h.OnNodeChanged("", node)
	require.NoError(t, err)
	assert.Equal(t, node, result)
}
