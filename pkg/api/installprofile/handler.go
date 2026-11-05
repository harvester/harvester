package installprofile

import (
	"github.com/harvester/harvester/pkg/config"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
	"github.com/harvester/harvester/pkg/settings"
)

type Handler struct{}

func NewHandler(_ *config.Scaled, _ config.Options) *Handler {
	return &Handler{}
}

func (h *Handler) Do(_ *harvesterServer.Ctx) (harvesterServer.ResponseBody, error) {
	skip := settings.SkipLonghornDefaultDisk.Get() == "true"
	return map[string]interface{}{
		"skipLonghornDefaultDisk": skip,
	}, nil
}
