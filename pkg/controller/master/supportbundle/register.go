package supportbundle

import (
	"context"
	"net/http"
	"time"

	"github.com/harvester/harvester/pkg/config"
)

const (
	controllerAgentName = "supportbundle-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	sbs := management.HarvesterFactory.Harvesterhci().V1beta1().SupportBundle()
	nodeCache := management.CoreFactory.Core().V1().Node().Cache()
	podCache := management.CoreFactory.Core().V1().Pod().Cache()
	deployments := management.AppsFactory.Apps().V1().Deployment()
	daemonsets := management.AppsFactory.Apps().V1().DaemonSet()
	services := management.CoreFactory.Core().V1().Service()
	appCache := management.CatalogFactory.Catalog().V1().App().Cache()
	managedChartCache := management.RancherManagementFactory.Management().V3().ManagedChart().Cache()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()

	handler := &Handler{
		supportBundles:          sbs,
		supportBundleController: sbs,
		nodeCache:               nodeCache,
		podCache:                podCache,
		deployments:             deployments,
		daemonSets:              daemonsets,
		services:                services,
		appCache:                appCache,
		managedChartCache:       managedChartCache,
		settings:                settings,
		settingCache:            settings.Cache(),
		manager: &Manager{
			deployments: deployments,
			nodeCache:   nodeCache,
			podCache:    podCache,
			services:    services,
			httpClient: http.Client{
				Timeout: 30 * time.Second,
			},
		},
	}

	sbs.OnChange(ctx, controllerAgentName, handler.OnSupportBundleChanged)
	return nil
}
