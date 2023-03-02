package upgrade

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func TestGetUpgradableVersions(t *testing.T) {
	type input struct {
		newVersions    []harvesterv1.Version
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
				newVersions: []harvesterv1.Version{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "v0.3.0",
						},
						Spec: harvesterv1.VersionSpec{
							ReleaseDate:          "2021-01-01T00:00:00Z",
							MinUpgradableVersion: "v0.2.0",
							Tags:                 []string{"latest"},
							ISOChecksum:          "xxx",
							ISOURL:               "https://somehwere/harvester.iso",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "v0.3.0",
						},
						Spec: harvesterv1.VersionSpec{
							ReleaseDate:          "2021-01-01T00:00:00Z",
							MinUpgradableVersion: "v0.2.0",
							Tags:                 []string{"dev", "latest"},
							ISOChecksum:          "xxx",
							ISOURL:               "https://somehwere/harvester.iso",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "v0.2.0",
						},
						Spec: harvesterv1.VersionSpec{
							ReleaseDate:          "2020-01-01T00:00:00Z",
							MinUpgradableVersion: "v0.1.0",
							Tags:                 []string{"v0.2-latest"},
							ISOChecksum:          "xxx",
							ISOURL:               "https://somehwere/harvester.iso",
						},
					},
					// Lack of ISOURL and ISOChecksum
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "v0.2.0",
						},
						Spec: harvesterv1.VersionSpec{
							ReleaseDate:          "2020-01-01T00:00:00Z",
							MinUpgradableVersion: "v0.1.0",
							Tags:                 []string{"v0.2-latest"},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "v0.3.0",
						},
						Spec: harvesterv1.VersionSpec{
							ReleaseDate:          "2021-01-01T00:00:00Z",
							MinUpgradableVersion: "",
							Tags:                 []string{"latest"},
							ISOChecksum:          "xxx",
							ISOURL:               "https://somehwere/harvester.iso",
						},
					},
				},
			},
			expected: output{
				canUpgrades: []bool{false, true, true, false, true},
			},
		},
		{
			name: "test2",
			given: input{
				currentVersion: "dev",
				newVersions: []harvesterv1.Version{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "v0.1.0",
						},
						Spec: harvesterv1.VersionSpec{
							ReleaseDate:          "2021-01-01T00:00:00Z",
							MinUpgradableVersion: "v0.1.0-rc1",
							Tags:                 []string{"v0.1.0"},
							ISOChecksum:          "xxx",
							ISOURL:               "https://somehwere/harvester.iso",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "v0.2.0",
						},
						Spec: harvesterv1.VersionSpec{
							ReleaseDate:          "2020-01-01T00:00:00Z",
							MinUpgradableVersion: "dev",
							Tags:                 []string{"v0.2.0"},
							ISOChecksum:          "xxx",
							ISOURL:               "https://somehwere/harvester.iso",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "v0.2.1",
						},
						Spec: harvesterv1.VersionSpec{
							ReleaseDate:          "2020-06-01T00:00:00Z",
							MinUpgradableVersion: "dev",
							Tags:                 []string{"v0.2.1", "latest"},
							ISOChecksum:          "xxx",
							ISOURL:               "https://somehwere/harvester.iso",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "v0.2.1",
						},
						Spec: harvesterv1.VersionSpec{
							ReleaseDate:          "2020-06-01T00:00:00Z",
							MinUpgradableVersion: "",
							Tags:                 []string{"v0.2.1", "latest"},
							ISOChecksum:          "xxx",
							ISOURL:               "https://somehwere/harvester.iso",
						},
					},
				},
			},
			expected: output{
				canUpgrades: []bool{false, true, true, true},
			},
		},
		{
			name: "test3",
			given: input{
				currentVersion: "v0.3.0",
				newVersions: []harvesterv1.Version{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "v0.4.0",
						},
						Spec: harvesterv1.VersionSpec{
							ReleaseDate: "2021-01-01T00:00:00Z",
							Tags:        []string{"latest"},
							ISOChecksum: "xxx",
							ISOURL:      "https://somehwere/harvester.iso",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "v0.4.0",
						},
						Spec: harvesterv1.VersionSpec{
							ReleaseDate:          "2021-01-01T00:00:00Z",
							MinUpgradableVersion: "v0.3.0",
							Tags:                 []string{"latest"},
							ISOChecksum:          "xxx",
							ISOURL:               "https://somehwere/harvester.iso",
						},
					},
				},
			},
			expected: output{
				canUpgrades: []bool{true, true},
			},
		},
		{
			name: "test4",
			given: input{
				currentVersion: "v0.2.0",
				newVersions: []harvesterv1.Version{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "v0.2.0",
						},
						Spec: harvesterv1.VersionSpec{
							ReleaseDate: "2021-01-01T00:00:00Z",
							Tags:        []string{"latest"},
							ISOChecksum: "xxx",
							ISOURL:      "https://somehwere/harvester.iso",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "v0.2.0",
						},
						Spec: harvesterv1.VersionSpec{
							ReleaseDate:          "2021-01-01T00:00:00Z",
							MinUpgradableVersion: "v0.2.0",
							Tags:                 []string{"latest"},
							ISOChecksum:          "xxx",
							ISOURL:               "https://somehwere/harvester.iso",
						},
					},
				},
			},
			expected: output{
				canUpgrades: []bool{false, false},
			},
		},
	}

	for _, tc := range testCases {
		var actual output

		for _, newV := range tc.given.newVersions {
			actual.canUpgrades = append(actual.canUpgrades, canUpgrade(tc.given.currentVersion, &newV))
		}

		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

func Test_formatQuantityToGi(t *testing.T) {
	type args struct {
		qs string
	}
	testCases := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test1",
			args: args{
				qs: "32920204Ki",
			},
			want: "32Gi",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := resource.ParseQuantity(tc.args.qs)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, formatQuantityToGi(&q))
		})
	}
}
