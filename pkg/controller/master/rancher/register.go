package rancher

import (
	"context"

	"github.com/rancher/harvester/pkg/config"
)

const (
	controllerRancherName = "harvester-rancher-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	if options.RancherEmbedded {
		secrets := management.CoreFactory.Core().V1().Secret()
		rancherSettings := management.RancherManagementFactory.Management().V3().Setting()
		settings := management.HarvesterFactory.Harvester().V1alpha1().Setting()
		h := Handler{
			Secrets:         secrets,
			SecretCache:     secrets.Cache(),
			RancherSettings: rancherSettings,
			Settings:        settings,
		}

		// add Rancher private CA
		go h.registerPrivateCA(ctx)

		rancherSettings.OnChange(ctx, controllerRancherName, h.SettingOnChange)
		settings.OnChange(ctx, controllerRancherName, h.ServerURLOnChange)
	}
	return nil
}
