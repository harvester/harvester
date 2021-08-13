package rancher

import (
	"context"

	rancherv3 "github.com/rancher/rancher/pkg/generated/controllers/management.cattle.io/v3"

	"github.com/harvester/harvester/pkg/config"
)

const controllerRancherName = "harvester-rancher-controller"

type Handler struct {
	RancherSettings rancherv3.SettingClient
}

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	if options.RancherEmbedded {
		rancherSettings := management.RancherManagementFactory.Management().V3().Setting()
		h := Handler{
			RancherSettings: rancherSettings,
		}

		rancherSettings.OnChange(ctx, controllerRancherName, h.RancherSettingOnChange)
	}
	return nil
}
