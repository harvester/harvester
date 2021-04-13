package setting

import (
	"context"

	"github.com/rancher/harvester/pkg/config"
)

const (
	controllerName = "harvester-setting-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	secrets := management.CoreFactory.Core().V1().Secret()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	controller := &Handler{
		SecretCache:  secrets.Cache(),
		SecretClient: secrets,
	}

	settings.OnChange(ctx, controllerName, controller.ServerURLOnChanged)
	settings.OnChange(ctx, controllerName, controller.LogLevelOnChanged)
	return nil
}
