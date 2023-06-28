package addon

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func Test_addonAnnotationsPatch(t *testing.T) {

	var testCases = []struct {
		name      string
		addon     *harvesterv1.Addon
		operation string
		input     types.PatchOps
		output    types.PatchOps
	}{
		{
			name: "add new annotation to record 'enable' operation when creating a new addon",
			addon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			operation: "enable", // create a new addon, which is enabled
			input:     types.PatchOps{},
			output: types.PatchOps{
				`{"op": "add", "path": "/metadata/annotations/harvesterhci.io~1addon-last-operation", "value": "enable"}`,
			},
		},
		{
			name: "add new annotation to record 'create' operation",
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
			operation: "disable", // create a new addon, which is disabled
			input:     types.PatchOps{},
			output: types.PatchOps{
				`{"op": "add", "path": "/metadata/annotations/harvesterhci.io~1addon-last-operation", "value": "disable"}`,
			},
		},
		{
			name: "add new annotation to record 'update' operation",
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
			operation: "update", // update addon, e.g. valueContents
			input:     types.PatchOps{},
			output: types.PatchOps{
				`{"op": "add", "path": "/metadata/annotations/harvesterhci.io~1addon-last-operation", "value": "update"}`,
			},
		},
		{
			name: "replace annotation to record 'update' operation",
			addon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
					Annotations: map[string]string{
						"harvesterhci.io/addon-last-operation": "create",
					},
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			operation: "update",
			input:     types.PatchOps{},
			output: types.PatchOps{
				`{"op": "replace", "path": "/metadata/annotations/harvesterhci.io~1addon-last-operation", "value": "update"}`,
			},
		},
		{
			name: "replace annotation to record 'disable' operation",
			addon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
					Annotations: map[string]string{
						"harvesterhci.io/addon-last-operation": "enable",
					},
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       false,
					ValuesContent: "sample",
				},
			},
			operation: "disable", // disable an addon
			input:     types.PatchOps{},
			output: types.PatchOps{
				`{"op": "replace", "path": "/metadata/annotations/harvesterhci.io~1addon-last-operation", "value": "disable"}`,
			},
		},
		{
			name: "replace annotation to record 'enable' operation",
			addon: &harvesterv1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "addon1",
					Annotations: map[string]string{
						"harvesterhci.io/addon-last-operation": "disable",
					},
				},
				Spec: harvesterv1.AddonSpec{
					Repo:          "repo1",
					Chart:         "chart1",
					Version:       "version1",
					Enabled:       true,
					ValuesContent: "sample",
				},
			},
			operation: "enable", // enable an addon
			input:     types.PatchOps{},
			output: types.PatchOps{
				`{"op": "replace", "path": "/metadata/annotations/harvesterhci.io~1addon-last-operation", "value": "enable"}`,
			},
		},
	}
	for _, testCase := range testCases {
		result, err := patchLastOperation(testCase.addon, testCase.input, testCase.operation)
		assert.Empty(t, err)
		assert.Equal(t, testCase.output[0], result[0])
	}
}
