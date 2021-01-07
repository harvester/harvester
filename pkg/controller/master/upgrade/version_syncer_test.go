package upgrade

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetUpgradableVersions(t *testing.T) {
	type input struct {
		versionMetaData string
		currentVersion  string
	}
	type output struct {
		upgradableVersions string
		err                error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "test1",
			given: input{
				currentVersion: "v0.1.0",
				versionMetaData: `
[
    {
        "version": "v0.3.0",
        "minUpgradableVersion": "v0.2.0"
    },
    {
        "version": "v0.2.0",
        "minUpgradableVersion": "v0.1.0"
    }
]`,
			},
			expected: output{
				upgradableVersions: "v0.2.0",
				err:                nil,
			},
		},
		{
			name: "test2",
			given: input{
				currentVersion: "dev",
				versionMetaData: `
[
  {
	  "version": "v0.1.0",
	  "minUpgradableVersion": "v0.1.0-rc1"
  },
  {
	  "version": "v0.2.0",
	  "minUpgradableVersion": "dev"
  },
  {
	  "version": "v0.2.1",
	  "minUpgradableVersion": "dev"
  }
]`,
			},
			expected: output{
				upgradableVersions: "v0.2.0,v0.2.1",
				err:                nil,
			},
		},
	}

	for _, tc := range testCases {
		var actual output
		actual.upgradableVersions, actual.err = getUpgradableVersions([]byte(tc.given.versionMetaData), tc.given.currentVersion)
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}
