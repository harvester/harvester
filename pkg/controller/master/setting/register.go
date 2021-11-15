package setting

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/harvester/harvester/pkg/config"
)

const (
	settingControllerName   = "harvester-setting-controller"
	preconfigControllerName = "harvester-setting-preconfig-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	secrets := management.CoreFactory.Core().V1().Secret()
	clusters := management.ProvisioningFactory.Provisioning().V1().Cluster()
	deployments := management.AppsFactory.Apps().V1().Deployment()
	configmaps := management.CoreFactory.Core().V1().ConfigMap()
	lhs := management.LonghornFactory.Longhorn().V1beta1().Setting()
	controller := &Handler{
		namespace:            options.Namespace,
		apply:                management.Apply,
		settings:             settings,
		secrets:              secrets,
		secretCache:          secrets.Cache(),
		clusters:             clusters,
		clusterCache:         clusters.Cache(),
		deployments:          deployments,
		deploymentCache:      deployments.Cache(),
		longhornSettings:     lhs,
		longhornSettingCache: lhs.Cache(),
		configmaps:           configmaps,
		configmapCache:       configmaps.Cache(),
		httpClient: http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}

	preconfigController := &LoadingPreconfigHandler{
		namespace: options.Namespace,
		settings:  settings,
	}

	syncers = map[string]syncerFunc{
		"additional-ca":            controller.syncAdditionalTrustedCAs,
		"cluster-registration-url": controller.registerCluster,
		"http-proxy":               controller.syncHTTPProxy,
		"log-level":                controller.setLogLevel,
		"overcommit-config":        controller.syncOvercommitConfig,
		"vip-pools":                controller.syncVipPoolsConfig,
	}

	settings.OnChange(ctx, settingControllerName, controller.settingOnChanged)
	settings.OnChange(ctx, preconfigControllerName, preconfigController.settingOnChanged)
	return nil
}
