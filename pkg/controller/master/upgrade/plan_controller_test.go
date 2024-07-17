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

func newTestPreparePlan() *upgradeapiv1.Plan {
	plan := preparePlan(newTestUpgradeBuilder().Build(), defaultImagePreloadConcurrency)
	plan.Status.LatestHash = testPlanHash
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
			name: "prepare plan is running",
			given: input{
				key:     testPlanName,
				plan:    newTestPreparePlan(),
				upgrade: newTestUpgradeBuilder().NodeUpgradeStatus("node-1", StateSucceeded, "", "").Build(),
				nodes: []*v1.Node{
					newNodeBuilder("node-1").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestPreparePlan().Name), testPlanHash).Build(),
					newNodeBuilder("node-2").Managed().ControlPlane().Build(),
					newNodeBuilder("node-3").Managed().ControlPlane().Build(),
				},
			},
			expected: output{
				plan: newTestPreparePlan(),
				err:  nil,
			},
		},
		{
			name: "set NodesPrepared condition when prepare plan completes",
			given: input{
				key:  testPlanName,
				plan: newTestPreparePlan(),
				upgrade: newTestUpgradeBuilder().
					NodeUpgradeStatus("node-1", nodeStateImagesPreloaded, "", "").
					NodeUpgradeStatus("node-2", nodeStateImagesPreloaded, "", "").
					NodeUpgradeStatus("node-3", nodeStateImagesPreloaded, "", "").
					NodeUpgradeStatus("node-4", nodeStateImagesPreloaded, "", "").
					NodesPreparedCondition(v1.ConditionUnknown, "", "").
					Build(),
				nodes: []*v1.Node{
					newNodeBuilder("node-1").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestPreparePlan().Name), testPlanHash).Build(),
					newNodeBuilder("node-2").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestPreparePlan().Name), testPlanHash).Build(),
					newNodeBuilder("node-3").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestPreparePlan().Name), testPlanHash).Build(),
					newNodeBuilder("node-4").Managed().ControlPlane().WithLabel(upgrade.LabelPlanName(newTestPreparePlan().Name), testPlanHash).Build(),
				},
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().
					NodeUpgradeStatus("node-1", nodeStateImagesPreloaded, "", "").
					NodeUpgradeStatus("node-2", nodeStateImagesPreloaded, "", "").
					NodeUpgradeStatus("node-3", nodeStateImagesPreloaded, "", "").
					NodeUpgradeStatus("node-4", nodeStateImagesPreloaded, "", "").
					NodesPreparedCondition(v1.ConditionTrue, "", "").
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
			emptyConditionsTime(tc.expected.upgrade.Status.Conditions)
			emptyConditionsTime(actual.upgrade.Status.Conditions)
			assert.Nil(t, err)
		}
		if tc.expected.plan != nil {
			actual.plan, err = handler.planClient.Get(sucNamespace, tc.expected.plan.Name, metav1.GetOptions{})
			assert.Nil(t, err)
			expectedPlanStatus := tc.expected.plan.Status
			sanitizeStatus(&expectedPlanStatus)
			sanitizeStatus(&actual.plan.Status)
			tc.expected.plan.Status = expectedPlanStatus
		}

		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

// sanitizeStatus resets hash and timestamps that may fail the comparison
func sanitizeStatus(status *upgradeapiv1.PlanStatus) {
	status.LatestHash = ""
	for i, cond := range status.Conditions {
		cond.LastTransitionTime = ""
		cond.LastUpdateTime = ""
		status.Conditions[i] = cond
	}
}
