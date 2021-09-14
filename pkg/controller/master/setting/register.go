package setting

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	controllerName = "harvester-setting-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	controller := &Handler{}

	settings.OnChange(ctx, controllerName, controller.LogLevelOnChanged)
	return nil
}
