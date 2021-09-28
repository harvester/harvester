package upgrade

import (
	"testing"

	upgradeapiv1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestUpgradeHandler_OnChanged(t *testing.T) {
	type input struct {
		key     string
		upgrade *harvesterv1.Upgrade
		nodes   []*v1.Node
	}
	type output struct {
		plan    *upgradeapiv1.Plan
		upgrade *harvesterv1.Upgrade
		err     error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "upgrade triggers plan creation",
			given: input{
				key:     testUpgradeName,
				upgrade: newTestUpgradeBuilder().Build(),
				nodes: []*v1.Node{
					newNodeBuilder("node-1").Managed().ControlPlane().Build(),
					newNodeBuilder("node-2").Managed().ControlPlane().Build(),
					newNodeBuilder("node-3").Managed().ControlPlane().Build(),
				},
			},
			expected: output{
				plan:    newTestServerPlan(),
				upgrade: newTestUpgradeBuilder().InitStatus().Build(),
				err:     nil,
			},
		},
		{
			name: "start upgrading the chart when nodes are upgraded",
			given: input{
				key:     testUpgradeName,
				upgrade: newTestUpgradeBuilder().Build(),
				nodes: []*v1.Node{
					newNodeBuilder("node-1").Managed().ControlPlane().Build(),
					newNodeBuilder("node-2").Managed().ControlPlane().Build(),
					newNodeBuilder("node-3").Managed().ControlPlane().Build(),
				},
			},
			expected: output{
				plan:    newTestServerPlan(),
				upgrade: newTestUpgradeBuilder().InitStatus().Build(),
				err:     nil,
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.upgrade)
		var nodes []runtime.Object
		for _, node := range tc.given.nodes {
			nodes = append(nodes, node)
		}
		var k8sclientset = k8sfake.NewSimpleClientset(nodes...)
		var handler = &upgradeHandler{
			namespace:     harvesterSystemNamespace,
			nodeCache:     fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
			planClient:    fakeclients.PlanClient(clientset.UpgradeV1().Plans),
			upgradeClient: fakeclients.UpgradeClient(clientset.HarvesterhciV1beta1().Upgrades),
			upgradeCache:  fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades),
		}
		var actual output
		var err error
		actual.upgrade, actual.err = handler.OnChanged(tc.given.key, tc.given.upgrade)
		if tc.expected.plan != nil {
			actual.plan, err = handler.planClient.Get(upgradeNamespace, tc.expected.plan.Name, metav1.GetOptions{})
			assert.Nil(t, err)
			//skip hash comparison
			actual.plan.Status.LatestHash = ""
			tc.expected.plan.Status.LatestHash = ""
		}

		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}
