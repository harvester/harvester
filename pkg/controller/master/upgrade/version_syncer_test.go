package upgrade

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetUpgradableVersions(t *testing.T) {
	type input struct {
		config         CheckUpgradeResponse
		currentVersion string
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
				config: CheckUpgradeResponse{
					Versions: []Version{
						{
							Name:                 "v0.3.0",
							ReleaseDate:          "2021-01-01T00:00:00Z",
							MinUpgradableVersion: "v0.2.0",
							Tags:                 []string{"latest"},
						},
						{
							Name:                 "v0.2.0",
							ReleaseDate:          "2020-01-01T00:00:00Z",
							MinUpgradableVersion: "v0.1.0",
							Tags:                 []string{"v0.2-latest"},
						},
					},
				},
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
				config: CheckUpgradeResponse{
					Versions: []Version{
						{
							Name:                 "v0.1.0",
							ReleaseDate:          "2021-01-01T00:00:00Z",
							MinUpgradableVersion: "v0.1.0-rc1",
							Tags:                 []string{"v0.1.0"},
						},
						{
							Name:                 "v0.2.0",
							ReleaseDate:          "2020-01-01T00:00:00Z",
							MinUpgradableVersion: "dev",
							Tags:                 []string{"v0.2.0"},
						},
						{
							Name:                 "v0.2.1",
							ReleaseDate:          "2020-06-01T00:00:00Z",
							MinUpgradableVersion: "dev",
							Tags:                 []string{"v0.2.1", "latest"},
						},
					},
				},
			},
			expected: output{
				upgradableVersions: "v0.2.0,v0.2.1",
				err:                nil,
			},
		},
		{
			name: "test3",
			given: input{
				currentVersion: "v0.3.0",
				config: CheckUpgradeResponse{
					Versions: []Version{
						{
							Name:        "v0.4.0",
							ReleaseDate: "2021-01-01T00:00:00Z",
							Tags:        []string{"latest"},
						},
					},
				},
			},
			expected: output{
				upgradableVersions: "v0.4.0",
				err:                nil,
			},
		},
		{
			name: "test4",
			given: input{
				currentVersion: "v0.2.0",
				config: CheckUpgradeResponse{
					Versions: []Version{
						{
							Name:        "v0.2.0",
							ReleaseDate: "2021-01-01T00:00:00Z",
							Tags:        []string{"latest"},
						},
					},
				},
			},
			expected: output{
				upgradableVersions: "",
				err:                nil,
			},
		},
	}

	for _, tc := range testCases {
		var actual output
		actual.upgradableVersions, actual.err = getUpgradableVersions(tc.given.config, tc.given.currentVersion)
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}
