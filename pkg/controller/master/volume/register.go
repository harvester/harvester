package volume

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	volumeControllerDetachVolume = "detach-volume-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	var (
		pvcCache      = management.CoreFactory.Core().V1().PersistentVolumeClaim().Cache()
		volumeClient  = management.LonghornFactory.Longhorn().V1beta1().Volume()
		volumeCache   = volumeClient.Cache()
		snapshotCache = management.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshot().Cache()
	)

	// registers the volumecontroller
	var volumeCtrl = &Controller{
		pvcCache:         pvcCache,
		volumes:          volumeClient,
		volumeController: volumeClient,
		volumeCache:      volumeCache,
		snapshotCache:    snapshotCache,
	}
	volumeClient.OnChange(ctx, volumeControllerDetachVolume, volumeCtrl.DetachVolumesOnChange)

	return nil
}
