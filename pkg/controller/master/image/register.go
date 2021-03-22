package image

import (
	"context"

	"github.com/rancher/harvester/pkg/config"
)

const (
	vmImageControllerName               = "vm-image-controller"
	backingImageStorageClassHandlerName = "backingimage-storageclass-handler"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	images := management.HarvesterFactory.Harvester().V1alpha1().VirtualMachineImage()
	storageClasses := management.StorageFactory.Storage().V1().StorageClass()
	controller := &Handler{
		images:     images,
		imageCache: images.Cache(),
		options:    options,
	}

	images.OnChange(ctx, vmImageControllerName, controller.OnImageChanged)
	images.OnRemove(ctx, vmImageControllerName, controller.OnImageRemove)

	backingImageStorageClassHandler := &backingImageStorageClassHandler{
		storageClasses: storageClasses,
	}
	images.OnChange(ctx, backingImageStorageClassHandlerName, backingImageStorageClassHandler.OnChanged)
	images.OnRemove(ctx, backingImageStorageClassHandlerName, backingImageStorageClassHandler.OnRemove)
	return nil
}
