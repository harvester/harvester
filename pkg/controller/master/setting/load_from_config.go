package setting

import (
	"github.com/sirupsen/logrus"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

type LoadingPreconfigHandler struct {
	namespace string
	settings  v1beta1.SettingClient
}

func (h *LoadingPreconfigHandler) settingOnChanged(_ string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil {
		return nil, nil
	}

	if value, ok := h.getPreconfigValue(setting); ok && !harvesterv1.SettingPreconfigLoaded.IsTrue(setting) {
		settingCpy := setting.DeepCopy()
		logrus.Infof("loading preconfig value for setting '%s'", setting.Name)
		settingCpy.Value = value
		harvesterv1.SettingPreconfigLoaded.SetStatusBool(settingCpy, true)

		return h.settings.Update(settingCpy)
	}

	return nil, nil
}

func (h *LoadingPreconfigHandler) getPreconfigValue(setting *harvesterv1.Setting) (string, bool) {
	value, ok := setting.Annotations[util.AnnotationSettingFromConfig]
	return value, ok
}
