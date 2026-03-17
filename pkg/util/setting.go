package util

import (
	"fmt"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		value = settings.AdditionalGuestMemoryOverheadRatioDefault
		return &value
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

func IsShareStorageNetwork(settingCache ctlharvesterv1.SettingCache) (bool, error) {
	if settingCache == nil {
		return false, fmt.Errorf("the settingCache is empty, can't get the setting")
	}
	rwxSN, err := settingCache.Get(settings.RWXNetworkSettingName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get %s setting, err: %v", settings.RWXNetworkSettingName, err)
	}

	rwxConfig, err := settings.DecodeConfig[settings.RWXNetworkConfig](rwxSN.EffectiveValue())
	if err != nil || rwxConfig == nil {
		return false, err
	}

	return rwxConfig.ShareStorageNetwork, nil
}
