package upgrade

import (
	"testing"

	"github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io"
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

func newTestServerPlan() *upgradeapiv1.Plan {
	plan := serverPlan(newTestUpgradeBuilder().Build(), false)
	plan.Status.LatestHash = testPlanHash
	return plan
}

func newTestAgentPlan() *upgradeapiv1.Plan {
	plan := agentPlan(newTestUpgradeBuilder().Build())
	plan.Status.LatestHash = testAgentPlanHash
	return plan
}

func TestPlanHandler_OnChanged(t *testing.T) {
	type input struct {
		key     string
		plan    *upgradeapiv1.Plan
		upgrade *harvesterv1.Upgrade
		nodes   []*v1.Node
	}
	type output struct {
		serverPlan *upgradeapiv1.Plan
		agentPlan  *upgradeapiv1.Plan
		upgrade    *harvesterv1.Upgrade
		err        error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "server plan is running",
			given: input{
				key:     testPlanName,
				plan:    newTestServerPlan(),
				upgrade: newTestUpgradeBuilder().NodeUpgradeStatus("node-1", stateSucceeded, "", "").Build(),
				nodes: []*v1.Node{
					newNodeBuilder("node-1").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestServerPlan().Name), testPlanHash).Build(),
					newNodeBuilder("node-2").Managed().ControlPlane().Build(),
					newNodeBuilder("node-3").Managed().ControlPlane().Build(),
				},
			},
			expected: output{
				serverPlan: newTestServerPlan(),
				err:        nil,
			},
		},
		{
			name: "agent plan is created when server plan completes",
			given: input{
				key:  testPlanName,
				plan: newTestServerPlan(),
				upgrade: newTestUpgradeBuilder().
					NodeUpgradeStatus("node-1", stateSucceeded, "", "").
					NodeUpgradeStatus("node-2", stateSucceeded, "", "").
					NodeUpgradeStatus("node-3", stateSucceeded, "", "").
					Build(),
				nodes: []*v1.Node{
					newNodeBuilder("node-1").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestServerPlan().Name), testPlanHash).Build(),
					newNodeBuilder("node-2").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestServerPlan().Name), testPlanHash).Build(),
					newNodeBuilder("node-3").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestServerPlan().Name), testPlanHash).Build(),
				},
			},
			expected: output{
				serverPlan: newTestServerPlan(),
				agentPlan:  newTestAgentPlan(),
				err:        nil,
			},
		},
		{
			name: "agent plan is running",
			given: input{
				key:  testPlanName,
				plan: newTestAgentPlan(),
				upgrade: newTestUpgradeBuilder().
					NodeUpgradeStatus("node-1", stateSucceeded, "", "").
					NodeUpgradeStatus("node-2", stateSucceeded, "", "").
					NodeUpgradeStatus("node-3", stateSucceeded, "", "").
					NodeUpgradeStatus("node-4", stateUpgrading, "", "").
					NodesUpgradedCondition(v1.ConditionUnknown, "", "").
					Build(),
				nodes: []*v1.Node{
					newNodeBuilder("node-1").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestServerPlan().Name), testPlanHash).Build(),
					newNodeBuilder("node-2").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestServerPlan().Name), testPlanHash).Build(),
					newNodeBuilder("node-3").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestServerPlan().Name), testPlanHash).Build(),
					newNodeBuilder("node-4").Managed().Build(),
				},
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().
					NodeUpgradeStatus("node-1", stateSucceeded, "", "").
					NodeUpgradeStatus("node-2", stateSucceeded, "", "").
					NodeUpgradeStatus("node-3", stateSucceeded, "", "").
					NodeUpgradeStatus("node-4", stateUpgrading, "", "").
					NodesUpgradedCondition(v1.ConditionUnknown, "", "").
					Build(),
				err: nil,
			},
		},
		{
			name: "set nodesUpgraded condition when agent plan completes",
			given: input{
				key:  testPlanName,
				plan: newTestAgentPlan(),
				upgrade: newTestUpgradeBuilder().
					NodeUpgradeStatus("node-1", stateSucceeded, "", "").
					NodeUpgradeStatus("node-2", stateSucceeded, "", "").
					NodeUpgradeStatus("node-3", stateSucceeded, "", "").
					NodeUpgradeStatus("node-4", stateSucceeded, "", "").
					NodesUpgradedCondition(v1.ConditionUnknown, "", "").
					Build(),
				nodes: []*v1.Node{
					newNodeBuilder("node-1").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestServerPlan().Name), testPlanHash).Build(),
					newNodeBuilder("node-2").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestServerPlan().Name), testPlanHash).Build(),
					newNodeBuilder("node-3").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestServerPlan().Name), testPlanHash).Build(),
					newNodeBuilder("node-4").Managed().WithLabel(upgrade.LabelPlanName(newTestAgentPlan().Name), testAgentPlanHash).Build(),
				},
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().
					NodeUpgradeStatus("node-1", stateSucceeded, "", "").
					NodeUpgradeStatus("node-2", stateSucceeded, "", "").
					NodeUpgradeStatus("node-3", stateSucceeded, "", "").
					NodeUpgradeStatus("node-4", stateSucceeded, "", "").
					NodesUpgradedCondition(v1.ConditionTrue, "", "").
					Build(),
				err: nil,
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.plan, tc.given.upgrade)
		var nodes []runtime.Object
		for _, node := range tc.given.nodes {
			nodes = append(nodes, node)
		}
		var k8sclientset = k8sfake.NewSimpleClientset(nodes...)
		var handler = &planHandler{
			namespace:     harvesterSystemNamespace,
			upgradeClient: fakeclients.UpgradeClient(clientset.HarvesterhciV1beta1().Upgrades),
			upgradeCache:  fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades),
			nodeCache:     fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
			planClient:    fakeclients.PlanClient(clientset.UpgradeV1().Plans),
		}
		var actual output
		var err error
		_, actual.err = handler.OnChanged(tc.given.key, tc.given.plan)
		if tc.expected.upgrade != nil {
			actual.upgrade, err = handler.upgradeCache.Get(handler.namespace, tc.given.upgrade.Name)
			assert.Nil(t, err)
		}
		if tc.expected.serverPlan != nil {
			actual.serverPlan, err = handler.planClient.Get(k3osSystemNamespace, tc.expected.serverPlan.Name, metav1.GetOptions{})
			assert.Nil(t, err)
			//skip hash comparison
			tc.expected.serverPlan.Status.LatestHash = ""
			actual.serverPlan.Status.LatestHash = ""
		}

		if tc.expected.agentPlan != nil {
			actual.agentPlan, err = handler.planClient.Get(k3osSystemNamespace, tc.expected.agentPlan.Name, metav1.GetOptions{})
			assert.Nil(t, err)
			//skip hash comparison
			tc.expected.agentPlan.Status.LatestHash = ""
			actual.agentPlan.Status.LatestHash = ""
		}
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}
