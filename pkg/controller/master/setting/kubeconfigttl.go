package setting

import (
	"fmt"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvSettings "github.com/harvester/harvester/pkg/settings"
)

const (
	AuthTokenMaxTTLSettinName = "auth-token-max-ttl-minutes"
)

func (h *Handler) syncKubeconfigTTL(setting *harvesterv1.Setting) error {
	rancherKubeconfigTTLSetting, err := h.rancherSettingsCache.Get(harvSettings.KubeconfigDefaultTokenTTLMinutesSettingName)
	if err != nil {
		return fmt.Errorf("error fetching setting %s: %v", harvSettings.KubeconfigDefaultTokenTTLMinutesSettingName, err)
	}
	rancherAuthTokenMaxTTLSetting, err := h.rancherSettingsCache.Get(AuthTokenMaxTTLSettinName)
	if err != nil {
		return fmt.Errorf("error fetching setting %s: %v", AuthTokenMaxTTLSettinName, err)
	}

	// if a custom ttl is set in harvester
	if len(setting.Value) > 0 {
		rancherKubeconfigTTLSetting.Value = setting.Value
		rancherAuthTokenMaxTTLSetting.Value = setting.Value
	} else { // apply default setting
		rancherKubeconfigTTLSetting.Value = setting.Default
		rancherAuthTokenMaxTTLSetting.Value = setting.Default
	}

	if _, err := h.rancherSettings.Update(rancherKubeconfigTTLSetting); err != nil {
		return fmt.Errorf("unable to update rancher setting %s: %v", rancherKubeconfigTTLSetting.Name, err)
	}

	if _, err := h.rancherSettings.Update(rancherAuthTokenMaxTTLSetting); err != nil {
		return fmt.Errorf("unable to update rancher setting %s: %v", rancherAuthTokenMaxTTLSetting.Name, err)
	}
	return nil
}
