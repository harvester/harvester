package upgrade

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	"github.com/harvester/harvester/pkg/webhook/types"
	"github.com/harvester/harvester/pkg/webhook/util"
)

func TestUpgradeMutator_PatchUpgradeConfig(t *testing.T) {
	type input struct {
		upgrade *harvesterv1.Upgrade
		setting *harvesterv1.Setting
		nodes   []*corev1.Node
	}
	type output struct {
		patchOps types.PatchOps
		err      error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "default value node upgrade auto mode",
			given: input{
				upgrade: &harvesterv1.Upgrade{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-upgrade",
					},
				},
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Default: `{"nodeUpgradeOption": {"strategy": {"mode": "auto"}}}`,
				},
			},
			expected: output{
				patchOps: nil,
			},
		},
		{
			name: "user-provided value node upgrade auto mode",
			given: input{
				upgrade: &harvesterv1.Upgrade{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-upgrade",
					},
				},
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"nodeUpgradeOption": {"strategy": {"mode": "auto"}}}`,
				},
			},
			expected: output{
				patchOps: nil,
			},
		},
		{
			name: "manual node upgrade mode without any pause node specified",
			given: input{
				upgrade: &harvesterv1.Upgrade{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-upgrade",
					},
				},
				nodes: util.NewNodes("node-0", "node-1", "node-2"),
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"nodeUpgradeOption": {"strategy": {"mode": "manual"}}}`,
				},
			},
			expected: output{
				patchOps: types.PatchOps{
					`{"op": "add", "path": "/metadata/annotations", "value": {}}`,
					`{"op": "add", "path": "/metadata/annotations/harvesterhci.io~1node-0", "value": "pause"}`,
					`{"op": "add", "path": "/metadata/annotations/harvesterhci.io~1node-1", "value": "pause"}`,
					`{"op": "add", "path": "/metadata/annotations/harvesterhci.io~1node-2", "value": "pause"}`,
				},
			},
		},
		{
			name: "manual node upgrade mode with pause nodes specified explicitly",
			given: input{
				upgrade: &harvesterv1.Upgrade{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-upgrade",
					},
				},
				nodes: util.NewNodes("node-0", "node-1", "node-2"),
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"nodeUpgradeOption": {"strategy": {"mode": "manual", "pauseNodes": ["node-0", "node-2"]}}}`,
				},
			},
			expected: output{
				patchOps: types.PatchOps{
					`{"op": "add", "path": "/metadata/annotations", "value": {}}`,
					`{"op": "add", "path": "/metadata/annotations/harvesterhci.io~1node-0", "value": "pause"}`,
					`{"op": "add", "path": "/metadata/annotations/harvesterhci.io~1node-2", "value": "pause"}`,
				},
			},
		},
		{
			name: "manual node upgrade mode with all pause nodes specified",
			given: input{
				upgrade: &harvesterv1.Upgrade{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-upgrade",
					},
				},
				nodes: util.NewNodes("node-0", "node-1", "node-2"),
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"nodeUpgradeOption": {"strategy": {"mode": "manual", "pauseNodes": ["node-0", "node-1", "node-2"]}}}`,
				},
			},
			expected: output{
				patchOps: types.PatchOps{
					`{"op": "add", "path": "/metadata/annotations", "value": {}}`,
					`{"op": "add", "path": "/metadata/annotations/harvesterhci.io~1node-0", "value": "pause"}`,
					`{"op": "add", "path": "/metadata/annotations/harvesterhci.io~1node-1", "value": "pause"}`,
					`{"op": "add", "path": "/metadata/annotations/harvesterhci.io~1node-2", "value": "pause"}`,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var objs = []runtime.Object{tc.given.setting}
			clientset := fake.NewSimpleClientset(objs...)
			var nodes []runtime.Object
			for _, node := range tc.given.nodes {
				nodes = append(nodes, node)
			}
			k8sclientset := k8sfake.NewSimpleClientset(nodes...)
			mutator := NewMutator(fakeclients.NodeCache(k8sclientset.CoreV1().Nodes), fakeclients.HarvesterSettingCache(clientset.HarvesterhciV1beta1().Settings))

			patchOps, err := mutator.(*upgradeMutator).patchPauseNodeAnnotations(tc.given.upgrade, nil)
			assert.Nil(t, tc.expected.err, err, tc.name)

			assert.Equal(t, tc.expected.patchOps, patchOps, tc.name)
		})
	}
}
