package util

import (
	"fmt"
	"strings"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

// GetUpgradeImage retrieves the VirtualMachineImage based on the upgrade image name.
// The upgradeImage parameter is expected to be in the format "namespace/name".
func GetUpgradeImage(upgradeImage string, vmImageCache ctlharvesterv1.VirtualMachineImageCache) (*harvesterv1.VirtualMachineImage, error) {
	tokens := strings.Split(upgradeImage, "/")
	if len(tokens) != 2 {
		return nil, fmt.Errorf("invalid image format name: %s", upgradeImage)
	}

	image, err := vmImageCache.Get(tokens[0], tokens[1])
	if err != nil {
		return nil, err
	}
	return image, nil
}

// GetHarvesterUpgradeImage returns the harvester-upgrade image of a version. Prime releases publish it under the
// Harvester registry's "harvester" repository, and an empty registry means the community image.
func GetHarvesterUpgradeImage(registry, version string) string {
	if registry != "" {
		return fmt.Sprintf("%s/harvester/harvester-upgrade:%s", registry, version)
	}
	return fmt.Sprintf("%s:%s", HarvesterUpgradeImageRepository, version)
}

// GetPreviousUpgradeImage returns the harvester-upgrade image of the version an upgrade starts from, which the
// upgrade records when it starts. Upgrades started by older versions don't record it and use the community image.
func GetPreviousUpgradeImage(upgrade *harvesterv1.Upgrade) string {
	if image := upgrade.Annotations[AnnotationPreviousUpgradeImage]; image != "" {
		return image
	}
	return GetHarvesterUpgradeImage("", upgrade.Status.PreviousVersion)
}

// IsUpgradeInProgress reports whether the upgrade is active.
// An upgrade is considered in progress when UpgradeCompleted is neither True nor False.
func IsUpgradeInProgress(upgrade *harvesterv1.Upgrade) bool {
	if upgrade == nil {
		return false
	}
	return !harvesterv1.UpgradeCompleted.IsTrue(upgrade) && !harvesterv1.UpgradeCompleted.IsFalse(upgrade)
}
