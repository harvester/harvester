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
