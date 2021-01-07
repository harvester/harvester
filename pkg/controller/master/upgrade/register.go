package upgrade

import (
	"context"

	"github.com/rancher/harvester/pkg/config"
)

const (
	upgradeControllerName = "harvester-upgrade-controller"
	planControllerName    = "harvester-plan-controller"
	jobControllerName     = "harvester-upgrade-job-controller"
	podControllerName     = "harvester-upgrade-pod-controller"
	settingControllerName = "harvester-version-setting-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	if !options.HCIMode {
		return nil
	}

	upgrades := management.HarvesterFactory.Harvester().V1alpha1().Upgrade()
	settings := management.HarvesterFactory.Harvester().V1alpha1().Setting()
	plans := management.UpgradeFactory.Upgrade().V1().Plan()
	nodes := management.CoreFactory.Core().V1().Node()
	jobs := management.BatchFactory.Batch().V1().Job()
	pods := management.CoreFactory.Core().V1().Pod()
	controller := &upgradeHandler{
		jobClient:     jobs,
		nodeCache:     nodes.Cache(),
		namespace:     options.Namespace,
		upgradeClient: upgrades,
		upgradeCache:  upgrades.Cache(),
		planClient:    plans,
	}
	upgrades.OnChange(ctx, upgradeControllerName, controller.OnChanged)

	planHandler := &planHandler{
		namespace:     options.Namespace,
		upgradeClient: upgrades,
		upgradeCache:  upgrades.Cache(),
		nodeCache:     nodes.Cache(),
		planClient:    plans,
	}
	plans.OnChange(ctx, planControllerName, planHandler.OnChanged)

	jobHandler := &jobHandler{
		namespace:     options.Namespace,
		planCache:     plans.Cache(),
		upgradeClient: upgrades,
		upgradeCache:  upgrades.Cache(),
	}
	jobs.OnChange(ctx, jobControllerName, jobHandler.OnChanged)

	podHandler := &podHandler{
		namespace:     options.Namespace,
		planCache:     plans.Cache(),
		upgradeClient: upgrades,
		upgradeCache:  upgrades.Cache(),
	}
	pods.OnChange(ctx, podControllerName, podHandler.OnChanged)
	versionSyncer := newVersionSyncer(ctx)

	settingHandler := settingHandler{
		versionSyncer: versionSyncer,
	}
	settings.OnChange(ctx, settingControllerName, settingHandler.OnChanged)

	go versionSyncer.start()

	return nil
}
