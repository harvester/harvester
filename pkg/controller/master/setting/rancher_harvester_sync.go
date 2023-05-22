package setting

import (
	"reflect"

	mgmtv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	ranchersettings "github.com/rancher/rancher/pkg/settings"

	"github.com/harvester/harvester/pkg/settings"
)

func (h *Handler) rancherSettingOnChange(_ string, rancherSetting *mgmtv3.Setting) (*mgmtv3.Setting, error) {
	if rancherSetting == nil || rancherSetting.DeletionTimestamp != nil {
		return rancherSetting, nil
	}

	var err error
	switch rancherSetting.Name {
	case ranchersettings.UIDashboardIndex.Name:
		err = h.copyRancherToHarvester(rancherSetting, settings.UIIndexSettingName)
	case ranchersettings.UIOfflinePreferred.Name:
		err = h.copyUIOfflinePreferredToHarvester(rancherSetting)
	}
	return rancherSetting, err
}

func (h *Handler) copyRancherToHarvester(rancherSetting *mgmtv3.Setting, harvesterSettingName string) error {
	harvesterSetting, err := h.settingCache.Get(harvesterSettingName)
	if err != nil {
		return err
	}

	harvesterSettingCopy := harvesterSetting.DeepCopy()
	harvesterSettingCopy.Value = rancherSetting.Value
	if !reflect.DeepEqual(harvesterSetting, harvesterSettingCopy) {
		_, err = h.settings.Update(harvesterSettingCopy)
	}
	return err
}

func (h *Handler) copyUIOfflinePreferredToHarvester(rancherSetting *mgmtv3.Setting) error {
	harvesterSetting, err := h.settingCache.Get(settings.UISourceSettingName)
	if err != nil {
		return err
	}

	harvesterSettingCopy := harvesterSetting.DeepCopy()
	switch rancherSetting.Value {
	case "dynamic":
		harvesterSettingCopy.Value = "auto"
	case "false":
		harvesterSettingCopy.Value = "external"
	case "true":
		harvesterSettingCopy.Value = "bundled"
	case "":
		harvesterSettingCopy.Value = ""
	}

	if !reflect.DeepEqual(harvesterSetting, harvesterSettingCopy) {
		_, err = h.settings.Update(harvesterSettingCopy)
	}
	return err
}
