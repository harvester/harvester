package addon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	//	corev1 "k8s.io/api/core/v1"
	//	"k8s.io/utils/pointer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	//	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func Test_volumeMountPatch(t *testing.T) {

	var testCases = []struct {
		name   string
		addon  *harvesterv1.Addon
		input  types.PatchOps
		output types.PatchOps
	}{
		{
			name: "patch addon operation create",
			addon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			intput: types.PatchOps,
			output: `[{"op": "add", "path": "/spec/annotations/harvesterhci.io/addon-last-operation", "value": "create"}]`,
		},
	}
	for _, testCase := range testCases {
		result, err := patchLastOperation(testCase.addon, patchOps, "create")

		assert.Empty(t, err)
		assert.Equal(t, testCase.output[0], result[0])
	}
}
