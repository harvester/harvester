package virtualmachinecpumodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtcorev1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	corefake "k8s.io/client-go/kubernetes/fake"
)

func TestHandler_OnChanged(t *testing.T) {
	type input struct {
		key      string
		obj      *harvesterv1.VirtualMachineCPUModel
		nodes    []*corev1.Node
		kubevirt *kubevirtcorev1.KubeVirt
	}
	type output struct {
		obj *harvesterv1.VirtualMachineCPUModel
		err error
	}

	testCases := []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "nil resource",
			given: input{
				key:   "",
				obj:   nil,
				nodes: []*corev1.Node{},
			},
			expected: output{
				obj: nil,
				err: nil,
			},
		},
		{
			name: "deleted resource",
			given: input{
				key: singletonName,
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name:              singletonName,
						DeletionTimestamp: &metav1.Time{},
					},
				},
				nodes: []*corev1.Node{},
			},
			expected: output{
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name:              singletonName,
						DeletionTimestamp: &metav1.Time{},
					},
				},
				err: nil,
			},
		},
		{
			name: "empty nodes, empty models",
			given: input{
				key: singletonName,
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name: singletonName,
					},
					Status: harvesterv1.VirtualMachineCPUModelStatus{
						TotalNodes:   0,
						GlobalModels: []string{},
						Models:       map[string]harvesterv1.CPUModelCapabilities{},
					},
				},
				nodes: []*corev1.Node{},
			},
			expected: output{
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name: singletonName,
					},
					Status: harvesterv1.VirtualMachineCPUModelStatus{
						TotalNodes: 0,
						GlobalModels: []string{
							kubevirtcorev1.CPUModeHostModel,
							kubevirtcorev1.CPUModeHostPassthrough,
						},
						Models: map[string]harvesterv1.CPUModelCapabilities{},
					},
				},
				err: nil,
			},
		},
		{
			name: "single ready node with CPU models",
			given: input{
				key: singletonName,
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name: singletonName,
					},
					Status: harvesterv1.VirtualMachineCPUModelStatus{
						TotalNodes:   0,
						GlobalModels: []string{},
						Models:       map[string]harvesterv1.CPUModelCapabilities{},
					},
				},
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node1",
							Labels: map[string]string{
								kubevirtcorev1.CPUModelLabel + "Skylake-Client-IBRS": "true",
								kubevirtcorev1.CPUModelLabel + "Broadwell-IBRS":      "true",
							},
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			expected: output{
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name: singletonName,
					},
					Status: harvesterv1.VirtualMachineCPUModelStatus{
						TotalNodes: 1,
						GlobalModels: []string{
							kubevirtcorev1.CPUModeHostModel,
							kubevirtcorev1.CPUModeHostPassthrough,
						},
						Models: map[string]harvesterv1.CPUModelCapabilities{
							"Skylake-Client-IBRS": {
								ReadyCount:    1,
								MigrationSafe: false,
							},
							"Broadwell-IBRS": {
								ReadyCount:    1,
								MigrationSafe: false,
							},
						},
					},
				},
				err: nil,
			},
		},
		{
			name: "multiple ready nodes with same CPU models - migration safe",
			given: input{
				key: singletonName,
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name: singletonName,
					},
					Status: harvesterv1.VirtualMachineCPUModelStatus{
						TotalNodes:   0,
						GlobalModels: []string{},
						Models:       map[string]harvesterv1.CPUModelCapabilities{},
					},
				},
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node1",
							Labels: map[string]string{
								kubevirtcorev1.CPUModelLabel + "Skylake-Client-IBRS": "true",
							},
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node2",
							Labels: map[string]string{
								kubevirtcorev1.CPUModelLabel + "Skylake-Client-IBRS": "true",
							},
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			expected: output{
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name: singletonName,
					},
					Status: harvesterv1.VirtualMachineCPUModelStatus{
						TotalNodes: 2,
						GlobalModels: []string{
							kubevirtcorev1.CPUModeHostModel,
							kubevirtcorev1.CPUModeHostPassthrough,
						},
						Models: map[string]harvesterv1.CPUModelCapabilities{
							"Skylake-Client-IBRS": {
								ReadyCount:    2,
								MigrationSafe: true,
							},
						},
					},
				},
				err: nil,
			},
		},
		{
			name: "mixed ready and not-ready nodes",
			given: input{
				key: singletonName,
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name: singletonName,
					},
					Status: harvesterv1.VirtualMachineCPUModelStatus{
						TotalNodes:   0,
						GlobalModels: []string{},
						Models:       map[string]harvesterv1.CPUModelCapabilities{},
					},
				},
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node1",
							Labels: map[string]string{
								kubevirtcorev1.CPUModelLabel + "Skylake-Client-IBRS": "true",
							},
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node2",
							Labels: map[string]string{
								kubevirtcorev1.CPUModelLabel + "Skylake-Client-IBRS": "true",
							},
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionFalse,
								},
							},
						},
					},
				},
			},
			expected: output{
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name: singletonName,
					},
					Status: harvesterv1.VirtualMachineCPUModelStatus{
						TotalNodes: 2,
						GlobalModels: []string{
							kubevirtcorev1.CPUModeHostModel,
							kubevirtcorev1.CPUModeHostPassthrough,
						},
						Models: map[string]harvesterv1.CPUModelCapabilities{
							"Skylake-Client-IBRS": {
								ReadyCount:    1,
								MigrationSafe: false,
							},
						},
					},
				},
				err: nil,
			},
		},
		{
			name: "nodes with different CPU models",
			given: input{
				key: singletonName,
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name: singletonName,
					},
					Status: harvesterv1.VirtualMachineCPUModelStatus{
						TotalNodes:   0,
						GlobalModels: []string{},
						Models:       map[string]harvesterv1.CPUModelCapabilities{},
					},
				},
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node1",
							Labels: map[string]string{
								kubevirtcorev1.CPUModelLabel + "Skylake-Client-IBRS": "true",
								kubevirtcorev1.CPUModelLabel + "Broadwell-IBRS":      "true",
							},
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node2",
							Labels: map[string]string{
								kubevirtcorev1.CPUModelLabel + "Cascadelake-Server": "true",
								kubevirtcorev1.CPUModelLabel + "Broadwell-IBRS":     "true",
							},
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			expected: output{
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name: singletonName,
					},
					Status: harvesterv1.VirtualMachineCPUModelStatus{
						TotalNodes: 2,
						GlobalModels: []string{
							kubevirtcorev1.CPUModeHostModel,
							kubevirtcorev1.CPUModeHostPassthrough,
						},
						Models: map[string]harvesterv1.CPUModelCapabilities{
							"Skylake-Client-IBRS": {
								ReadyCount:    1,
								MigrationSafe: false,
							},
							"Broadwell-IBRS": {
								ReadyCount:    2,
								MigrationSafe: true,
							},
							"Cascadelake-Server": {
								ReadyCount:    1,
								MigrationSafe: false,
							},
						},
					},
				},
				err: nil,
			},
		},
		{
			name: "kubevirt with custom CPU model",
			given: input{
				key: singletonName,
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name: singletonName,
					},
					Status: harvesterv1.VirtualMachineCPUModelStatus{
						TotalNodes:   0,
						GlobalModels: []string{},
						Models:       map[string]harvesterv1.CPUModelCapabilities{},
					},
				},
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node1",
							Labels: map[string]string{
								kubevirtcorev1.CPUModelLabel + "Skylake-Client-IBRS": "true",
							},
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				},
				kubevirt: &kubevirtcorev1.KubeVirt{
					ObjectMeta: metav1.ObjectMeta{
						Name:      util.KubeVirtObjectName,
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Spec: kubevirtcorev1.KubeVirtSpec{
						Configuration: kubevirtcorev1.KubeVirtConfiguration{
							CPUModel: "Haswell-noTSX-IBRS",
						},
					},
				},
			},
			expected: output{
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name: singletonName,
					},
					Status: harvesterv1.VirtualMachineCPUModelStatus{
						TotalNodes: 1,
						GlobalModels: []string{
							kubevirtcorev1.CPUModeHostModel,
							kubevirtcorev1.CPUModeHostPassthrough,
							"Haswell-noTSX-IBRS",
						},
						Models: map[string]harvesterv1.CPUModelCapabilities{
							"Skylake-Client-IBRS": {
								ReadyCount:    1,
								MigrationSafe: false,
							},
						},
					},
				},
				err: nil,
			},
		},
		{
			name: "ignore labels with wrong prefix or value",
			given: input{
				key: singletonName,
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name: singletonName,
					},
					Status: harvesterv1.VirtualMachineCPUModelStatus{
						TotalNodes:   0,
						GlobalModels: []string{},
						Models:       map[string]harvesterv1.CPUModelCapabilities{},
					},
				},
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node1",
							Labels: map[string]string{
								kubevirtcorev1.CPUModelLabel + "Skylake-Client-IBRS": "true",
								kubevirtcorev1.CPUModelLabel + "Invalid":             "false", // should be ignored
								"random.label/cpu-model-something":                   "true",  // should be ignored
								kubevirtcorev1.CPUModelLabel:                         "true",  // empty model name, should be ignored
							},
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				},
			},
			expected: output{
				obj: &harvesterv1.VirtualMachineCPUModel{
					ObjectMeta: metav1.ObjectMeta{
						Name: singletonName,
					},
					Status: harvesterv1.VirtualMachineCPUModelStatus{
						TotalNodes: 1,
						GlobalModels: []string{
							kubevirtcorev1.CPUModeHostModel,
							kubevirtcorev1.CPUModeHostPassthrough,
						},
						Models: map[string]harvesterv1.CPUModelCapabilities{
							"Skylake-Client-IBRS": {
								ReadyCount:    1,
								MigrationSafe: false,
							},
						},
					},
				},
				err: nil,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create fake clientsets
			harvesterClientset := fake.NewSimpleClientset()
			coreClientset := corefake.NewSimpleClientset()

			// Add test resources to tracker
			if tc.given.obj != nil {
				err := harvesterClientset.Tracker().Add(tc.given.obj)

				assert.Nil(t, err, "mock resource should add into fake controller tracker")
			}

			if tc.given.kubevirt != nil {
				err := harvesterClientset.Tracker().Add(tc.given.kubevirt)
				assert.Nil(t, err, "mock resource should add into fake controller tracker")
			}

			for _, node := range tc.given.nodes {
				err := coreClientset.Tracker().Add(node)
				assert.Nil(t, err, "mock node should add into fake controller tracker")
			}

			// Create handler with fake clients
			handler := &Handler{
				nodeCache:        fakeclients.NodeCache(coreClientset.CoreV1().Nodes),
				kubevirtCache:    fakeclients.KubeVirtCache(harvesterClientset.KubevirtV1().KubeVirts),
				vmCpuModelClient: fakeclients.VirtualMachineCPUModelClient(harvesterClientset.HarvesterhciV1beta1().VirtualMachineCPUModels),
				vmCpuModelCache:  fakeclients.VirtualMachineCPUModelCache(harvesterClientset.HarvesterhciV1beta1().VirtualMachineCPUModels),
			}

			// Execute handler
			actual, err := handler.OnChanged(tc.given.key, tc.given.obj)

			// Assert
			if tc.expected.err != nil {
				assert.Error(t, err)
				if errors.IsNotFound(tc.expected.err) {
					assert.True(t, errors.IsNotFound(err), "expected NotFound error")
				}
			} else {
				assert.NoError(t, err)
				if tc.expected.obj != nil {
					assert.NotNil(t, actual)
					assert.Equal(t, tc.expected.obj.Status.TotalNodes, actual.Status.TotalNodes)
					assert.ElementsMatch(t, tc.expected.obj.Status.GlobalModels, actual.Status.GlobalModels)
					assert.Equal(t, len(tc.expected.obj.Status.Models), len(actual.Status.Models))
					for model, expectedCaps := range tc.expected.obj.Status.Models {
						actualCaps, ok := actual.Status.Models[model]
						assert.True(t, ok, "model %s should exist", model)
						assert.Equal(t, expectedCaps, actualCaps, "model %s capabilities mismatch", model)
					}
				}
			}
		})
	}
}
