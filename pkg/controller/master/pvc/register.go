package pvc

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	pvcControllerName = "persistentvolumeclaim-controller"
)

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	dataVolume := management.CdiFactory.Cdi().V1beta1().DataVolume()
	ctlpvc := management.CoreFactory.Core().V1().PersistentVolumeClaim()

	pvcHandler := &pvcHandler{
		dataVolumeClient: dataVolume,
		pvcClient:        ctlpvc,
		pvcController:    ctlpvc,
	}

	ctlpvc.OnRemove(ctx, pvcControllerName, pvcHandler.cleanupDataVolume)
	return nil
}
