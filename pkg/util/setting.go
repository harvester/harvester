package util

import (
	"fmt"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

// get the value of AdditionalGuestMemoryOverheadRatio, if failed, return the default ratio
func GetAdditionalGuestMemoryOverheadRatioWithoutError(settingCache ctlharvesterv1.SettingCache) *string {
	value := settings.AdditionalGuestMemoryOverheadRatioDefault
	if settingCache == nil {
		return &value
	}
	s, err := settingCache.Get(settings.AdditionalGuestMemoryOverheadRatioName)
	if err != nil || s == nil {
		return &value
	}
	value = s.Value
	if value == "" {
		value = s.Default
		if value == "" {
			value = settings.AdditionalGuestMemoryOverheadRatioDefault
			return &value
		}
	}
	agmorc, err := settings.NewAdditionalGuestMemoryOverheadRatioConfig(value)
	if err != nil {
		value = settings.AdditionalGuestMemoryOverheadRatioDefault
		return &value
	}
	if agmorc.IsEmpty() {
		return nil
	}
	value = agmorc.Value()
	return &value
}

func GetAdditionalGuestMemoryOverheadRatio(settingCache ctlharvesterv1.SettingCache) (*string, error) {
	value := ""
	if settingCache == nil {
		return nil, fmt.Errorf("the settingCache is empty, can't get the setting")
	}
	s, err := settingCache.Get(settings.AdditionalGuestMemoryOverheadRatioName)
	if err != nil {
		return nil, err
	}
	value = s.Value
	if value == "" {
		value = s.Default
	}
	agmorc, err := settings.NewAdditionalGuestMemoryOverheadRatioConfig(value)
	if err != nil {
		return nil, err
	}
	if agmorc.IsEmpty() {
		return nil, nil
	}
	value = agmorc.Value()
	return &value, nil
}

func IsRestoreVM(settingCache ctlharvesterv1.SettingCache) (bool, error) {
	value := ""
	if settingCache == nil {
		return false, fmt.Errorf("the settingCache is empty, can't get the setting")
	}
	s, err := settingCache.Get(settings.UpgradeConfigSettingName)
	if err != nil {
		return false, err
	}
	value = s.Value
	if value == "" {
		value = s.Default
	}

	upgradeConfig, err := settings.DecodeConfig[settings.UpgradeConfig](value)
	if err != nil || upgradeConfig == nil {
		return false, err
	}
	return upgradeConfig.RestoreVM, nil
}
