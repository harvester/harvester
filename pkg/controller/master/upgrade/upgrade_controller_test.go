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
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

const (
	testJobName        = "test-job"
	testPlanName       = "test-plan"
	testNodeName       = "test-node"
	testUpgradeName    = "test-upgrade"
	testUpgradeLogName = "test-upgrade-upgradelog"
	testVersion        = "test-version"
	testUpgradeImage   = "test-upgrade-image"
	testPlanHash       = "test-hash"
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

func newTestUpgradeLog() *harvesterv1.UpgradeLog {
	return &harvesterv1.UpgradeLog{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				harvesterUpgradeLabel: testUpgradeName,
			},
			Name:      testUpgradeLogName,
			Namespace: harvesterSystemNamespace,
		},
		Spec: harvesterv1.UpgradeLogSpec{
			UpgradeName: testUpgradeName,
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

func Test_isVersionUpgradable(t *testing.T) {
	var testCases = []struct {
		name                 string
		currentVersion       string
		minUpgradableVersion string
		isUpgradable         bool
	}{
		{
			name:                 "upgrade from versions above the minimal requirement",
			currentVersion:       "v1.1.1",
			minUpgradableVersion: "v1.1.0",
			isUpgradable:         true,
		},
		{
			name:                 "upgrade from the exact version of the minimal requirement",
			currentVersion:       "v1.1.0",
			minUpgradableVersion: "v1.1.0",
			isUpgradable:         true,
		},
		{
			name:                 "upgrade from rc versions lower than the minimal requirement",
			currentVersion:       "v1.1.0-rc1",
			minUpgradableVersion: "v1.1.0",
			isUpgradable:         false,
		},
		{
			name:                 "upgrade from rc versions above the minimal requirement (rc minUpgradableVersion)",
			currentVersion:       "v1.1.0-rc2",
			minUpgradableVersion: "v1.1.0-rc1",
			isUpgradable:         true,
		},
		{
			name:                 "upgrade from rc versions lower than the minimal requirement (rc minUpgradableVersion)",
			currentVersion:       "v1.1.0-rc1",
			minUpgradableVersion: "v1.1.0-rc2",
			isUpgradable:         false,
		},
		{
			name:                 "upgrade from the exact rc version of the minimal requirement (rc minUpgradableVersion)",
			currentVersion:       "v1.1.0-rc1",
			minUpgradableVersion: "v1.1.0-rc1",
			isUpgradable:         true,
		},
		{
			name:                 "upgrade from versions lower than the minimal requirement",
			currentVersion:       "v1.0.3",
			minUpgradableVersion: "v1.1.0",
			isUpgradable:         false,
		},
	}
	for _, tc := range testCases {
		err := isVersionUpgradable(tc.currentVersion, tc.minUpgradableVersion)
		if tc.isUpgradable {
			assert.Nil(t, err, "case %q", tc.name)
		} else {
			assert.NotNil(t, err, "case %q", tc.name)
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
