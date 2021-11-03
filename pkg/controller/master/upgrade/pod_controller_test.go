package upgrade

import (
	"testing"

	upgradeapiv1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestPodHandler_OnChanged(t *testing.T) {
	type input struct {
		key     string
		pod     *corev1.Pod
		plan    *upgradeapiv1.Plan
		upgrade *harvesterv1.Upgrade
	}
	type output struct {
		upgrade *harvesterv1.Upgrade
		err     error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "upgrade repo vm ready",
			given: input{
				key: "upgrade-repo-vm-pod",
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "upgrade-repo-vm-pod",
						Namespace: upgradeNamespace,
						Labels: map[string]string{
							harvesterUpgradeLabel:          testUpgradeName,
							harvesterUpgradeComponentLabel: upgradeComponentRepo,
						},
					},
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Ready: true,
							},
						},
					},
				},
				plan:    newTestPlanBuilder().Build(),
				upgrade: newTestUpgradeBuilder().WithLabel(upgradeStateLabel, StatePreparingRepo).Build(),
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().WithLabel(upgradeStateLabel, StateRepoPrepared).RepoProvisionedCondition(corev1.ConditionTrue, "", "").Build(),
				err:     nil,
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.plan, tc.given.upgrade)
		var handler = &podHandler{
			namespace:     harvesterSystemNamespace,
			planCache:     fakeclients.PlanCache(clientset.UpgradeV1().Plans),
			upgradeClient: fakeclients.UpgradeClient(clientset.HarvesterhciV1beta1().Upgrades),
			upgradeCache:  fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades),
		}
		var actual output
		var getErr error
		_, actual.err = handler.OnChanged(tc.given.key, tc.given.pod)
		actual.upgrade, getErr = handler.upgradeCache.Get(handler.namespace, tc.given.upgrade.Name)
		assert.Nil(t, getErr)

		emptyConditionsTime(tc.expected.upgrade.Status.Conditions)
		emptyConditionsTime(actual.upgrade.Status.Conditions)

		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}
