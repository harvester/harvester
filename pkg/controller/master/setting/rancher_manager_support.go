package setting

import (
	"reflect"
	"strconv"
	"strings"

	ranchersettings "github.com/rancher/rancher/pkg/settings"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

const (
	rancherName   = "Rancher"
	harvesterName = "Harvester"
)

// syncRancherManagerSupport updates ui-pl and ui-brand setting in rancher
// to match the value of rancher-manager-support setting
// if rancher-manager-support is true, ui-pl value is set to "Rancher" and ui-brand default is set to ""
// if rancher-manager-support is false, ui-pl value is set to "Harvester" and ui-brand default is set to "harvester"
func (h *Handler) syncRancherManagerSupport(setting *harvesterv1.Setting) error {
	value := setting.Value
	if value == "" {
		value = setting.Default
	}

	enable, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}

	err = h.updateUIPL(enable)
	if err != nil {
		return err
	}
	err = h.updateUIBrand(enable)
	if err != nil {
		return err
	}

	return err
}

func (h *Handler) updateUIPL(enable bool) error {
	uiPL, err := h.rancherSettingCache.Get(ranchersettings.UIPL.Name)
	if err != nil {
		return err
	}

	uiPLCopy := uiPL.DeepCopy()
	if enable {
		uiPLCopy.Value = rancherName
	} else {
		uiPLCopy.Value = harvesterName
	}

	if !reflect.DeepEqual(uiPL, uiPLCopy) {
		_, err = h.rancherSettings.Update(uiPLCopy)
	}

	return err
}

func (h *Handler) updateUIBrand(enable bool) error {
	uiBrand, err := h.rancherSettingCache.Get(ranchersettings.UIBrand.Name)
	if err != nil {
		return err
	}

	uiBrandCopy := uiBrand.DeepCopy()
	if enable {
		uiBrandCopy.Default = ""
	} else {
		uiBrandCopy.Default = strings.ToLower(harvesterName)
	}

	if !reflect.DeepEqual(uiBrand, uiBrandCopy) {
		_, err = h.rancherSettings.Update(uiBrandCopy)
	}

	return err
}
