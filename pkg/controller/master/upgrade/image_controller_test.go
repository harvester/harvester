package upgrade

import (
	"fmt"
	"testing"

	"github.com/rancher/wrangler/v3/pkg/condition"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func newTestImportedUpgradeImage(imported corev1.ConditionStatus) *harvesterv1.VirtualMachineImage {
	image := newTestExistingVirtualMachineImage(harvesterSystemNamespace, testUpgradeImage)
	image.Labels = map[string]string{harvesterUpgradeLabel: testUpgradeName}
	image.Status.Conditions = []harvesterv1.Condition{
		{Type: condition.Cond(harvesterv1.ImageImported), Status: imported},
	}
	return image
}

func TestVMImageHandler_OnChanged(t *testing.T) {
	expectedImageID := fmt.Sprintf("%s/%s", harvesterSystemNamespace, testUpgradeImage)

	testCases := []struct {
		name            string
		upgrade         *harvesterv1.Upgrade
		image           *harvesterv1.VirtualMachineImage
		expectedImageID string
		expectedReady   corev1.ConditionStatus
	}{
		{
			// An existing image is labelled before the upgrade handler's status update
			// lands, so the cached upgrade has no image ID yet.
			name:            "imported image sets image ID when it is missing",
			upgrade:         newTestUpgradeBuilder().WithImage(expectedImageID).InitStatus().Build(),
			image:           newTestImportedUpgradeImage(corev1.ConditionTrue),
			expectedImageID: expectedImageID,
			expectedReady:   corev1.ConditionTrue,
		},
		{
			name:            "existing image ID is kept",
			upgrade:         newTestUpgradeBuilder().WithImage(expectedImageID).InitStatus().ImageIDStatus("harvester-system/other").Build(),
			image:           newTestImportedUpgradeImage(corev1.ConditionTrue),
			expectedImageID: "harvester-system/other",
			expectedReady:   corev1.ConditionTrue,
		},
	}

	for _, tc := range testCases {
		clientset := fake.NewSimpleClientset(tc.upgrade, tc.image)
		handler := &vmImageHandler{
			namespace:     harvesterSystemNamespace,
			upgradeClient: fakeclients.UpgradeClient(clientset.HarvesterhciV1beta1().Upgrades),
			upgradeCache:  fakeclients.UpgradeCache(clientset.HarvesterhciV1beta1().Upgrades),
		}

		_, err := handler.OnChanged("", tc.image)
		assert.Nil(t, err, "case %q", tc.name)

		actual, err := clientset.HarvesterhciV1beta1().Upgrades(harvesterSystemNamespace).Get(t.Context(), testUpgradeName, metav1.GetOptions{})
		assert.Nil(t, err, "case %q", tc.name)
		assert.Equal(t, tc.expectedImageID, actual.Status.ImageID, "case %q", tc.name)
		assert.Equal(t, string(tc.expectedReady), harvesterv1.ImageReady.GetStatus(actual), "case %q", tc.name)
	}
}
