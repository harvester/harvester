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
		plan    *upgradeapiv1.Plan
		upgrade *harvesterv1.Upgrade
		vmi     *harvesterv1.VirtualMachineImage
		err     error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "upgrade triggers an image creation from ISOURL",
			given: input{
				key:     testUpgradeName,
				upgrade: newTestUpgradeBuilder().Build(),
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
					ImageReadyCondition(v1.ConditionUnknown, "", "").Build(),
			},
		},
		{
			name: "upgrade with an existing image",
			given: input{
				key:     testUpgradeName,
				upgrade: newTestUpgradeBuilder().WithImage(testUpgradeImage).Build(),
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
			namespace:     harvesterSystemNamespace,
			nodeCache:     fakeclients.NodeCache(k8sclientset.CoreV1().Nodes),
			planClient:    fakeclients.PlanClient(clientset.UpgradeV1().Plans),
			upgradeClient: fakeclients.UpgradeClient(clientset.HarvesterhciV1beta1().Upgrades),
			upgradeCache:  fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades),
			versionCache:  fakeclients.VersionCache(clientset.HarvesterhciV1beta1().Versions),
			vmClient:      fakeclients.VirtualMachineClient(clientset.KubevirtV1().VirtualMachines),
			vmImageClient: fakeclients.VirtualMachineImageClient(clientset.HarvesterhciV1beta1().VirtualMachineImages),
			vmImageCache:  fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
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
	for _, c := range conditions {
		c.LastTransitionTime = ""
		c.LastUpdateTime = ""
	}
}
