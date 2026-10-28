package upgrade

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/upgrade/repoinfo"
	"github.com/harvester/harvester/pkg/util"
)

func TestUpgradeImages(t *testing.T) {
	const (
		previousUpgradeImage = "registry.example.com/harvester/harvester-upgrade:v1.0.0"
		overrideUpgradeImage = "registry.example.com/dev/harvester-upgrade:dev"
	)

	newUpgrade := func(annotatedImage string, upgradeImage *string) *harvesterv1.Upgrade {
		upgrade := newTestUpgradeBuilder().Build()
		upgrade.Status.PreviousVersion = "v1.0.0"
		upgrade.Spec.UpgradeImage = upgradeImage
		if annotatedImage != "" {
			upgrade.Annotations = map[string]string{util.AnnotationPreviousUpgradeImage: annotatedImage}
		}
		return upgrade
	}
	newRepoInfo := func(registry string) *repoinfo.RepoInfo {
		return &repoinfo.RepoInfo{
			Release: repoinfo.HarvesterRelease{
				Harvester:                      "v1.1.0",
				HarvesterSystemDefaultRegistry: registry,
			},
		}
	}

	var testCases = []struct {
		name             string
		upgrade          *harvesterv1.Upgrade
		repoInfo         *repoinfo.RepoInfo
		expectedPrevious string
		expectedTarget   string
	}{
		{
			name:             "community to community",
			upgrade:          newUpgrade("rancher/harvester-upgrade:v1.0.0", nil),
			repoInfo:         newRepoInfo(""),
			expectedPrevious: "rancher/harvester-upgrade:v1.0.0",
			expectedTarget:   "rancher/harvester-upgrade:v1.1.0",
		},
		{
			name:             "community to Prime",
			upgrade:          newUpgrade("rancher/harvester-upgrade:v1.0.0", nil),
			repoInfo:         newRepoInfo("registry.example.com"),
			expectedPrevious: "rancher/harvester-upgrade:v1.0.0",
			expectedTarget:   "registry.example.com/harvester/harvester-upgrade:v1.1.0",
		},
		{
			name:             "Prime to Prime",
			upgrade:          newUpgrade(previousUpgradeImage, nil),
			repoInfo:         newRepoInfo("registry.example.com"),
			expectedPrevious: previousUpgradeImage,
			expectedTarget:   "registry.example.com/harvester/harvester-upgrade:v1.1.0",
		},
		{
			name:             "upgrades started by older versions use the community image",
			upgrade:          newUpgrade("", nil),
			repoInfo:         newRepoInfo(""),
			expectedPrevious: "rancher/harvester-upgrade:v1.0.0",
			expectedTarget:   "rancher/harvester-upgrade:v1.1.0",
		},
		{
			name:             "spec.upgradeImage overrides both images",
			upgrade:          newUpgrade(previousUpgradeImage, ptr.To(overrideUpgradeImage)),
			repoInfo:         newRepoInfo("registry.example.com"),
			expectedPrevious: overrideUpgradeImage,
			expectedTarget:   overrideUpgradeImage,
		},
	}

	for _, tc := range testCases {
		previousImages := map[string]string{
			"skip manifests plan": prepareSkipManifestPlan(tc.upgrade, manifestsToSkip, true).Spec.Upgrade.Image,
			"cleanup plan":        prepareCleanupPlan(tc.upgrade, nil).Spec.Upgrade.Image,
			"image preload plan":  preparePlan(tc.upgrade, defaultImagePreloadConcurrency).Spec.Upgrade.Image,
		}
		for job, image := range previousImages {
			assert.Equal(t, tc.expectedPrevious, image, "case %q: %s", tc.name, job)
		}

		targetImages := map[string]string{
			"node job":       applyNodeJob(tc.upgrade, tc.repoInfo, testNodeName, upgradeJobTypePreDrain).Spec.Template.Spec.Containers[0].Image,
			"restore VM job": applyRestoreVMJob(tc.upgrade, tc.repoInfo, testNodeName).Spec.Template.Spec.Containers[0].Image,
			"manifests job":  applyManifestsJob(tc.upgrade, tc.repoInfo).Spec.Template.Spec.Containers[0].Image,
		}
		for job, image := range targetImages {
			assert.Equal(t, tc.expectedTarget, image, "case %q: %s", tc.name, job)
		}
	}
}

func TestInitStatusRecordsPreviousUpgradeImage(t *testing.T) {
	upgrade := newTestUpgradeBuilder().Build()
	initStatus(upgrade)

	// Without a settings provider, the version and registry settings return their defaults.
	assert.Equal(t, "rancher/harvester-upgrade:dev", upgrade.Annotations[util.AnnotationPreviousUpgradeImage])
}

func TestApplyImageRetainList(t *testing.T) {
	tests := []struct {
		name     string
		images   []string
		expected []string
	}{
		{
			name: "community upgrade image",
			images: []string{
				"docker.io/rancher/harvester-upgrade:v1.0.0",
				"docker.io/rancher/harvester:v1.0.0",
			},
			expected: []string{"docker.io/rancher/harvester:v1.0.0"},
		},
		{
			name: "upgrade image under a registry",
			images: []string{
				"registry.example.com/harvester/harvester-upgrade:v1.0.0",
				"registry.example.com/harvester/harvester:v1.0.0",
			},
			expected: []string{"registry.example.com/harvester/harvester:v1.0.0"},
		},
		{
			name:     "other images",
			images:   []string{"docker.io/rancher/harvester-upgrade-helper:v1.0.0"},
			expected: []string{"docker.io/rancher/harvester-upgrade-helper:v1.0.0"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, applyImageRetainList(tc.images))
		})
	}
}
