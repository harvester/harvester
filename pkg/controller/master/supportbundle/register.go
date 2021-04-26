package supportbundle

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	controllerAgentName = "supportbundle-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	sbs := management.HarvesterFactory.Harvesterhci().V1beta1().SupportBundle()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	nodes := management.CoreFactory.Core().V1().Node()
	nodesCache := nodes.Cache()
	deployments := management.AppsFactory.Apps().V1().Deployment()
	daemonsets := management.AppsFactory.Apps().V1().DaemonSet()
	services := management.CoreFactory.Core().V1().Service()

	controller := &Handler{
		supportBundles: sbs,
		settingCache:   settings.Cache(),
		nodeCache:      nodesCache,
		deployments:    deployments,
		daemonSets:     daemonsets,
		services:       services,
		manager: &Manager{
			deployments: deployments,
			nodeCache:   nodesCache,
			services:    services,
		},
	}

	sbs.OnChange(ctx, controllerAgentName, controller.OnSupportBundleChanged)
	return nil
}
