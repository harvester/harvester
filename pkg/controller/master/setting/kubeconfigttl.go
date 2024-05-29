package setting

import (
	"fmt"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvSettings "github.com/harvester/harvester/pkg/settings"
)

func (h *Handler) syncKubeconfigTTL(setting *harvesterv1.Setting) error {
	rancherKubeconfigTTLSetting, err := h.rancherSettingsCache.Get(harvSettings.KubeconfigTTLSettingName)
	if err != nil {
		return fmt.Errorf("error fetching setting %s: %v", harvSettings.KubeconfigTTLSettingName, err)
	}

	// if a custom ttl is set in harvester
	if len(setting.Value) > 0 {
		rancherKubeconfigTTLSetting.Value = setting.Value
	} else { // apply default setting
		rancherKubeconfigTTLSetting.Value = setting.Default
	}
	_, err = h.rancherSettings.Update(rancherKubeconfigTTLSetting)
	return err
}
