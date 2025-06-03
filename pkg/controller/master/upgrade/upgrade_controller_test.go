package upgrade

import (
	"fmt"
	"strings"
	"testing"

	upgradeapiv1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const (
	testJobName         = "test-job"
	testPlanName        = "test-plan"
	testPreparePlanName = "test-upgrade-prepare"
	testNodeName        = "test-node"
	testNodeName1       = "test-node-1"
	testNodeName2       = "test-node-2"
	testNodeName3       = "test-node-3"
	testUpgradeName     = "test-upgrade"
	testUpgradeLogName  = "test-upgrade-upgradelog"
	testVersion         = "test-version"
	testUpgradeImage    = "test-upgrade-image"
	testPlanHash        = "test-hash"
)

func newTestNodeJobBuilder() *jobBuilder {
	return newJobBuilder(testJobName).
		WithLabel(upgradePlanLabel, testPlanName).
		WithLabel(upgradeNodeLabel, testNodeName)
}

func newTestPlanBuilder() *planBuilder {
	return newPlanBuilder(testPlanName).
		Version(testVersion).
		WithLabel(harvesterUpgradeLabel, testUpgradeName).
		Hash(testPlanHash)
}

func newTestChartJobBuilder() *jobBuilder {
	return newJobBuilder(testJobName).
		WithLabel(harvesterUpgradeLabel, testUpgradeName).
		WithLabel(harvesterUpgradeComponentLabel, manifestComponent)
}

func newTestUpgradeBuilder() *upgradeBuilder {
	return newUpgradeBuilder(testUpgradeName).
		WithLabel(harvesterLatestUpgradeLabel, "true").
		Version(testVersion)
}

func newTestExistingVirtualMachineImage(namespace, name string) *harvesterv1.VirtualMachineImage {
	return &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}

func newTestVirtualMachineImage() *harvesterv1.VirtualMachineImage {
	return &harvesterv1.VirtualMachineImage{
		Spec: harvesterv1.VirtualMachineImageSpec{
			DisplayName: getISODisplayNameImageName(testUpgradeName, testVersion),
		},
	}
}

func TestUpgradeHandler_OnChanged(t *testing.T) {
	type input struct {
		key     string
		upgrade *harvesterv1.Upgrade
		version *harvesterv1.Version
		vmi     *harvesterv1.VirtualMachineImage
		nodes   []*v1.Node
	}
	type output struct {
		plan       *upgradeapiv1.Plan
		upgrade    *harvesterv1.Upgrade
		upgradeLog *harvesterv1.UpgradeLog
		vmi        *harvesterv1.VirtualMachineImage
		err        error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "new upgrade with log disabled",
			given: input{
				key:     testUpgradeName,
				upgrade: newTestUpgradeBuilder().WithLogEnabled(false).Build(),
				version: newVersionBuilder(testVersion).Build(),
				vmi:     newTestExistingVirtualMachineImage(upgradeNamespace, testUpgradeImage),
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().InitStatus().
					WithLabel(upgradeStateLabel, StateLoggingInfraPrepared).
					LogReadyCondition(v1.ConditionFalse, "Disabled", "Upgrade observability is administratively disabled").Build(),
			},
		},
		{
			name: "new upgrade with log enabled",
			given: input{
				key:     testUpgradeName,
				upgrade: newTestUpgradeBuilder().WithLogEnabled(true).Build(),
				version: newVersionBuilder(testVersion).Build(),
				vmi:     newTestExistingVirtualMachineImage(upgradeNamespace, testUpgradeImage),
			},
			expected: output{
				upgradeLog: prepareUpgradeLog(newTestUpgradeBuilder().Build()),
				upgrade: newTestUpgradeBuilder().WithLogEnabled(true).InitStatus().
					WithLabel(upgradeStateLabel, StatePreparingLoggingInfra).
					UpgradeLogStatus(testUpgradeLogName).
					LogReadyCondition(v1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "upgrade triggers an image creation from ISOURL",
			given: input{
				key: testUpgradeName,
				upgrade: newTestUpgradeBuilder().
					InitStatus().
					LogReadyCondition(v1.ConditionFalse, "Disabled", "Upgrade observability is administratively disabled").Build(),
				version: newVersionBuilder(testVersion).Build(),
				vmi:     newTestExistingVirtualMachineImage(upgradeNamespace, testUpgradeImage),
				nodes: []*v1.Node{
					newNodeBuilder("node-1").Managed().ControlPlane().Build(),
					newNodeBuilder("node-2").Managed().ControlPlane().Build(),
					newNodeBuilder("node-3").Managed().ControlPlane().Build(),
				},
			},
			expected: output{
				vmi: newTestVirtualMachineImage(),
				upgrade: newTestUpgradeBuilder().InitStatus().
					LogReadyCondition(v1.ConditionFalse, "Disabled", "Upgrade observability is administratively disabled").
					ImageReadyCondition(v1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "upgrade with an existing image",
			given: input{
				key: testUpgradeName,
				upgrade: newTestUpgradeBuilder().WithImage(testUpgradeImage).
					InitStatus().
					LogReadyCondition(v1.ConditionFalse, "Disabled", "Upgrade observability is administratively disabled").Build(),
				version: newVersionBuilder(testVersion).Build(),
				vmi:     newTestExistingVirtualMachineImage(upgradeNamespace, testUpgradeImage),
				nodes: []*v1.Node{
					newNodeBuilder("node-1").Managed().ControlPlane().Build(),
					newNodeBuilder("node-2").Managed().ControlPlane().Build(),
					newNodeBuilder("node-3").Managed().ControlPlane().Build(),
				},
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().InitStatus().
					LogReadyCondition(v1.ConditionFalse, "Disabled", "Upgrade observability is administratively disabled").
					WithImage(testUpgradeImage).
					ImageIDStatus(fmt.Sprintf("%s/%s", upgradeNamespace, testUpgradeImage)).
					ImageReadyCondition(v1.ConditionUnknown, "", "").Build(),
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset(tc.given.upgrade, tc.given.version, tc.given.vmi)
		var nodes []runtime.Object
		for _, node := range tc.given.nodes {
			nodes = append(nodes, node)
		}
		var k8sclientset = k8sfake.NewSimpleClientset(nodes...)
		var handler = &upgradeHandler{
			namespace:        harvesterSystemNamespace,
			nodeCache:        fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
			planClient:       fakeclients.PlanClient(clientset.UpgradeV1().Plans),
			upgradeClient:    fakeclients.UpgradeClient(clientset.HarvesterhciV1beta1().Upgrades),
			upgradeCache:     fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades),
			upgradeLogClient: fakeclients.UpgradeLogClient(clientset.HarvesterhciV1beta1().UpgradeLogs),
			versionCache:     fakeclients.VersionCache(clientset.HarvesterhciV1beta1().Versions),
			vmClient:         fakeclients.VirtualMachineClient(clientset.KubevirtV1().VirtualMachines),
			vmImageClient:    fakeclients.VirtualMachineImageClient(clientset.HarvesterhciV1beta1().VirtualMachineImages),
			vmImageCache:     fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
		}
		var actual output
		actual.upgrade, actual.err = handler.OnChanged(tc.given.key, tc.given.upgrade)
		if tc.expected.vmi != nil {
			exist, err := fakeImageExist(handler.vmImageCache, tc.expected.vmi.Spec.DisplayName)
			assert.Nil(t, err)
			assert.True(t, exist, "case %q: fail to find image: %s", tc.name, tc.expected.vmi.Spec.DisplayName)
		}

		if tc.expected.plan != nil {
			var err error
			actual.plan, err = handler.planClient.Get(upgradeNamespace, tc.expected.plan.Name, metav1.GetOptions{})
			assert.Nil(t, err)
			//skip hash comparison
			actual.plan.Status.LatestHash = ""
			tc.expected.plan.Status.LatestHash = ""
		}

		if tc.expected.upgrade != nil {
			emptyConditionsTime(tc.expected.upgrade.Status.Conditions)
			emptyConditionsTime(actual.upgrade.Status.Conditions)

			// A Generated image ID is unpredictable. Verify
			// the image is there and compare the display name.
			imageID := actual.upgrade.Status.ImageID
			if imageID != "" && tc.expected.vmi != nil {
				tokens := strings.Split(imageID, "/")
				assert.True(t, len(tokens) == 2)
				vmi, err := handler.vmImageCache.Get(tokens[0], tokens[1])
				assert.Nil(t, err)
				assert.Equal(t, vmi.Spec.DisplayName, tc.expected.vmi.Spec.DisplayName)

				actual.upgrade.Status.ImageID = ""
			}

			assert.Equal(t, tc.expected.upgrade, actual.upgrade, "case %q", tc.name)
		}

		if tc.expected.upgradeLog != nil {
			var err error
			actual.upgradeLog, err = handler.upgradeLogClient.Get(upgradeNamespace, tc.expected.upgradeLog.Name, metav1.GetOptions{})
			assert.Nil(t, err)

			assert.Equal(t, tc.expected.upgradeLog, actual.upgradeLog, "case %q", tc.name)
		}
	}
}

func TestUpgradeHandler_prepareNodesForUpgrade(t *testing.T) {
	type input struct {
		upgrade *harvesterv1.Upgrade
		setting *harvesterv1.Setting
		nodes   []*v1.Node
	}
	type output struct {
		upgrade *harvesterv1.Upgrade
		plan    *upgradeapiv1.Plan
		err     error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "set image preload strategy type to skip will skip the creation of the prepare plan",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"imagePreloadOption":{"strategy":{"type":"skip"}}}`,
				},
				nodes: []*v1.Node{
					newNodeBuilder(testNodeName1).Build(),
					newNodeBuilder(testNodeName2).Build(),
					newNodeBuilder(testNodeName3).Build(),
				},
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().
					WithLabel(upgradeStateLabel, StatePreparingNodes).
					NodeUpgradeStatus(testNodeName1, nodeStateImagesPreloaded, "", "").
					NodeUpgradeStatus(testNodeName2, nodeStateImagesPreloaded, "", "").
					NodeUpgradeStatus(testNodeName3, nodeStateImagesPreloaded, "", "").
					NodesPreparedCondition(v1.ConditionTrue, "", "").Build(),
			},
		},
		{
			name: "set image preload strategy type to skip will skip the creation of the prepare plan even concurrency is not 0",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"imagePreloadOption":{"strategy":{"type":"skip","concurrency":1}}}`,
				},
				nodes: []*v1.Node{
					newNodeBuilder(testNodeName1).Build(),
					newNodeBuilder(testNodeName2).Build(),
					newNodeBuilder(testNodeName3).Build(),
				},
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().
					WithLabel(upgradeStateLabel, StatePreparingNodes).
					NodeUpgradeStatus(testNodeName1, nodeStateImagesPreloaded, "", "").
					NodeUpgradeStatus(testNodeName2, nodeStateImagesPreloaded, "", "").
					NodeUpgradeStatus(testNodeName3, nodeStateImagesPreloaded, "", "").
					NodesPreparedCondition(v1.ConditionTrue, "", "").Build(),
			},
		},
		{
			name: "set image preload strategy type to sequential will create the prepare plan with concurrency 1",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"imagePreloadOption":{"strategy":{"type":"sequential"}}}`,
				},
				nodes: []*v1.Node{
					newNodeBuilder(testNodeName1).Build(),
					newNodeBuilder(testNodeName2).Build(),
					newNodeBuilder(testNodeName3).Build(),
				},
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().
					WithLabel(upgradeStateLabel, StatePreparingNodes).
					NodesPreparedCondition(v1.ConditionUnknown, "", "").Build(),
				plan: newPlanBuilder(testPreparePlanName).Concurrency(1).Build(),
			},
		},
		{
			name: "set image preload strategy type to sequential will create the prepare plan with concurrency 1 even the concurrency setting is 0",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"imagePreloadOption":{"strategy":{"type":"sequential","concurrency":0}}}`,
				},
				nodes: []*v1.Node{
					newNodeBuilder(testNodeName1).Build(),
					newNodeBuilder(testNodeName2).Build(),
					newNodeBuilder(testNodeName3).Build(),
				},
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().
					WithLabel(upgradeStateLabel, StatePreparingNodes).
					NodesPreparedCondition(v1.ConditionUnknown, "", "").Build(),
				plan: newPlanBuilder(testPreparePlanName).Concurrency(1).Build(),
			},
		},
		{
			name: "set image preload strategy type to sequential will create the prepare plan with concurrency 1 even the concurrency setting is 2",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"imagePreloadOption":{"strategy":{"type":"sequential","concurrency":2}}}`,
				},
				nodes: []*v1.Node{
					newNodeBuilder(testNodeName1).Build(),
					newNodeBuilder(testNodeName2).Build(),
					newNodeBuilder(testNodeName3).Build(),
				},
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().
					WithLabel(upgradeStateLabel, StatePreparingNodes).
					NodesPreparedCondition(v1.ConditionUnknown, "", "").Build(),
				plan: newPlanBuilder(testPreparePlanName).Concurrency(1).Build(),
			},
		},
		{
			name: "set image preload strategy type to parallel and concurrency to 2 will result in a prepare plan with concurrency 2 being created",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":2}}}`,
				},
				nodes: []*v1.Node{
					newNodeBuilder(testNodeName1).Build(),
					newNodeBuilder(testNodeName2).Build(),
					newNodeBuilder(testNodeName3).Build(),
				},
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().
					WithLabel(upgradeStateLabel, StatePreparingNodes).
					NodesPreparedCondition(v1.ConditionUnknown, "", "").Build(),
				plan: newPlanBuilder(testPreparePlanName).Concurrency(2).Build(),
			},
		},
		{
			name: "set image preload strategy type to parallel and concurrency to 0 will result in a prepare plan with concurrency same as the node count being created",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":0}}}`,
				},
				nodes: []*v1.Node{
					newNodeBuilder(testNodeName1).Build(),
					newNodeBuilder(testNodeName2).Build(),
					newNodeBuilder(testNodeName3).Build(),
				},
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().
					WithLabel(upgradeStateLabel, StatePreparingNodes).
					NodesPreparedCondition(v1.ConditionUnknown, "", "").Build(),
				plan: newPlanBuilder(testPreparePlanName).Concurrency(3).Build(),
			},
		},
		{
			name: "set image preload strategy type to parallel and concurrency to 100 will result in a prepare plan with concurrency same as the node count being created",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":100}}}`,
				},
				nodes: []*v1.Node{
					newNodeBuilder(testNodeName1).Build(),
					newNodeBuilder(testNodeName2).Build(),
					newNodeBuilder(testNodeName3).Build(),
				},
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().
					WithLabel(upgradeStateLabel, StatePreparingNodes).
					NodesPreparedCondition(v1.ConditionUnknown, "", "").Build(),
				plan: newPlanBuilder(testPreparePlanName).Concurrency(3).Build(),
			},
		},
		{
			name: "set image preload strategy type with an invalid type",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"imagePreloadOption":{"strategy":{"type":"all"}}}`,
				},
				nodes: []*v1.Node{
					newNodeBuilder(testNodeName1).Build(),
					newNodeBuilder(testNodeName2).Build(),
					newNodeBuilder(testNodeName3).Build(),
				},
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().Build(),
				err:     fmt.Errorf("invalid image preload strategy type: all"),
			},
		},
		{
			name: "set image preload strategy concurrency to a negative value",
			given: input{
				upgrade: newTestUpgradeBuilder().Build(),
				setting: &harvesterv1.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: settings.UpgradeConfigSettingName,
					},
					Value: `{"imagePreloadOption":{"strategy":{"type":"parallel","concurrency":-1}}}`,
				},
				nodes: []*v1.Node{
					newNodeBuilder(testNodeName1).Build(),
					newNodeBuilder(testNodeName2).Build(),
					newNodeBuilder(testNodeName3).Build(),
				},
			},
			expected: output{
				upgrade: newTestUpgradeBuilder().Build(),
				err:     fmt.Errorf("invalid image preload strategy concurrency: -1"),
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
			nodeCache:     fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
			upgradeClient: fakeclients.UpgradeClient(clientset.HarvesterhciV1beta1().Upgrades),
			upgradeCache:  fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades),
			planClient:    fakeclients.PlanClient(clientset.UpgradeV1().Plans),
		}

		var err error
		err = settings.UpgradeConfigSet.Set(tc.given.setting.Value)
		assert.Nil(t, err, "case %q", tc.name)

		var actual output
		actual.upgrade, actual.err = handler.prepareNodesForUpgrade(tc.given.upgrade, "")
		assert.Equal(t, tc.expected.err, actual.err, "case %q", tc.name)
		assert.Equal(t, tc.expected.upgrade, actual.upgrade, "case %q", tc.name)

		if tc.expected.plan != nil {
			actual.plan, err = handler.planClient.Get(sucNamespace, tc.expected.plan.Name, metav1.GetOptions{})
			assert.Nil(t, err, "case %q", tc.name)
			assert.Equal(t, tc.expected.plan.Spec.Concurrency, actual.plan.Spec.Concurrency, "case %q", tc.name)
		}
	}
}

func fakeImageExist(imageCache ctlharvesterv1.VirtualMachineImageCache, displayName string) (bool, error) {
	vmis, err := imageCache.List(upgradeNamespace, labels.Everything())
	if err != nil {
		return false, err
	}

	for _, vmi := range vmis {
		if vmi.Spec.DisplayName == displayName {
			return true, nil
		}
	}
	return false, nil
}

func emptyConditionsTime(conditions []harvesterv1.Condition) {
	for k := range conditions {
		conditions[k].LastTransitionTime = ""
		conditions[k].LastUpdateTime = ""
	}
}
