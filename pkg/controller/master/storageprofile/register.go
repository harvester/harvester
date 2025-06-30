package storageprofile

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	storageProfileControllerName = "storage-profile-controller"
)

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	sc := management.StorageFactory.Storage().V1().StorageClass()
	ctlstorageprofile := management.CdiFactory.Cdi().V1beta1().StorageProfile()

	storageProfileHandler := &storageProfileHandler{
		scClient:                 sc,
		storageProfileClient:     ctlstorageprofile,
		storageProfileController: ctlstorageprofile,
	}

	ctlstorageprofile.OnChange(ctx, storageProfileControllerName, storageProfileHandler.OnChanged)
	return nil
}
