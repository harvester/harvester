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
	secrets := management.CoreFactory.Core().V1().Secret()
	deployments := management.AppsFactory.Apps().V1().Deployment()
	lhs := management.LonghornFactory.Longhorn().V1beta1().Setting()
	controller := &Handler{
		namespace:            options.Namespace,
		settings:             settings,
		secrets:              secrets,
		secretCache:          secrets.Cache(),
		deployments:          deployments,
		deploymentCache:      deployments.Cache(),
		longhornSettings:     lhs,
		longhornSettingCache: lhs.Cache(),
	}

	syncers = map[string]syncerFunc{
		"http-proxy":        controller.syncHTTPProxy,
		"log-level":         controller.setLogLevel,
		"overcommit-config": controller.syncOvercommitConfig,
	}

	settings.OnChange(ctx, controllerName, controller.settingOnChanged)
	return nil
}
