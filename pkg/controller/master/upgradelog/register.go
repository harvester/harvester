package upgradelog

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	upgradeLogControllerName    = "harvester-upgradelog-controller"
	clusterFlowControllerName   = "harvester-upgradelog-clusterflow-controller"
	clusterOutputControllerName = "harvester-upgradelog-clusteroutput-controller"
	daemonSetControllerName     = "harvester-upgradelog-daemonset-controller"
	deploymentControllerName    = "harvester-upgradelog-deployment-controller"
	jobControllerName           = "harvester-upgradelog-job-controller"
	loggingControllerName       = "harvester-upgradelog-logging-controller"
	statefulSetControllerName   = "harvester-upgradelog-statefulset-controller"
	managedChartControllerName  = "harvester-upgradelog-managedchart-controller"
	upgradeControllerName       = "harvester-upgradelog-upgrade-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	upgradeLogController := management.HarvesterFactory.Harvesterhci().V1beta1().UpgradeLog()
	addonController := management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
	clusterFlowController := management.LoggingFactory.Logging().V1beta1().ClusterFlow()
	clusterOutputController := management.LoggingFactory.Logging().V1beta1().ClusterOutput()
	daemonSetController := management.AppsFactory.Apps().V1().DaemonSet()
	deploymentController := management.AppsFactory.Apps().V1().Deployment()
	jobController := management.BatchFactory.Batch().V1().Job()
	loggingController := management.LoggingFactory.Logging().V1beta1().Logging()
	managedChartController := management.RancherManagementFactory.Management().V3().ManagedChart()
	pvcController := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	serviceController := management.CoreFactory.Core().V1().Service()
	statefulSetController := management.AppsFactory.Apps().V1().StatefulSet()
	upgradeController := management.HarvesterFactory.Harvesterhci().V1beta1().Upgrade()

	handler := &handler{
		ctx:                 ctx,
		namespace:           options.Namespace,
		addonCache:          addonController.Cache(),
		clusterFlowClient:   clusterFlowController,
		clusterOutputClient: clusterOutputController,
		daemonSetClient:     daemonSetController,
		daemonSetCache:      daemonSetController.Cache(),
		deploymentClient:    deploymentController,
		jobClient:           jobController,
		jobCache:            jobController.Cache(),
		loggingClient:       loggingController,
		managedChartClient:  managedChartController,
		managedChartCache:   managedChartController.Cache(),
		pvcClient:           pvcController,
		serviceClient:       serviceController,
		statefulSetClient:   statefulSetController,
		statefulSetCache:    statefulSetController.Cache(),
		upgradeClient:       upgradeController,
		upgradeCache:        upgradeController.Cache(),
		upgradeLogClient:    upgradeLogController,
		upgradeLogCache:     upgradeLogController.Cache(),
	}

	upgradeLogController.OnChange(ctx, upgradeLogControllerName, handler.OnUpgradeLogChange)
	upgradeLogController.OnRemove(ctx, upgradeLogControllerName, handler.OnUpgradeLogRemove)
	clusterFlowController.OnChange(ctx, clusterFlowControllerName, handler.OnClusterFlowChange)
	clusterOutputController.OnChange(ctx, clusterOutputControllerName, handler.OnClusterOutputChange)
	daemonSetController.OnChange(ctx, daemonSetControllerName, handler.OnDaemonSetChange)
	deploymentController.OnChange(ctx, deploymentControllerName, handler.OnDeploymentChange)
	jobController.OnChange(ctx, jobControllerName, handler.OnJobChange)
	statefulSetController.OnChange(ctx, statefulSetControllerName, handler.OnStatefulSetChange)
	managedChartController.OnChange(ctx, managedChartControllerName, handler.OnManagedChartChange)
	upgradeController.OnChange(ctx, upgradeControllerName, handler.OnUpgradeChange)

	return nil
}
