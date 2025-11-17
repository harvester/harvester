package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	kubevirtcorev1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestHandleCPUMigrationCapabilitiesRequest(t *testing.T) {
	tests := []struct {
		name             string
		nodes            []*corev1.Node
		kubevirt         *kubevirtcorev1.KubeVirt
		expectedError    bool
		expectedResponse CPUMigrationCapabilities
	}{
		{
			name: "single ready node, no custom kubevirt config",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							kubevirtcorev1.CPUModelLabel + "Intel": "true",
						},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				},
			},
			expectedResponse: CPUMigrationCapabilities{
				TotalNodes: 1,
				GlobalModels: []string{
					kubevirtcorev1.CPUModeHostModel,
					kubevirtcorev1.CPUModeHostPassthrough,
				},
				Models: map[string]CPUModelCapabilities{
					"Intel": {
						ReadyCount:    1,
						MigrationSafe: false,
					},
				},
			},
		},
		{
			name: "two ready nodes, mixed models",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							kubevirtcorev1.CPUModelLabel + "Intel": "true",
						},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							kubevirtcorev1.CPUModelLabel + "AMD": "true",
						},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				},
			},
			expectedResponse: CPUMigrationCapabilities{
				TotalNodes: 2,
				GlobalModels: []string{
					kubevirtcorev1.CPUModeHostModel,
					kubevirtcorev1.CPUModeHostPassthrough,
				},
				Models: map[string]CPUModelCapabilities{
					"Intel": {
						ReadyCount:    1,
						MigrationSafe: false,
					},
					"AMD": {
						ReadyCount:    1,
						MigrationSafe: false,
					},
				},
			},
		},
		{
			name: "two ready nodes with same model",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							kubevirtcorev1.CPUModelLabel + "Intel": "true",
						},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							kubevirtcorev1.CPUModelLabel + "Intel": "true",
						},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				},
			},
			expectedResponse: CPUMigrationCapabilities{
				TotalNodes: 2,
				GlobalModels: []string{
					kubevirtcorev1.CPUModeHostModel,
					kubevirtcorev1.CPUModeHostPassthrough,
				},
				Models: map[string]CPUModelCapabilities{
					"Intel": {
						ReadyCount:    2,
						MigrationSafe: true,
					},
				},
			},
		},
		{
			name: "one ready, one unready node",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							kubevirtcorev1.CPUModelLabel + "Intel": "true",
						},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							kubevirtcorev1.CPUModelLabel + "AMD": "true",
						},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
						},
					},
				},
			},
			expectedResponse: CPUMigrationCapabilities{
				TotalNodes: 2,
				GlobalModels: []string{
					kubevirtcorev1.CPUModeHostModel,
					kubevirtcorev1.CPUModeHostPassthrough,
				},
				Models: map[string]CPUModelCapabilities{
					"Intel": {
						ReadyCount:    1,
						MigrationSafe: false,
					},
				},
			},
		},
		{
			name: "node with no conditions",
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							kubevirtcorev1.CPUModelLabel + "Intel": "true",
						},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{},
					},
				},
			},
			expectedResponse: CPUMigrationCapabilities{
				TotalNodes: 1,
				GlobalModels: []string{
					kubevirtcorev1.CPUModeHostModel,
					kubevirtcorev1.CPUModeHostPassthrough,
				},
				Models: map[string]CPUModelCapabilities{},
			},
		},
		{
			name:  "kubevirt config with cpu model",
			nodes: []*corev1.Node{},
			kubevirt: &kubevirtcorev1.KubeVirt{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.KubeVirtObjectName,
					Namespace: util.HarvesterSystemNamespaceName,
				},
				Spec: kubevirtcorev1.KubeVirtSpec{
					Configuration: kubevirtcorev1.KubeVirtConfiguration{
						CPUModel: "CustomCPU",
					},
				},
			},
			expectedResponse: CPUMigrationCapabilities{
				TotalNodes: 0,
				GlobalModels: []string{
					kubevirtcorev1.CPUModeHostModel,
					kubevirtcorev1.CPUModeHostPassthrough,
					"CustomCPU",
				},
				Models: map[string]CPUModelCapabilities{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sclientset := k8sfake.NewClientset()
			kubevirtclientset := fake.NewSimpleClientset()

			// Add kubevirt object to fake tracker if provided
			if tt.kubevirt != nil {
				err := kubevirtclientset.Tracker().Add(tt.kubevirt)
				assert.NoError(t, err, "Mock resource should add into fake controller tracker")
			}

			// Add nodes to fake tracker
			for _, node := range tt.nodes {
				err := k8sclientset.Tracker().Add(node)
				assert.NoError(t, err, "Mock resource should add into fake controller tracker")
			}

			store := &Store{
				nodeCache:     fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
				kubeVirtCache: fakeclients.KubeVirtCache(kubevirtclientset.KubevirtV1().KubeVirts),
			}

			resultList, err := store.handleCPUMigrationCapabilitiesRequest()
			if tt.expectedError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, 1, resultList.Count)
			assert.Len(t, resultList.Objects, 1)

			obj := resultList.Objects[0]
			assert.Equal(t, objectType, obj.Type)
			assert.Equal(t, objectID, obj.ID)

			capabilities, ok := obj.Object.(CPUMigrationCapabilities)
			assert.True(t, ok)
			assert.Equal(t, tt.expectedResponse, capabilities)
		})
	}
}
