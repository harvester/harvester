package setting

import (
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func (h *Handler) syncSupportBundleImage(setting *harvesterv1.Setting) error {
	return h.syncImageFromHelmValues(setting, "support-bundle-kit")
}
