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
// if rancher-manager-support is false, ui-brand value is set to "harvester"
// if rancher-manager-support is true, ui-brand value is set to ""
// the default value of ui-pl in Harvester is patched to "Harvester" in pkg/controller/master/rancher/embedded_rancher.go
// If the user does not have ui-pl set and rancher-manager-support is enabled, change ui-pl from the default Harvester to Rancher
// If the user does not have ui-pl set and rancher-manager-support is disabled, change ui-pl from the Rancher to default Harvester
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
		if uiPLCopy.Default == harvesterName && uiPLCopy.Value == "" {
			// ui show "Rancher" label
			uiPLCopy.Value = rancherName
		}
	} else {
		if uiPLCopy.Value == rancherName {
			// ui show "Harvester" label
			uiPLCopy.Value = ""
		}
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
		// ui show Rancher logo
		uiBrandCopy.Value = ""
	} else {
		// ui show Harvester logo
		uiBrandCopy.Value = strings.ToLower(harvesterName)
	}

	if !reflect.DeepEqual(uiBrand, uiBrandCopy) {
		_, err = h.rancherSettings.Update(uiBrandCopy)
	}

	return err
}
