package upgrade

import (
	"context"
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/relatedresource"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	capiv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/scheme"
	"github.com/harvester/harvester/pkg/util"
)

const (
	upgradeControllerName    = "harvester-upgrade-controller"
	planControllerName       = "harvester-plan-controller"
	jobControllerName        = "harvester-upgrade-job-controller"
	deploymentControllerName = "harvester-upgrade-deployment-controller"
	settingControllerName    = "harvester-version-setting-controller"
	vmImageControllerName    = "harvester-upgrade-vm-image-controller"
	secretControllerName     = "harvester-upgrade-secret-controller"
	nodeControllerName       = "harvester-upgrade-node-controller"
)

type handler struct {
	nodeCache ctlcorev1.NodeCache
}

func (h *handler) NotifyUnpausedMachinePlanSecret(_ string, _ string, obj runtime.Object) ([]relatedresource.Key, error) {
	upgrade, ok := obj.(*harvesterv1.Upgrade)
	if !ok {
		return nil, nil
	}

	if upgrade.DeletionTimestamp != nil || upgrade.Labels[harvesterLatestUpgradeLabel] != "true" {
		return nil, nil
	}

	pauseMap, err := getNodeUpgradePauseMap(upgrade)
	if err != nil {
		return nil, err
	}

	if pauseMap == nil {
		return nil, nil
	}

	machinePlanSecretKeys := make([]relatedresource.Key, len(pauseMap))
	for nodeName, state := range pauseMap {
		if state != util.NodeUnpause {
			continue
		}

		// Skip the already-unpaused nodes
		nodeUpgradeStatus, ok := upgrade.Status.NodeStatuses[nodeName]
		if !ok {
			continue
		}
		if nodeUpgradeStatus.State != nodeStateUpgradePaused {
			continue
		}

		node, err := h.nodeCache.Get(nodeName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		machineName, ok := node.Annotations[capiv1.MachineAnnotation]
		if !ok {
			return nil, fmt.Errorf("machine name not found on node %s", node.Name)
		}
		key := relatedresource.Key{
			Namespace: util.FleetLocalNamespaceName,
			Name:      fmt.Sprintf("%s-machine-plan", machineName),
		}
		machinePlanSecretKeys = append(machinePlanSecretKeys, key)
	}

	return machinePlanSecretKeys, nil
}

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	if !options.HCIMode {
		return nil
	}

	upgrades := management.HarvesterFactory.Harvesterhci().V1beta1().Upgrade()
	upgradeLogs := management.HarvesterFactory.Harvesterhci().V1beta1().UpgradeLog()
	versions := management.HarvesterFactory.Harvesterhci().V1beta1().Version()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	plans := management.UpgradeFactory.Upgrade().V1().Plan()
	managedcharts := management.RancherManagementFactory.Management().V3().ManagedChart()
	nodes := management.CoreFactory.Core().V1().Node()
	jobs := management.BatchFactory.Batch().V1().Job()
	deployments := management.AppsFactory.Apps().V1().Deployment()
	vmImages := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage()
	vms := management.VirtFactory.Kubevirt().V1().VirtualMachine()
	services := management.CoreFactory.Core().V1().Service()
	namespaces := management.CoreFactory.Core().V1().Namespace()
	clusters := management.ProvisioningFactory.Provisioning().V1().Cluster()
	machines := management.ClusterFactory.Cluster().V1beta1().Machine()
	secrets := management.CoreFactory.Core().V1().Secret()
	pvcs := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	lhSettings := management.LonghornFactory.Longhorn().V1beta2().Setting()
	configMaps := management.CoreFactory.Core().V1().ConfigMap()
	addons := management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
	storageClasses := management.StorageFactory.Storage().V1().StorageClass()

	virtSubsrcConfig := rest.CopyConfig(management.RestConfig)
	virtSubsrcConfig.GroupVersion = &schema.GroupVersion{Group: "subresources.kubevirt.io", Version: "v1"}
	virtSubsrcConfig.APIPath = "/apis"
	virtSubsrcConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	virtSubresourceClient, err := rest.RESTClientFor(virtSubsrcConfig)
	if err != nil {
		return err
	}

	controller := &upgradeHandler{
		ctx:                ctx,
		jobClient:          jobs,
		jobCache:           jobs.Cache(),
		nodeCache:          nodes.Cache(),
		namespace:          options.Namespace,
		upgradeClient:      upgrades,
		upgradeCache:       upgrades.Cache(),
		upgradeController:  upgrades,
		upgradeLogClient:   upgradeLogs,
		upgradeLogCache:    upgradeLogs.Cache(),
		versionCache:       versions.Cache(),
		planClient:         plans,
		planCache:          plans.Cache(),
		addonClient:        addons,
		addonCache:         addons.Cache(),
		managedChartClient: managedcharts,
		managedChartCache:  managedcharts.Cache(),
		vmImageClient:      vmImages,
		vmImageCache:       vmImages.Cache(),
		vmClient:           vms,
		vmCache:            vms.Cache(),
		serviceClient:      services,
		pvcClient:          pvcs,
		clusterClient:      clusters,
		clusterCache:       clusters.Cache(),
		lhSettingClient:    lhSettings,
		lhSettingCache:     lhSettings.Cache(),
		vmRestClient:       virtSubresourceClient,
		deploymentClient:   deployments,
		deploymentCache:    deployments.Cache(),
		scClient:           storageClasses,
		scCache:            storageClasses.Cache(),
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
		namespace:      options.Namespace,
		planCache:      plans.Cache(),
		upgradeClient:  upgrades,
		upgradeCache:   upgrades.Cache(),
		machineCache:   machines.Cache(),
		secretClient:   secrets,
		nodeClient:     nodes,
		nodeCache:      nodes.Cache(),
		jobClient:      jobs,
		jobCache:       jobs.Cache(),
		configMapCache: configMaps.Cache(),
		settingCache:   settings.Cache(),
	}
	jobs.OnChange(ctx, jobControllerName, jobHandler.OnChanged)

	deploymentHandler := &deploymentHandler{
		upgradeClient: upgrades,
		upgradeCache:  upgrades.Cache(),
	}
	deployments.OnChange(ctx, deploymentControllerName, deploymentHandler.OnChanged)

	vmImageHandler := &vmImageHandler{
		namespace:     options.Namespace,
		upgradeClient: upgrades,
		upgradeCache:  upgrades.Cache(),
	}
	vmImages.OnChange(ctx, vmImageControllerName, vmImageHandler.OnChanged)

	secretHandler := &secretHandler{
		namespace:        options.Namespace,
		upgradeClient:    upgrades,
		upgradeCache:     upgrades.Cache(),
		jobClient:        jobs,
		jobCache:         jobs.Cache(),
		machineCache:     machines.Cache(),
		secretController: secrets,
	}
	secrets.OnChange(ctx, secretControllerName, secretHandler.OnChanged)

	nodeHandler := &nodeHandler{
		namespace:     options.Namespace,
		nodeClient:    nodes,
		nodeCache:     nodes.Cache(),
		upgradeClient: upgrades,
		upgradeCache:  upgrades.Cache(),
		secretClient:  secrets,
	}
	nodes.OnChange(ctx, nodeControllerName, nodeHandler.OnChanged)

	versionSyncer := newVersionSyncer(ctx, options.Namespace, versions, nodes, namespaces)

	settingHandler := settingHandler{
		versionSyncer: versionSyncer,
	}
	settings.OnChange(ctx, settingControllerName, settingHandler.OnChanged)

	addOnHandler := &addonHandler{
		namespace:    options.Namespace,
		upgradeCache: upgrades.Cache(),
		addonClient:  addons,
		addonCache:   addons.Cache(),
	}
	addons.OnChange(ctx, "harvester-descheduler-addon-controller", addOnHandler.OnChanged)

	h := &handler{
		nodeCache: nodes.Cache(),
	}
	relatedresource.Watch(ctx, "watch-upgrade", h.NotifyUnpausedMachinePlanSecret, secrets, upgrades)

	go versionSyncer.start()

	return nil
}
