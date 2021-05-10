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
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache()
	nodeCache := management.CoreFactory.Core().V1().Node().Cache()
	podCache := management.CoreFactory.Core().V1().Pod().Cache()
	deployments := management.AppsFactory.Apps().V1().Deployment()
	daemonsets := management.AppsFactory.Apps().V1().DaemonSet()
	services := management.CoreFactory.Core().V1().Service()

	handler := &Handler{
		supportBundles:          sbs,
		supportBundleController: sbs,
		settingCache:            settings,
		nodeCache:               nodeCache,
		podCache:                podCache,
		deployments:             deployments,
		daemonSets:              daemonsets,
		services:                services,
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
