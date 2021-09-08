package rancher

import (
	rancherv3api "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
)

var UpdateRancherUISettings = map[string]string{
	"ui-dashboard-index":   "https://releases.rancher.com/harvester-ui/dashboard/v0.3.0-preview1/index.html",
	"ui-offline-preferred": "false",
	"ui-pl":                "Harvester",
}

func (h *Handler) RancherSettingOnChange(key string, setting *rancherv3api.Setting) (*rancherv3api.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil {
		return nil, nil
	}

	for name, value := range UpdateRancherUISettings {
		if setting.Name == name && setting.Default != value {
			logrus.Debugf("Updating rancher dashboard setting %s, %s => %s", name, setting.Default, value)
			settCopy := setting.DeepCopy()
			settCopy.Default = value
			if _, err := h.RancherSettings.Update(settCopy); err != nil {
				return setting, err
			}
		}
	}
	return nil, nil
}
