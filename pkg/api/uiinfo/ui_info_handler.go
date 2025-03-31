package uiinfo

import (
	"os"

	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
	"github.com/harvester/harvester/pkg/settings"
)

type Handler struct {
	settingsCache ctlharvesterv1.SettingCache
}

func NewUIInfoHandler(scaled *config.Scaled, _ config.Options) *Handler {
	return &Handler{
		settingsCache: scaled.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache(),
	}
}

func (h *Handler) Do(_ *harvesterServer.Ctx) (harvesterServer.ResponseBody, error) {
	uiSource := settings.UISource.Get()
	if uiSource == "auto" {
		if !settings.IsRelease() {
			uiSource = "external"
		} else {
			uiSource = "bundled"
		}
	}

	return map[string]string{
		settings.UISourceSettingName:               uiSource,
		settings.UIIndexSettingName:                settings.UIIndex.Get(),
		settings.UIPluginIndexSettingName:          settings.UIPluginIndex.Get(),
		settings.UIPluginBundledVersionSettingName: os.Getenv(settings.GetEnvKey(settings.UIPluginBundledVersionSettingName)),
	}, nil
}
