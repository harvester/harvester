package image

import (
	"context"
	"net/http"
	"time"

	"github.com/rancher/harvester/pkg/config"
)

const (
	controllerName = "vm-image-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	images := management.HarvesterFactory.Harvester().V1alpha1().VirtualMachineImage()
	storageClasses := management.StorageFactory.Storage().V1().StorageClass()
	backingImageStorageClassHandler := &handler{
		storageClasses: storageClasses,
		images:         images,
		httpClient: http.Client{
			Timeout: 15 * time.Second,
		},
	}
	images.OnChange(ctx, controllerName, backingImageStorageClassHandler.OnChanged)
	images.OnRemove(ctx, controllerName, backingImageStorageClassHandler.OnRemove)
	return nil
}
