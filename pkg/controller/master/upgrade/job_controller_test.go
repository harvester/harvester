package upgrade

import (
	"testing"

	upgradeapiv1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestJobHandler_OnChanged(t *testing.T) {
	type input struct {
		key     string
		job     *batchv1.Job
		plan    *upgradeapiv1.Plan
		upgrade *harvesterv1.Upgrade
	}
	type output struct {
		job     *batchv1.Job
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
			name: "preparing a node",
			given: input{
				key:     testJobName,
				job:     newTestNodeJobBuilder().Running().Build(),
				plan:    newTestPlanBuilder().Build(),
				upgrade: newTestUpgradeBuilder().WithLabel(upgradeStateLabel, StatePreparingNodes).Build(),
			},
			expected: output{
				job:     newTestNodeJobBuilder().Running().Build(),
				upgrade: newTestUpgradeBuilder().WithLabel(upgradeStateLabel, StatePreparingNodes).NodeUpgradeStatus(testNodeName, nodeStateImagesPreloading, "", "").Build(),
				err:     nil,
			},
		},
		{
			name: "node preparing job failed",
			given: input{
				key:     testJobName,
				job:     newTestNodeJobBuilder().Failed("FailedReason", "failed message").Build(),
				plan:    newTestPlanBuilder().Build(),
				upgrade: newTestUpgradeBuilder().WithLabel(upgradeStateLabel, StatePreparingNodes).Build(),
			},
			expected: output{
				job:     newTestNodeJobBuilder().Failed("FailedReason", "failed message").Build(),
				upgrade: newTestUpgradeBuilder().NodeUpgradeStatus("test-node", StateFailed, "FailedReason", "failed message").Build(),
				err:     nil,
			},
		},
		{
			name: "node preparing job succeeded",
			given: input{
				key:     testJobName,
				job:     newTestNodeJobBuilder().Completed().Build(),
				plan:    newTestPlanBuilder().Build(),
				upgrade: newTestUpgradeBuilder().WithLabel(upgradeStateLabel, StatePreparingNodes).Build(),
			},
			expected: output{
				job:     newTestNodeJobBuilder().Completed().Build(),
				upgrade: newTestUpgradeBuilder().WithLabel(upgradeStateLabel, StatePreparingNodes).NodeUpgradeStatus("test-node", nodeStateImagesPreloaded, "", "").Build(),
				err:     nil,
			},
		},
		{
			name: "upgrading manifest job is running",
			given: input{
				key:     testJobName,
				job:     newTestChartJobBuilder().Running().Build(),
				plan:    newTestPlanBuilder().Build(),
				upgrade: newTestUpgradeBuilder().ChartUpgradeStatus(v1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				job:     newTestChartJobBuilder().Running().Build(),
				upgrade: newTestUpgradeBuilder().ChartUpgradeStatus(v1.ConditionUnknown, "", "").Build(),
				err:     nil,
			},
		},
		{
			name: "upgrading manifest job failed",
			given: input{
				key:     testJobName,
				job:     newTestChartJobBuilder().Failed("FailedReason", "failed message").Build(),
				plan:    newTestPlanBuilder().Build(),
				upgrade: newTestUpgradeBuilder().ChartUpgradeStatus(v1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				job:     newTestChartJobBuilder().Failed("FailedReason", "failed message").Build(),
				upgrade: newTestUpgradeBuilder().ChartUpgradeStatus(v1.ConditionFalse, "FailedReason", "failed message").Build(),
				err:     nil,
			},
		},
		{
			name: "upgrading manifest job succeeded",
			given: input{
				key:     testJobName,
				job:     newTestChartJobBuilder().Completed().Build(),
				plan:    newTestPlanBuilder().Build(),
				upgrade: newTestUpgradeBuilder().ChartUpgradeStatus(v1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				job:     newTestChartJobBuilder().Completed().Build(),
				upgrade: newTestUpgradeBuilder().ChartUpgradeStatus(v1.ConditionTrue, "", "").Build(),
				err:     nil,
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.plan, tc.given.upgrade)
		var handler = &jobHandler{
			namespace:     harvesterSystemNamespace,
			planCache:     fakeclients.PlanCache(clientset.UpgradeV1().Plans),
			upgradeClient: fakeclients.UpgradeClient(clientset.HarvesterhciV1beta1().Upgrades),
			upgradeCache:  fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades),
		}
		var actual output
		actual.job, actual.err = handler.OnChanged(tc.given.key, tc.given.job)
		if tc.expected.upgrade != nil {
			var err error
			actual.upgrade, err = handler.upgradeCache.Get(handler.namespace, tc.given.upgrade.Name)
			assert.Nil(t, err)
		}
		if tc.expected.plan != nil {
			var err error
			actual.plan, err = handler.planCache.Get(upgradeNamespace, tc.given.upgrade.Name)
			assert.Nil(t, err)
		}

		emptyConditionsTime(tc.expected.upgrade.Status.Conditions)
		emptyConditionsTime(actual.upgrade.Status.Conditions)

		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}
