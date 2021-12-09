package upgrade

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetUpgradableVersions(t *testing.T) {
	type input struct {
		newVersions    []Version
		currentVersion string
	}
	type output struct {
		canUpgrades []bool
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
				newVersions: []Version{
					{
						Name:                 "v0.3.0",
						ReleaseDate:          "2021-01-01T00:00:00Z",
						MinUpgradableVersion: "v0.2.0",
						Tags:                 []string{"latest"},
						ISOChecksum:          "xxx",
						ISOURL:               "https://somehwere/harvester.iso",
					},
					{
						Name:                 "v0.3.0",
						ReleaseDate:          "2021-01-01T00:00:00Z",
						MinUpgradableVersion: "v0.2.0",
						Tags:                 []string{"dev", "latest"},
						ISOChecksum:          "xxx",
						ISOURL:               "https://somehwere/harvester.iso",
					},
					{
						Name:                 "v0.2.0",
						ReleaseDate:          "2020-01-01T00:00:00Z",
						MinUpgradableVersion: "v0.1.0",
						Tags:                 []string{"v0.2-latest"},
						ISOChecksum:          "xxx",
						ISOURL:               "https://somehwere/harvester.iso",
					},
					// Lack of ISOURL and ISOChecksum
					{
						Name:                 "v0.2.0",
						ReleaseDate:          "2020-01-01T00:00:00Z",
						MinUpgradableVersion: "v0.1.0",
						Tags:                 []string{"v0.2-latest"},
					},
				},
			},
			expected: output{
				canUpgrades: []bool{false, true, true, false},
			},
		},
		{
			name: "test2",
			given: input{
				currentVersion: "dev",
				newVersions: []Version{
					{
						Name:                 "v0.1.0",
						ReleaseDate:          "2021-01-01T00:00:00Z",
						MinUpgradableVersion: "v0.1.0-rc1",
						Tags:                 []string{"v0.1.0"},
						ISOChecksum:          "xxx",
						ISOURL:               "https://somehwere/harvester.iso",
					},
					{
						Name:                 "v0.2.0",
						ReleaseDate:          "2020-01-01T00:00:00Z",
						MinUpgradableVersion: "dev",
						Tags:                 []string{"v0.2.0"},
						ISOChecksum:          "xxx",
						ISOURL:               "https://somehwere/harvester.iso",
					},
					{
						Name:                 "v0.2.1",
						ReleaseDate:          "2020-06-01T00:00:00Z",
						MinUpgradableVersion: "dev",
						Tags:                 []string{"v0.2.1", "latest"},
						ISOChecksum:          "xxx",
						ISOURL:               "https://somehwere/harvester.iso",
					},
				},
			},
			expected: output{
				canUpgrades: []bool{false, true, true},
			},
		},
		{
			name: "test3",
			given: input{
				currentVersion: "v0.3.0",
				newVersions: []Version{
					{
						Name:        "v0.4.0",
						ReleaseDate: "2021-01-01T00:00:00Z",
						Tags:        []string{"latest"},
						ISOChecksum: "xxx",
						ISOURL:      "https://somehwere/harvester.iso",
					},
				},
			},
			expected: output{
				canUpgrades: []bool{true},
			},
		},
		{
			name: "test4",
			given: input{
				currentVersion: "v0.2.0",
				newVersions: []Version{
					{
						Name:        "v0.2.0",
						ReleaseDate: "2021-01-01T00:00:00Z",
						Tags:        []string{"latest"},
						ISOChecksum: "xxx",
						ISOURL:      "https://somehwere/harvester.iso",
					},
				},
			},
			expected: output{
				canUpgrades: []bool{false},
			},
		},
	}

	for _, tc := range testCases {
		var actual output

		for _, newV := range tc.given.newVersions {
			actual.canUpgrades = append(actual.canUpgrades, canUpgrade(tc.given.currentVersion, newV.createVersionCR("test")))
		}

		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}
