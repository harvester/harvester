package addon

import (
	"fmt"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

// the return string shows a meanful message when the second bool param is true
// the return bool indicates where the addon is still under processing
func IsAddonOnProcessing(addon *harvesterv1.Addon) (string, bool) {
	if addon == nil {
		return "", false
	}
	// when it is disabled
	if !addon.Spec.Enabled {
		// disabled, the AddonInitState means the addon is created and never enabled/disabled
		if addon.Status.Status == harvesterv1.AddonDisabled || addon.Status.Status == harvesterv1.AddonInitState {
			return "", false
		}
		// still on processing
		return fmt.Sprintf("addon %s/%s is disabled but still on status %v", addon.Namespace, addon.Name, addon.Status.Status), true
	}

	// when it is  enabled
	if addon.Status.Status == harvesterv1.AddonDeployed {
		return "", false
	}
	return fmt.Sprintf("addon %s/%s is enabled but still on status %v", addon.Namespace, addon.Name, addon.Status.Status), true
}
