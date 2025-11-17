package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtcorev1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/yaml"

	kubevirtfake "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	corefake "k8s.io/client-go/kubernetes/fake"
)

func Test_configmap_reconcile(t *testing.T) {
	type input struct {
		nodes          []*corev1.Node
		kubevirt       *kubevirtcorev1.KubeVirt
		existingConfig *corev1.ConfigMap
	}
	type output struct {
		data CPUModelData
		err  error
	}

	testCases := []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "empty nodes, empty models",
			given: input{
				nodes: []*corev1.Node{},
				existingConfig: &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Data: map[string]string{
						configMapDataKey: "totalNodes: 0\nglobalModels: []\nmodels: {}",
					},
				},
			},
			expected: output{
				data: CPUModelData{
					TotalNodes: 0,
					GlobalModels: []string{
						kubevirtcorev1.CPUModeHostModel,
						kubevirtcorev1.CPUModeHostPassthrough,
					},
					Models: map[string]CPUModelCapabilities{},
				},
				err: nil,
			},
		},
		{
			name: "single ready node with CPU models",
			given: input{
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
				existingConfig: &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Data: map[string]string{
						configMapDataKey: "totalNodes: 0\nglobalModels: []\nmodels: {}",
					},
				},
			},
			expected: output{
				data: CPUModelData{
					TotalNodes: 1,
					GlobalModels: []string{
						kubevirtcorev1.CPUModeHostModel,
						kubevirtcorev1.CPUModeHostPassthrough,
					},
					Models: map[string]CPUModelCapabilities{
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
				err: nil,
			},
		},
		{
			name: "multiple ready nodes with same CPU models - migration safe",
			given: input{
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
				existingConfig: &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Data: map[string]string{
						configMapDataKey: "totalNodes: 0\nglobalModels: []\nmodels: {}",
					},
				},
			},
			expected: output{
				data: CPUModelData{
					TotalNodes: 2,
					GlobalModels: []string{
						kubevirtcorev1.CPUModeHostModel,
						kubevirtcorev1.CPUModeHostPassthrough,
					},
					Models: map[string]CPUModelCapabilities{
						"Skylake-Client-IBRS": {
							ReadyCount:    2,
							MigrationSafe: true,
						},
					},
				},
				err: nil,
			},
		},
		{
			name: "mixed ready and not-ready nodes",
			given: input{
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
				existingConfig: &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Data: map[string]string{
						configMapDataKey: "totalNodes: 0\nglobalModels: []\nmodels: {}",
					},
				},
			},
			expected: output{
				data: CPUModelData{
					TotalNodes: 2,
					GlobalModels: []string{
						kubevirtcorev1.CPUModeHostModel,
						kubevirtcorev1.CPUModeHostPassthrough,
					},
					Models: map[string]CPUModelCapabilities{
						"Skylake-Client-IBRS": {
							ReadyCount:    1,
							MigrationSafe: false,
						},
					},
				},
				err: nil,
			},
		},
		{
			name: "nodes with different CPU models",
			given: input{
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
				existingConfig: &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Data: map[string]string{
						configMapDataKey: "totalNodes: 0\nglobalModels: []\nmodels: {}",
					},
				},
			},
			expected: output{
				data: CPUModelData{
					TotalNodes: 2,
					GlobalModels: []string{
						kubevirtcorev1.CPUModeHostModel,
						kubevirtcorev1.CPUModeHostPassthrough,
					},
					Models: map[string]CPUModelCapabilities{
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
				err: nil,
			},
		},
		{
			name: "kubevirt with custom CPU model",
			given: input{
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
				existingConfig: &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Data: map[string]string{
						configMapDataKey: "totalNodes: 0\nglobalModels: []\nmodels: {}",
					},
				},
			},
			expected: output{
				data: CPUModelData{
					TotalNodes: 1,
					GlobalModels: []string{
						kubevirtcorev1.CPUModeHostModel,
						kubevirtcorev1.CPUModeHostPassthrough,
						"Haswell-noTSX-IBRS",
					},
					Models: map[string]CPUModelCapabilities{
						"Skylake-Client-IBRS": {
							ReadyCount:    1,
							MigrationSafe: false,
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
			coreClientset := corefake.NewSimpleClientset()
			kubevirtClientset := kubevirtfake.NewSimpleClientset()

			// Add test resources to tracker
			if tc.given.existingConfig != nil {
				err := coreClientset.Tracker().Add(tc.given.existingConfig)
				assert.Nil(t, err, "mock configmap should add into fake controller tracker")
			}

			if tc.given.kubevirt != nil {
				err := kubevirtClientset.Tracker().Add(tc.given.kubevirt)
				assert.Nil(t, err, "mock kubevirt should add into fake controller tracker")
			}

			for _, node := range tc.given.nodes {
				err := coreClientset.Tracker().Add(node)
				assert.Nil(t, err, "mock node should add into fake controller tracker")
			}

			// Create handler with fake clients
			handler := &cpuModelConfigHandler{
				nodeCache:       fakeclients.NodeCache(coreClientset.CoreV1().Nodes),
				kubevirtCache:   fakeclients.KubeVirtCache(kubevirtClientset.KubevirtV1().KubeVirts),
				configMapClient: fakeclients.ConfigmapClient(coreClientset.CoreV1().ConfigMaps),
				configMapCache:  fakeclients.ConfigmapCache(coreClientset.CoreV1().ConfigMaps),
			}

			// Execute reconcile
			err := handler.reconcile()

			// Assert
			if tc.expected.err != nil {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Get the updated configmap
				updatedConfigMap, err := coreClientset.CoreV1().ConfigMaps(util.HarvesterSystemNamespaceName).Get(nil, configMapName, metav1.GetOptions{})
				assert.NoError(t, err)
				assert.NotNil(t, updatedConfigMap)

				// Print the ConfigMap data in YAML format
				t.Logf("ConfigMap data:\n%s", updatedConfigMap.Data[configMapDataKey])

				// Parse the YAML data
				var actualData CPUModelData
				err = yaml.Unmarshal([]byte(updatedConfigMap.Data[configMapDataKey]), &actualData)
				assert.NoError(t, err)

				// Compare results
				assert.Equal(t, tc.expected.data.TotalNodes, actualData.TotalNodes)
				assert.ElementsMatch(t, tc.expected.data.GlobalModels, actualData.GlobalModels)
				assert.Equal(t, len(tc.expected.data.Models), len(actualData.Models))
				for model, expectedCaps := range tc.expected.data.Models {
					actualCaps, ok := actualData.Models[model]
					assert.True(t, ok, "model %s should exist", model)
					assert.Equal(t, expectedCaps, actualCaps, "model %s capabilities mismatch", model)
				}
			}
		})
	}
}
