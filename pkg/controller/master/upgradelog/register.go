package upgradelog

import (
	"context"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/client-go/kubernetes"

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
	bundleControllerName        = "harvester-upgradelog-bundle-controller"
	pvcControllerName           = "harvester-upgradelog-pvc-controller"
	upgradeControllerName       = "harvester-upgradelog-upgrade-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	upgradeLogController := management.HarvesterFactory.Harvesterhci().V1beta1().UpgradeLog()
	addonController := management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
	configMapController := management.CoreFactory.Core().V1().ConfigMap()
	clusterFlowController := management.LoggingFactory.Logging().V1beta1().ClusterFlow()
	clusterOutputController := management.LoggingFactory.Logging().V1beta1().ClusterOutput()
	daemonSetController := management.AppsFactory.Apps().V1().DaemonSet()
	deploymentController := management.AppsFactory.Apps().V1().Deployment()
	jobController := management.BatchFactory.Batch().V1().Job()
	loggingController := management.LoggingFactory.Logging().V1beta1().Logging()
	pvcController := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	serviceController := management.CoreFactory.Core().V1().Service()
	statefulSetController := management.AppsFactory.Apps().V1().StatefulSet()
	upgradeController := management.HarvesterFactory.Harvesterhci().V1beta1().Upgrade()
	bundleController := management.FleetFactory.Fleet().V1alpha1().Bundle()

	restClientGetter := cli.New().RESTClientGetter()
	kubeClient := kube.New(restClientGetter)
	var kubeInterface kubernetes.Interface
	kubeInterface = management.ClientSet
	driverSecret := driver.NewSecrets(kubeInterface.CoreV1().Secrets(""))
	store := storage.Init(driverSecret)

	handler := &handler{
		ctx:                 ctx,
		namespace:           options.Namespace,
		addonCache:          addonController.Cache(),
		configMapClient:     configMapController,
		clusterFlowClient:   clusterFlowController,
		clusterOutputClient: clusterOutputController,
		daemonSetClient:     daemonSetController,
		daemonSetCache:      daemonSetController.Cache(),
		deploymentClient:    deploymentController,
		jobClient:           jobController,
		jobCache:            jobController.Cache(),
		loggingClient:       loggingController,
		bundleClient:        bundleController,
		bundleCache:         bundleController.Cache(),
		pvcClient:           pvcController,
		serviceClient:       serviceController,
		statefulSetClient:   statefulSetController,
		statefulSetCache:    statefulSetController.Cache(),
		upgradeClient:       upgradeController,
		upgradeCache:        upgradeController.Cache(),
		upgradeLogClient:    upgradeLogController,
		upgradeLogCache:     upgradeLogController.Cache(),

		helmConfiguration: action.Configuration{
			RESTClientGetter: restClientGetter,
			Releases:         store,
			KubeClient:       kubeClient,
		},
	}

	upgradeLogController.OnChange(ctx, upgradeLogControllerName, handler.OnUpgradeLogChange)
	upgradeLogController.OnRemove(ctx, upgradeLogControllerName, handler.OnUpgradeLogRemove)
	clusterFlowController.OnChange(ctx, clusterFlowControllerName, handler.OnClusterFlowChange)
	clusterOutputController.OnChange(ctx, clusterOutputControllerName, handler.OnClusterOutputChange)
	daemonSetController.OnChange(ctx, daemonSetControllerName, handler.OnDaemonSetChange)
	deploymentController.OnChange(ctx, deploymentControllerName, handler.OnDeploymentChange)
	jobController.OnChange(ctx, jobControllerName, handler.OnJobChange)
	statefulSetController.OnChange(ctx, statefulSetControllerName, handler.OnStatefulSetChange)
	bundleController.OnChange(ctx, bundleControllerName, handler.OnBundleChange)
	pvcController.OnChange(ctx, pvcControllerName, handler.OnPvcChange)
	upgradeController.OnChange(ctx, upgradeControllerName, handler.OnUpgradeChange)

	return nil
}
