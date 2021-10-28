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

	var dsClient = management.AppsFactory.Apps().V1().DaemonSet()
	var dsCache = dsClient.Cache()
	var cmClient = management.CoreFactory.Core().V1().ConfigMap()
	var cmCache = cmClient.Cache()
	controller := &Handler{
		dsClient:          dsClient,
		dsCache:           dsCache,
		settingController: settings,
		settingCache:      settings.Cache(),
		cmClient:          cmClient,
		cmCache:           cmCache,
	}

	settings.OnChange(ctx, controllerName, controller.LogLevelOnChanged)
	settings.OnChange(ctx, controllerName, controller.AutoAddDiskPathsOnChanged)
	settings.OnChange(ctx, controllerName, controller.LoadSettingsFromInstallerOnChanged)
	return nil
}
