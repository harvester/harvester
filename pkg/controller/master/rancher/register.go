package rancher

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	controllerRancherName = "harvester-rancher-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	if options.RancherEmbedded {
		secrets := management.CoreFactory.Core().V1().Secret()
		rancherSettings := management.RancherManagementFactory.Management().V3().Setting()
		settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
		nodeDrivers := management.RancherManagementFactory.Management().V3().NodeDriver()
		h := Handler{
			Secrets:         secrets,
			SecretCache:     secrets.Cache(),
			RancherSettings: rancherSettings,
			Settings:        settings,
			NodeDrivers:     nodeDrivers,
		}

		// add Rancher private CA
		go h.registerPrivateCA(ctx)

		if _, err := h.AddHarvesterNodeDriver(); err != nil {
			return err
		}

		rancherSettings.OnChange(ctx, controllerRancherName, h.SettingOnChange)
	}
	return nil
}
