package setting

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

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
		apply:                management.Apply,
		settings:             settings,
		secrets:              secrets,
		secretCache:          secrets.Cache(),
		deployments:          deployments,
		deploymentCache:      deployments.Cache(),
		longhornSettings:     lhs,
		longhornSettingCache: lhs.Cache(),
		httpClient: http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}

	syncers = map[string]syncerFunc{
		"additional-ca":            controller.syncAdditionalTrustedCAs,
		"cluster-registration-url": controller.registerCluster,
		"http-proxy":               controller.syncHTTPProxy,
		"log-level":                controller.setLogLevel,
		"overcommit-config":        controller.syncOvercommitConfig,
	}

	settings.OnChange(ctx, controllerName, controller.settingOnChanged)
	return nil
}
