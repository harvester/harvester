package uiinfo

import (
	"net/http"
	"os"

	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

type Handler struct {
	settingsCache ctlharvesterv1.SettingCache
}

func NewUIInfoHandler(scaled *config.Scaled, _ config.Options) *Handler {
	return &Handler{
		settingsCache: scaled.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache(),
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, _ *http.Request) {
	uiSource := settings.UISource.Get()
	if uiSource == "auto" {
		if !settings.IsRelease() {
			uiSource = "external"
		} else {
			uiSource = "bundled"
		}
	}
	util.ResponseOKWithBody(rw, map[string]string{
		settings.UISourceSettingName:               uiSource,
		settings.UIIndexSettingName:                settings.UIIndex.Get(),
		settings.UIPluginIndexSettingName:          settings.UIPluginIndex.Get(),
		settings.UIPluginBundledVersionSettingName: os.Getenv(settings.GetEnvKey(settings.UIPluginBundledVersionSettingName)),
	})
}
