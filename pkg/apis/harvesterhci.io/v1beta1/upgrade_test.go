package v1beta1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func TestGetUpgradeImage(t *testing.T) {
	testCases := []struct {
		name         string
		upgrade      *Upgrade
		defaultImage string
		expected     string
	}{
		{
			name: "Without spec.upgradeImage",
			upgrade: &Upgrade{
				Spec: UpgradeSpec{},
			},
			defaultImage: "xyz:v1.0.0",
			expected:     "xyz:v1.0.0",
		},
		{
			name: "With nil spec.upgradeImage",
			upgrade: &Upgrade{
				Spec: UpgradeSpec{
					UpgradeImage: nil,
				},
			},
			defaultImage: "xyz:v1.0.0",
			expected:     "xyz:v1.0.0",
		},
		{
			name: "With spec.upgradeImage",
			upgrade: &Upgrade{
				Spec: UpgradeSpec{
					UpgradeImage: ptr.To("foo:v1.2.3"),
				},
			},
			defaultImage: "xyz:v1.0.0",
			expected:     "foo:v1.2.3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.upgrade.GetUpgradeImage(tc.defaultImage)
			assert.Equal(t, tc.expected, result, "case %q", tc.name)
		})
	}
}
