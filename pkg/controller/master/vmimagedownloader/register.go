package vmimagedownloader

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	vmImageDownloaderControllerName = "vmimage-downloader-controller"
	deploymentControllerName        = "deployment-controller"
)

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	vmImageDownloader := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImageDownloader()
	vmImage := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage()
	deployment := management.AppsFactory.Apps().V1().Deployment()
	clientSet := management.ClientSet

	storageProfileHandler := &vmImageDownloaderHandler{
		clientSet:                   clientSet,
		vmImageClient:               vmImage,
		deploymentClient:            deployment,
		vmImageDownloaders:          vmImageDownloader,
		vmImageDownloaderController: vmImageDownloader,
	}

	vmImageDownloader.OnChange(ctx, vmImageDownloaderControllerName, storageProfileHandler.OnChanged)
	vmImageDownloader.OnRemove(ctx, vmImageDownloaderControllerName, storageProfileHandler.OnRemoved)

	deploymentHandler := &deploymentHandler{
		vmImageDownloaderClient: vmImageDownloader,
		deploymentCache:         deployment.Cache(),
		deploymentController:    deployment,
	}

	deployment.OnChange(ctx, deploymentControllerName, deploymentHandler.OnChanged)
	deployment.OnRemove(ctx, deploymentControllerName, deploymentHandler.OnRemoved)
	return nil
}
