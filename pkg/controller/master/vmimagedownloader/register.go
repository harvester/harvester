package vmimagedownloader

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/relatedresource"

	"github.com/harvester/harvester/pkg/config"
)

const (
	vmImageDownloaderControllerName = "vmimage-downloader-controller"
	deploymentWatcherName           = "deployment-watcher"
	deploymentControllerName        = "deployment-controller"
)

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	vmImageDownloader := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImageDownloader()
	vmImage := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage()
	deployment := management.AppsFactory.Apps().V1().Deployment()
	pvcCache := management.CoreFactory.Core().V1().PersistentVolumeClaim().Cache()
	clientSet := management.ClientSet

	storageProfileHandler := &vmImageDownloaderHandler{
		clientSet:                   clientSet,
		vmImageClient:               vmImage,
		pvcCache:                    pvcCache,
		deploymentClient:            deployment,
		vmImageDownloaders:          vmImageDownloader,
		vmImageDownloaderController: vmImageDownloader,
	}

	vmImageDownloader.OnChange(ctx, vmImageDownloaderControllerName, storageProfileHandler.OnChanged)
	vmImageDownloader.OnRemove(ctx, vmImageDownloaderControllerName, storageProfileHandler.OnRemoved)
	relatedresource.Watch(ctx, deploymentWatcherName, storageProfileHandler.ReconcileDeploymentOwners, vmImageDownloader, deployment)

	return nil
}
