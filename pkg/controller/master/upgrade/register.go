package upgrade

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	upgradeControllerName = "harvester-upgrade-controller"
	planControllerName    = "harvester-plan-controller"
	jobControllerName     = "harvester-upgrade-job-controller"
	podControllerName     = "harvester-upgrade-pod-controller"
	settingControllerName = "harvester-version-setting-controller"
	vmImageControllerName = "harvester-upgrade-vm-image-controller"
	machineControllerName = "harvester-upgrade-machine-controller"
	nodeControllerName    = "harvester-upgrade-node-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	if !options.HCIMode {
		return nil
	}

	upgrades := management.HarvesterFactory.Harvesterhci().V1beta1().Upgrade()
	versions := management.HarvesterFactory.Harvesterhci().V1beta1().Version()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	plans := management.UpgradeFactory.Upgrade().V1().Plan()
	nodes := management.CoreFactory.Core().V1().Node()
	jobs := management.BatchFactory.Batch().V1().Job()
	pods := management.CoreFactory.Core().V1().Pod()
	vmImages := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	services := management.CoreFactory.Core().V1().Service()
	clusters := management.ProvisioningFactory.Provisioning().V1().Cluster()
	machines := management.ClusterFactory.Cluster().V1alpha4().Machine()

	controller := &upgradeHandler{
		ctx:           ctx,
		jobClient:     jobs,
		jobCache:      jobs.Cache(),
		nodeCache:     nodes.Cache(),
		namespace:     options.Namespace,
		upgradeClient: upgrades,
		upgradeCache:  upgrades.Cache(),
		versionCache:  versions.Cache(),
		planClient:    plans,
		planCache:     plans.Cache(),
		vmImageClient: vmImages,
		vmImageCache:  vmImages.Cache(),
		vmClient:      vms,
		serviceClient: services,
		clusterClient: clusters,
		clusterCache:  clusters.Cache(),
	}
	upgrades.OnChange(ctx, upgradeControllerName, controller.OnChanged)
	upgrades.OnRemove(ctx, upgradeControllerName, controller.OnRemove)

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
		machineClient: machines,
		machineCache:  machines.Cache(),
		nodeClient:    nodes,
		nodeCache:     nodes.Cache(),
	}
	jobs.OnChange(ctx, jobControllerName, jobHandler.OnChanged)

	podHandler := &podHandler{
		namespace:     options.Namespace,
		planCache:     plans.Cache(),
		upgradeClient: upgrades,
		upgradeCache:  upgrades.Cache(),
	}
	pods.OnChange(ctx, podControllerName, podHandler.OnChanged)

	vmImageHandler := &vmImageHandler{
		namespace:     options.Namespace,
		upgradeClient: upgrades,
		upgradeCache:  upgrades.Cache(),
	}
	vmImages.OnChange(ctx, vmImageControllerName, vmImageHandler.OnChanged)

	machineHandler := &machineHandler{
		namespace:     options.Namespace,
		upgradeClient: upgrades,
		upgradeCache:  upgrades.Cache(),
		jobClient:     jobs,
		jobCache:      jobs.Cache(),
	}
	machines.OnChange(ctx, machineControllerName, machineHandler.OnChanged)

	nodeHandler := &nodeHandler{
		namespace:     options.Namespace,
		nodeClient:    nodes,
		nodeCache:     nodes.Cache(),
		upgradeClient: upgrades,
		upgradeCache:  upgrades.Cache(),
		machineClient: machines,
		machineCache:  machines.Cache(),
	}
	nodes.OnChange(ctx, nodeControllerName, nodeHandler.OnChanged)

	versionSyncer := newVersionSyncer(ctx, options.Namespace, versions, versions.Cache())

	settingHandler := settingHandler{
		versionSyncer: versionSyncer,
	}
	settings.OnChange(ctx, settingControllerName, settingHandler.OnChanged)

	go versionSyncer.start()

	return nil
}
