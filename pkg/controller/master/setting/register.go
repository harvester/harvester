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
	clusters := management.ProvisioningFactory.Provisioning().V1().Cluster()
	deployments := management.AppsFactory.Apps().V1().Deployment()
	configmaps := management.CoreFactory.Core().V1().ConfigMap()
	lhs := management.LonghornFactory.Longhorn().V1beta1().Setting()
	apps := management.CatalogFactory.Catalog().V1().App()
	managedCharts := management.RancherManagementFactory.Management().V3().ManagedChart()
	ingresses := management.NetworkingFactory.Networking().V1().Ingress()
	helmChartConfigs := management.HelmFactory.Helm().V1().HelmChartConfig()
	controller := &Handler{
		namespace:            options.Namespace,
		apply:                management.Apply,
		settings:             settings,
		settingCache:         settings.Cache(),
		secrets:              secrets,
		secretCache:          secrets.Cache(),
		clusters:             clusters,
		clusterCache:         clusters.Cache(),
		deployments:          deployments,
		deploymentCache:      deployments.Cache(),
		ingresses:            ingresses,
		ingressCache:         ingresses.Cache(),
		longhornSettings:     lhs,
		longhornSettingCache: lhs.Cache(),
		configmaps:           configmaps,
		configmapCache:       configmaps.Cache(),
		apps:                 apps,
		managedCharts:        managedCharts,
		managedChartCache:    managedCharts.Cache(),
		helmChartConfigs:     helmChartConfigs,
		helmChartConfigCache: helmChartConfigs.Cache(),
		httpClient: http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}

	syncers = map[string]syncerFunc{
		"additional-ca":             controller.syncAdditionalTrustedCAs,
		"cluster-registration-url":  controller.registerCluster,
		"http-proxy":                controller.syncHTTPProxy,
		"log-level":                 controller.setLogLevel,
		"overcommit-config":         controller.syncOvercommitConfig,
		"vip-pools":                 controller.syncVipPoolsConfig,
		"auto-disk-provision-paths": controller.syncNDMAutoProvisionPaths,
		"ssl-certificates":          controller.syncSSLCertificate,
		"ssl-parameters":            controller.syncSSLParameters,
		"containerd-registry":       controller.syncContainerdRegistry,
		// for "backup-target" syncer, please check harvester-backup-target-controller
		// for "storage-network" syncer, please check harvester-storage-network-controller
	}

	settings.OnChange(ctx, controllerName, controller.settingOnChanged)
	apps.OnChange(ctx, controllerName, controller.appOnChanged)
	return nil
}
