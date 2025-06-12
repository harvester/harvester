package vmlivemigratedetector

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"
)

func TestVMSelector(t *testing.T) {
	tests := []struct {
		name           string
		nodeName       string
		excludeRepoVM  bool
		expectedLabels map[string]string
		excludeLabel   string
	}{
		{
			name:          "No exclusion, match node label",
			nodeName:      "node-1",
			excludeRepoVM: false,
			expectedLabels: map[string]string{
				"kubevirt.io/nodeName": "node-1",
			},
		},
		{
			name:          "Exclude upgrade VMs",
			nodeName:      "node-2",
			excludeRepoVM: true,
			expectedLabels: map[string]string{
				"kubevirt.io/nodeName": "node-2",
			},
			excludeLabel: "harvesterhci.io/upgrade",
		},
	}

	assert := require.New(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			selector, err := vmSelector(tc.nodeName, tc.excludeRepoVM)
			assert.NoError(err)

			// Must match expected nodeName
			assert.True(selector.Matches(labels.Set(tc.expectedLabels)))

			// Must not match excluded label if applicable
			if tc.excludeLabel != "" {
				excludeSet := labels.Set{
					"kubevirt.io/nodeName": tc.nodeName,
					tc.excludeLabel:        "true",
				}
				assert.False(selector.Matches(excludeSet))
			}
		})
	}
}
