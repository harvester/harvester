package upgrade

import (
	"testing"

	upgradeapiv1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/upgrade/repoinfo"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
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
		{
			name: "a previous upgrading manifest job should not complete upgrade",
			given: input{
				key: testJobName,
				job: newJobBuilder(testJobName).
					WithLabel(harvesterUpgradeLabel, "test-upgrade-old").
					WithLabel(harvesterUpgradeComponentLabel, manifestComponent).
					Completed().Build(),
				plan:    newTestPlanBuilder().Build(),
				upgrade: newTestUpgradeBuilder().WithLabel(harvesterLatestUpgradeLabel, "true").ChartUpgradeStatus(v1.ConditionUnknown, "", "").Build(),
			},
			expected: output{
				job: newJobBuilder(testJobName).
					WithLabel(harvesterUpgradeLabel, "test-upgrade-old").
					WithLabel(harvesterUpgradeComponentLabel, manifestComponent).
					Completed().Build(),
				upgrade: newTestUpgradeBuilder().WithLabel(harvesterLatestUpgradeLabel, "true").ChartUpgradeStatus(v1.ConditionUnknown, "", "").Build(),
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

func TestJobHandler_sendRestoreVMJob(t *testing.T) {
	type input struct {
		upgrade   *harvesterv1.Upgrade
		node      *v1.Node
		configMap *v1.ConfigMap
		jobs      []*batchv1.Job
		restoreVM bool
	}
	type output struct {
		shouldCreateJob bool
		err             error
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "restore VM disabled - should not create job",
			given: input{
				upgrade:   newTestUpgradeBuilder().Build(),
				node:      newNodeBuilder("test-node").Build(),
				restoreVM: false,
			},
			expected: output{
				shouldCreateJob: false,
				err:             nil,
			},
		},
		{
			name: "witness node - should not create job",
			given: input{
				upgrade:   newTestUpgradeBuilder().Build(),
				node:      newNodeBuilder("test-node").WithLabel(util.HarvesterWitnessNodeLabelKey, "true").Build(),
				restoreVM: true,
			},
			expected: output{
				shouldCreateJob: false,
				err:             nil,
			},
		},
		{
			name: "no ConfigMap - should not create job",
			given: input{
				upgrade:   newTestUpgradeBuilder().Build(),
				node:      newNodeBuilder("test-node").Build(),
				configMap: nil,
				restoreVM: true,
			},
			expected: output{
				shouldCreateJob: false,
				err:             nil,
			},
		},
		{
			name: "ConfigMap with nil Data - should not create job",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				node:    newNodeBuilder("test-node").Build(),
				configMap: &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      util.GetRestoreVMConfigMapName("test-upgrade"),
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Data: nil,
				},
				restoreVM: true,
			},
			expected: output{
				shouldCreateJob: false,
				err:             nil,
			},
		},
		{
			name: "ConfigMap with empty string - should not create job",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				node:    newNodeBuilder("test-node").Build(),
				configMap: &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      util.GetRestoreVMConfigMapName("test-upgrade"),
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Data: map[string]string{
						"test-node": "",
					},
				},
				restoreVM: true,
			},
			expected: output{
				shouldCreateJob: false,
				err:             nil,
			},
		},
		{
			name: "ConfigMap with whitespace only - should not create job",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				node:    newNodeBuilder("test-node").Build(),
				configMap: &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      util.GetRestoreVMConfigMapName("test-upgrade"),
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Data: map[string]string{
						"test-node": "   \t\n  ",
					},
				},
				restoreVM: true,
			},
			expected: output{
				shouldCreateJob: false,
				err:             nil,
			},
		},
		{
			name: "ConfigMap with VM names - should create job",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				node:    newNodeBuilder("test-node").Build(),
				configMap: &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      util.GetRestoreVMConfigMapName("test-upgrade"),
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Data: map[string]string{
						"test-node": "default/vm1,default/vm2",
					},
				},
				restoreVM: true,
			},
			expected: output{
				shouldCreateJob: true,
				err:             nil,
			},
		},
		{
			name: "job already exists - should not create job",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				node:    newNodeBuilder("test-node").Build(),
				configMap: &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      util.GetRestoreVMConfigMapName("test-upgrade"),
						Namespace: util.HarvesterSystemNamespaceName,
					},
					Data: map[string]string{
						"test-node": "default/vm1",
					},
				},
				jobs: []*batchv1.Job{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-upgrade-test-node-restore-vm",
							Namespace: "harvester-system",
						},
					},
				},
				restoreVM: true,
			},
			expected: output{
				shouldCreateJob: false,
				err:             nil,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup fake clients
			var k8sObjects []runtime.Object
			if tc.given.configMap != nil {
				k8sObjects = append(k8sObjects, tc.given.configMap)
			}
			for _, job := range tc.given.jobs {
				k8sObjects = append(k8sObjects, job)
			}

			var harvesterObjects []runtime.Object
			harvesterObjects = append(harvesterObjects, tc.given.upgrade)
			// Mock the setting
			if tc.given.restoreVM {
				setting := &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"restoreVM":true}`,
				}
				harvesterObjects = append(harvesterObjects, setting)
			}

			k8sClientset := k8sfake.NewSimpleClientset(k8sObjects...)
			harvesterClientset := fake.NewSimpleClientset(harvesterObjects...)

			handler := &jobHandler{
				namespace:      util.HarvesterSystemNamespaceName,
				upgradeCache:   fakeclients.UpgradeCache(harvesterClientset.HarvesterhciV1beta1().Upgrades),
				jobClient:      fakeclients.JobClient(k8sClientset.BatchV1().Jobs),
				jobCache:       fakeclients.JobCache(k8sClientset.BatchV1().Jobs),
				configMapCache: fakeclients.ConfigmapCache(k8sClientset.CoreV1().ConfigMaps),
				settingCache:   fakeclients.HarvesterSettingCache(harvesterClientset.HarvesterhciV1beta1().Settings),
			}

			// Create mock repo info
			repoInfo := &repoinfo.RepoInfo{
				Release: repoinfo.HarvesterRelease{
					OS: "test-os",
				},
			}

			// Call the method
			err := handler.sendRestoreVMJob(tc.given.upgrade, tc.given.node, repoInfo)

			// Verify error expectation
			if tc.expected.err != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expected.err.Error())
			} else {
				assert.NoError(t, err)
			}

			// Verify job creation
			if tc.expected.shouldCreateJob {
				expectedJobName := tc.given.upgrade.Name + "-" + upgradeJobTypeRestoreVM + "-" + tc.given.node.Name
				job, err := handler.jobCache.Get(util.HarvesterSystemNamespaceName, expectedJobName)
				assert.NoError(t, err)
				assert.NotNil(t, job)
				assert.Equal(t, upgradeJobTypeRestoreVM, job.Labels[upgradeJobTypeLabel])
			}
		})
	}
}
