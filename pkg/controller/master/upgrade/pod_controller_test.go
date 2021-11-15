package upgrade

import (
	"testing"

	upgradeapiv1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
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
			name: "upgrade node pod running",
			given: input{
				key: "upgrade-node-pod",
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "upgrade-node-pod",
						Namespace: upgradeNamespace,
						Labels: map[string]string{
							upgradePlanLabel: testPlanName,
							upgradeNodeLabel: testNodeName,
						},
					},
				},
				plan:    newTestPlanBuilder().Build(),
				upgrade: newTestUpgradeBuilder().Build(),
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().NodeUpgradeStatus(testNodeName, stateUpgrading, "", "").Build(),
				err:     nil,
			},
		},
		{
			name: "upgrade node pod in waiting state",
			given: input{
				key: "upgrade-node-pod",
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "upgrade-node-pod",
						Namespace: upgradeNamespace,
						Labels: map[string]string{
							upgradePlanLabel: testPlanName,
							upgradeNodeLabel: testNodeName,
						},
					},
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Waiting: &corev1.ContainerStateWaiting{
										Reason:  "ErrImagePull",
										Message: "failed to pull and unpack image",
									},
								},
							},
						},
					},
				},
				plan:    newTestPlanBuilder().Build(),
				upgrade: newTestUpgradeBuilder().Build(),
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().NodeUpgradeStatus(testNodeName, stateUpgrading, "ErrImagePull", "failed to pull and unpack image").Build(),
				err:     nil,
			},
		},
		{
			name: "upgrade chart pod running",
			given: input{
				key: "upgrade-chart-pod",
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "upgrade-chart-pod",
						Namespace: util.KubeSystemNamespace,
						Labels: map[string]string{
							helmChartLabel: harvesterChartname,
						},
					},
				},
				plan:    newTestPlanBuilder().Build(),
				upgrade: newTestUpgradeBuilder().ChartUpgradeStatus(corev1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().ChartUpgradeStatus(corev1.ConditionUnknown, "", "").Build(),
				err:     nil,
			},
		},
		{
			name: "upgrade chart pod in waiting state",
			given: input{
				key: "upgrade-chart-pod",
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "upgrade-chart-pod",
						Namespace: upgradeNamespace,
						Labels: map[string]string{
							helmChartLabel: harvesterChartname,
						},
					},
					Status: corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{
							{
								State: corev1.ContainerState{
									Waiting: &corev1.ContainerStateWaiting{
										Reason:  "ErrImagePull",
										Message: "failed to pull and unpack image",
									},
								},
							},
						},
					},
				},
				plan:    newTestPlanBuilder().Build(),
				upgrade: newTestUpgradeBuilder().ChartUpgradeStatus(corev1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().ChartUpgradeStatus(corev1.ConditionUnknown, "ErrImagePull", "failed to pull and unpack image").Build(),
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
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}
