package upgrade

import (
	"testing"

	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harvester/harvester/pkg/util"
)

// Test_filterKubevirtComparePatches tests the filterKubevirtComparePatches function
// which removes the specific jsonPointer "/spec/workloadUpdateStrategy/workloadUpdateMethods"
// from kubevirt comparePatches while preserving other jsonPointers and patches.
func Test_filterKubevirtComparePatches(t *testing.T) {
	const targetJsonPointer = "/spec/workloadUpdateStrategy/workloadUpdateMethods"

	tests := []struct {
		name               string
		inputPatches       []fleet.ComparePatch
		expectedRemoved    bool
		expectedPatchCount int
		validateResult     func(t *testing.T, result []fleet.ComparePatch)
	}{
		{
			name:               "empty comparePatches",
			inputPatches:       []fleet.ComparePatch{},
			expectedRemoved:    false,
			expectedPatchCount: 0,
			validateResult: func(t *testing.T, result []fleet.ComparePatch) {
				assert.Empty(t, result)
			},
		},
		{
			name: "kubevirt patch with only target jsonPointer - should be removed entirely",
			inputPatches: []fleet.ComparePatch{
				{
					APIVersion: "kubevirt.io/v1",
					Kind:       "KubeVirt",
					Name:       "kubevirt",
					JsonPointers: []string{
						targetJsonPointer,
					},
				},
			},
			expectedRemoved:    true,
			expectedPatchCount: 0,
			validateResult: func(t *testing.T, result []fleet.ComparePatch) {
				assert.Empty(t, result, "patch should be completely removed when it only contains target jsonPointer")
			},
		},
		{
			name: "kubevirt patch with target and other jsonPointers - should keep patch with other pointers",
			inputPatches: []fleet.ComparePatch{
				{
					APIVersion: "kubevirt.io/v1",
					Kind:       "KubeVirt",
					Name:       "kubevirt",
					JsonPointers: []string{
						targetJsonPointer,
						"/spec/someOtherField",
						"/spec/configuration/developerConfiguration",
					},
				},
			},
			expectedRemoved:    true,
			expectedPatchCount: 1,
			validateResult: func(t *testing.T, result []fleet.ComparePatch) {
				require.Len(t, result, 1, "should keep one patch")
				patch := result[0]
				assert.Equal(t, "kubevirt.io/v1", patch.APIVersion)
				assert.Equal(t, "KubeVirt", patch.Kind)
				assert.Equal(t, "kubevirt", patch.Name)
				assert.Len(t, patch.JsonPointers, 2, "should have 2 remaining jsonPointers")
				assert.Equal(t, []string{"/spec/someOtherField", "/spec/configuration/developerConfiguration"}, patch.JsonPointers)
				assert.NotContains(t, patch.JsonPointers, targetJsonPointer, "target jsonPointer should be removed")
			},
		},
		{
			name: "kubevirt patch without target jsonPointer - should remain unchanged",
			inputPatches: []fleet.ComparePatch{
				{
					APIVersion: "kubevirt.io/v1",
					Kind:       "KubeVirt",
					Name:       "kubevirt",
					JsonPointers: []string{
						"/spec/someOtherField",
						"/spec/anotherField",
					},
				},
			},
			expectedRemoved:    false,
			expectedPatchCount: 1,
			validateResult: func(t *testing.T, result []fleet.ComparePatch) {
				require.Len(t, result, 1)
				assert.Equal(t, []string{"/spec/someOtherField", "/spec/anotherField"}, result[0].JsonPointers)
			},
		},
		{
			name: "non-kubevirt patches should remain unchanged",
			inputPatches: []fleet.ComparePatch{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "test-deployment",
					JsonPointers: []string{
						"/spec/replicas",
					},
				},
				{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       "test-service",
					JsonPointers: []string{
						"/spec/ports",
					},
				},
			},
			expectedRemoved:    false,
			expectedPatchCount: 2,
			validateResult: func(t *testing.T, result []fleet.ComparePatch) {
				require.Len(t, result, 2)
				assert.Equal(t, "apps/v1", result[0].APIVersion)
				assert.Equal(t, "v1", result[1].APIVersion)
			},
		},
		{
			name: "mixed patches - kubevirt with target and other patches",
			inputPatches: []fleet.ComparePatch{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "test-deployment",
					JsonPointers: []string{
						"/spec/replicas",
					},
				},
				{
					APIVersion: "kubevirt.io/v1",
					Kind:       "KubeVirt",
					Name:       "kubevirt",
					JsonPointers: []string{
						targetJsonPointer,
					},
				},
				{
					APIVersion: "v1",
					Kind:       "Service",
					Name:       "test-service",
					JsonPointers: []string{
						"/spec/ports",
					},
				},
			},
			expectedRemoved:    true,
			expectedPatchCount: 2,
			validateResult: func(t *testing.T, result []fleet.ComparePatch) {
				require.Len(t, result, 2, "should have 2 patches after removing kubevirt patch")
				assert.Equal(t, "apps/v1", result[0].APIVersion)
				assert.Equal(t, "v1", result[1].APIVersion)
				// Verify kubevirt patch is not present
				for _, patch := range result {
					assert.NotEqual(t, "kubevirt.io/v1", patch.APIVersion)
				}
			},
		},
		{
			name: "kubevirt patch with wrong name - should remain unchanged",
			inputPatches: []fleet.ComparePatch{
				{
					APIVersion: "kubevirt.io/v1",
					Kind:       "KubeVirt",
					Name:       "different-kubevirt",
					JsonPointers: []string{
						targetJsonPointer,
					},
				},
			},
			expectedRemoved:    false,
			expectedPatchCount: 1,
			validateResult: func(t *testing.T, result []fleet.ComparePatch) {
				require.Len(t, result, 1)
				assert.Equal(t, "different-kubevirt", result[0].Name)
				assert.Contains(t, result[0].JsonPointers, targetJsonPointer, "should not filter patches with different name")
			},
		},
		{
			name: "kubevirt patch with wrong kind - should remain unchanged",
			inputPatches: []fleet.ComparePatch{
				{
					APIVersion: "kubevirt.io/v1",
					Kind:       "VirtualMachine",
					Name:       "kubevirt",
					JsonPointers: []string{
						targetJsonPointer,
					},
				},
			},
			expectedRemoved:    false,
			expectedPatchCount: 1,
			validateResult: func(t *testing.T, result []fleet.ComparePatch) {
				require.Len(t, result, 1)
				assert.Equal(t, "VirtualMachine", result[0].Kind)
				assert.Contains(t, result[0].JsonPointers, targetJsonPointer, "should not filter patches with different kind")
			},
		},
		{
			name: "multiple kubevirt patches - only matching one should be modified",
			inputPatches: []fleet.ComparePatch{
				{
					APIVersion: "kubevirt.io/v1",
					Kind:       "KubeVirt",
					Name:       "kubevirt",
					Namespace:  util.HarvesterSystemNamespaceName,
					JsonPointers: []string{
						targetJsonPointer,
						"/spec/otherField",
					},
				},
				{
					APIVersion: "kubevirt.io/v1",
					Kind:       "KubeVirt",
					Name:       "other-kubevirt",
					JsonPointers: []string{
						"/spec/someField",
					},
				},
			},
			expectedRemoved:    true,
			expectedPatchCount: 2,
			validateResult: func(t *testing.T, result []fleet.ComparePatch) {
				require.Len(t, result, 2)
				// Find the kubevirt patch
				var kubevirtPatch *fleet.ComparePatch
				var otherPatch *fleet.ComparePatch
				for i := range result {
					switch result[i].Name {
					case "kubevirt":
						kubevirtPatch = &result[i]
					case "other-kubevirt":
						otherPatch = &result[i]
					}
				}
				require.NotNil(t, kubevirtPatch, "kubevirt patch should still exist")
				require.NotNil(t, otherPatch, "other-kubevirt patch should exist")

				assert.Equal(t, []string{"/spec/otherField"}, kubevirtPatch.JsonPointers, "target jsonPointer should be removed from matching patch")
				assert.Equal(t, []string{"/spec/someField"}, otherPatch.JsonPointers, "non-matching patch should be unchanged")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualPatches, actualRemoved := filterKubevirtComparePatches(tt.inputPatches, targetJsonPointer)

			assert.Equal(t, tt.expectedRemoved, actualRemoved, "removed flag should match expected")
			assert.Equal(t, tt.expectedPatchCount, len(actualPatches), "patch count should match expected")

			if tt.validateResult != nil {
				tt.validateResult(t, actualPatches)
			}
		})
	}
}
