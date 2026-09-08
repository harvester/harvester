package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestMaintainNodeCompletesEvacuation(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	util.SetMaintenanceModeCondition(node, corev1.ConditionTrue, util.NodeConditionReasonEvacuating, "evacuating")
	clientset := fake.NewSimpleClientset(node)
	handler := &maintainNodeHandler{
		nodes:                       fakeclients.NodeClient(clientset.CoreV1().Nodes),
		virtualMachineCache:         fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
		virtualMachineInstanceCache: fakeclients.VirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances),
	}

	_, err := handler.OnNodeChanged(node.Name, node)
	require.NoError(t, err)
	updated, err := clientset.CoreV1().Nodes().Get(context.Background(), node.Name, metav1.GetOptions{})
	require.NoError(t, err)
	condition := util.GetMaintenanceModeCondition(updated)
	require.NotNil(t, condition)
	require.Equal(t, util.NodeConditionReasonCompleted, condition.Reason)
}

func TestMaintainNodeRequeuesWhileVMIRemains(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	util.SetMaintenanceModeCondition(node, corev1.ConditionTrue, util.NodeConditionReasonEvacuating, "evacuating")
	vmi := &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "vm-1", Namespace: "default", Labels: map[string]string{kubevirtv1.NodeNameLabel: node.Name}},
		Status:     kubevirtv1.VirtualMachineInstanceStatus{NodeName: node.Name},
	}
	clientset := fake.NewSimpleClientset(node, vmi)
	var requeued bool
	handler := &maintainNodeHandler{
		nodes:                       fakeclients.NodeClient(clientset.CoreV1().Nodes),
		virtualMachineInstanceCache: fakeclients.VirtualMachineInstanceCache(clientset.KubevirtV1().VirtualMachineInstances),
		enqueueAfter: func(key string, delay time.Duration) {
			requeued = key == node.Name && delay == requeueDelay
		},
	}

	_, err := handler.OnNodeChanged(node.Name, node)
	require.NoError(t, err)
	require.True(t, requeued)
}

func TestRestartMaintenanceModeVMsOnNode(t *testing.T) {
	nodeName := "node-1"
	haltedStrategy := kubevirtv1.RunStrategyHalted
	runningStrategy := kubevirtv1.RunStrategyRerunOnFailure

	tests := []struct {
		name                 string
		vm                   *kubevirtv1.VirtualMachine
		targetNode           string
		targetStrategy       string
		wantRunStrategy      kubevirtv1.VirtualMachineRunStrategy
		wantAnnotationExists bool
		wantAnnotationValue  string
	}{
		{
			name: "halted VM with stored strategy restarts and clears annotation",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm-halted-with-strategy",
					Namespace: "default",
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: util.MaintainModeStrategyShutdownAndRestartAfterDisable,
					},
					Annotations: map[string]string{
						util.AnnotationMaintainModeStrategyNodeName: nodeName,
						util.AnnotationRunStrategy:                  string(kubevirtv1.RunStrategyRerunOnFailure),
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &haltedStrategy,
				},
			},
			targetNode:           nodeName,
			targetStrategy:       util.MaintainModeStrategyShutdownAndRestartAfterDisable,
			wantRunStrategy:      kubevirtv1.RunStrategyRerunOnFailure,
			wantAnnotationExists: false,
		},
		{
			name: "already running VM keeps run strategy and clears annotation",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm-already-running",
					Namespace: "default",
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: util.MaintainModeStrategyShutdownAndRestartAfterDisable,
					},
					Annotations: map[string]string{
						util.AnnotationMaintainModeStrategyNodeName: nodeName,
						util.AnnotationRunStrategy:                  string(kubevirtv1.RunStrategyRerunOnFailure),
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runningStrategy,
				},
			},
			targetNode:           nodeName,
			targetStrategy:       util.MaintainModeStrategyShutdownAndRestartAfterDisable,
			wantRunStrategy:      kubevirtv1.RunStrategyRerunOnFailure,
			wantAnnotationExists: false,
		},
		{
			name: "halted VM without stored strategy defaults to RerunOnFailure and clears annotation",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm-without-stored-strategy",
					Namespace: "default",
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: util.MaintainModeStrategyShutdownAndRestartAfterDisable,
					},
					Annotations: map[string]string{
						util.AnnotationMaintainModeStrategyNodeName: nodeName,
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &haltedStrategy,
				},
			},
			targetNode:           nodeName,
			targetStrategy:       util.MaintainModeStrategyShutdownAndRestartAfterDisable,
			wantRunStrategy:      kubevirtv1.RunStrategyRerunOnFailure,
			wantAnnotationExists: false,
		},
		{
			name: "VM on different node is ignored and unchanged",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm-other-node",
					Namespace: "default",
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: util.MaintainModeStrategyShutdownAndRestartAfterDisable,
					},
					Annotations: map[string]string{
						util.AnnotationMaintainModeStrategyNodeName: "node-2",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &haltedStrategy,
				},
			},
			targetNode:           nodeName,
			targetStrategy:       util.MaintainModeStrategyShutdownAndRestartAfterDisable,
			wantRunStrategy:      kubevirtv1.RunStrategyHalted,
			wantAnnotationExists: true,
			wantAnnotationValue:  "node-2",
		},
		{
			name: "VM with different maintenance strategy label is ignored and unchanged",
			vm: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm-other-strategy",
					Namespace: "default",
					Labels: map[string]string{
						util.LabelMaintainModeStrategy: util.MaintainModeStrategyShutdownAndRestartAfterEnable,
					},
					Annotations: map[string]string{
						util.AnnotationMaintainModeStrategyNodeName: nodeName,
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &haltedStrategy,
				},
			},
			targetNode:           nodeName,
			targetStrategy:       util.MaintainModeStrategyShutdownAndRestartAfterDisable,
			wantRunStrategy:      kubevirtv1.RunStrategyHalted,
			wantAnnotationExists: true,
			wantAnnotationValue:  nodeName,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.vm)
			vmClient := fakeclients.VirtualMachineClient(clientset.KubevirtV1().VirtualMachines)
			vmCache := fakeclients.VirtualMachineCache(clientset.KubevirtV1().VirtualMachines)

			err := util.RestartMaintenanceModeVMsOnNode(vmClient, vmCache, tc.targetNode, tc.targetStrategy)
			require.NoError(t, err)

			updated, err := clientset.KubevirtV1().VirtualMachines(tc.vm.Namespace).Get(context.Background(), tc.vm.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, tc.wantRunStrategy, *updated.Spec.RunStrategy)

			if tc.wantAnnotationExists {
				require.Equal(t, tc.wantAnnotationValue, updated.Annotations[util.AnnotationMaintainModeStrategyNodeName])
			} else {
				require.NotContains(t, updated.Annotations, util.AnnotationMaintainModeStrategyNodeName)
			}
		})
	}
}
