package storageclass

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	storageClassControllerName = "harvester-storage-class-controller"
)

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	storageClasses := management.StorageFactory.Storage().V1().StorageClass()
	storageProfiles := management.CdiFactory.Cdi().V1beta1().StorageProfile()
	cdi := management.CdiFactory.Cdi().V1beta1().CDI()
	volumeSnapshotClasses := management.SnapshotFactory.Snapshot().V1().VolumeSnapshotClass()

	storageClassHandler := &storageClassHandler{
		storageClassController:   storageClasses,
		storageProfileClient:     storageProfiles,
		storageProfileCache:      storageProfiles.Cache(),
		cdiClient:                cdi,
		cdiCache:                 cdi.Cache(),
		volumeSnapshotClassCache: volumeSnapshotClasses.Cache(),
	}

	storageClasses.OnChange(ctx, storageClassControllerName, storageClassHandler.OnChanged)
	return nil
}
