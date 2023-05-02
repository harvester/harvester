package setting

import (
	"reflect"

	ranchersettings "github.com/rancher/rancher/pkg/settings"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

func (h *Handler) syncHarvesterToRancher(setting *harvesterv1.Setting) error {
	switch setting.Name {
	case settings.UIIndexSettingName:
		return h.copyHarvesterToRancher(setting, ranchersettings.UIIndex.Name)
	case settings.UISourceSettingName:
		return h.copyUISourceToRancher(setting)
	}
	return nil
}

func (h *Handler) copyHarvesterToRancher(setting *harvesterv1.Setting, rancherSettingName string) error {
	rancherSetting, err := h.rancherSettingCache.Get(rancherSettingName)
	if err != nil {
		return err
	}

	rancherSettingCopy := rancherSetting.DeepCopy()
	rancherSettingCopy.Value = setting.Value
	if !reflect.DeepEqual(rancherSetting, rancherSettingCopy) {
		_, err = h.rancherSettings.Update(rancherSettingCopy)
	}
	return err
}

func (h *Handler) copyUISourceToRancher(setting *harvesterv1.Setting) error {
	rancherSetting, err := h.rancherSettingCache.Get(ranchersettings.UIOfflinePreferred.Name)
	if err != nil {
		return err
	}

	rancherSettingCopy := rancherSetting.DeepCopy()
	switch setting.Value {
	case "external":
		rancherSettingCopy.Value = "false"
	case "bundled":
		rancherSettingCopy.Value = "true"
	case "auto":
		rancherSettingCopy.Value = "dynamic"
	case "":
		rancherSettingCopy.Value = ""
	}

	if !reflect.DeepEqual(rancherSetting, rancherSettingCopy) {
		_, err = h.rancherSettings.Update(rancherSettingCopy)
	}
	return err
}
