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

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	sbs := management.HarvesterFactory.Harvesterhci().V1beta1().SupportBundle()
	nodeCache := management.CoreFactory.Core().V1().Node().Cache()
	podCache := management.CoreFactory.Core().V1().Pod().Cache()
	deployments := management.AppsFactory.Apps().V1().Deployment()
	daemonsets := management.AppsFactory.Apps().V1().DaemonSet()
	services := management.CoreFactory.Core().V1().Service()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()

	handler := &Handler{
		supportBundles:          sbs,
		supportBundleController: sbs,
		nodeCache:               nodeCache,
		podCache:                podCache,
		deployments:             deployments,
		daemonSets:              daemonsets,
		services:                services,
		settings:                settings,
		settingCache:            settings.Cache(),
		clientset:               management.ClientSet,
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
