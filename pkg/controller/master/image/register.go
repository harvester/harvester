package image

import (
	"context"
	"net/http"
	"time"

	"github.com/harvester/harvester/pkg/config"
)

const (
	vmImageControllerName      = "vm-image-controller"
	backingImageControllerName = "backing-image-controller"
)

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	backingImages := management.LonghornFactory.Longhorn().V1beta2().BackingImage()
	images := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage()
	storageClasses := management.StorageFactory.Storage().V1().StorageClass()
	pvcs := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	secrets := management.CoreFactory.Core().V1().Secret()
	vmImageHandler := &vmImageHandler{
		backingImages:     backingImages,
		backingImageCache: backingImages.Cache(),
		storageClasses:    storageClasses,
		storageClassCache: storageClasses.Cache(),
		images:            images,
		imageController:   images,
		httpClient: http.Client{
			Timeout: 15 * time.Second,
		},
		pvcCache:    pvcs.Cache(),
		secretCache: secrets.Cache(),
	}
	backingImageHandler := &backingImageHandler{
		vmImages:          images,
		vmImageCache:      images.Cache(),
		backingImages:     backingImages,
		backingImageCache: backingImages.Cache(),
	}

	images.OnChange(ctx, vmImageControllerName, vmImageHandler.OnChanged)
	images.OnRemove(ctx, vmImageControllerName, vmImageHandler.OnRemove)

	backingImages.OnChange(ctx, backingImageControllerName, backingImageHandler.OnChanged)
	return nil
}
