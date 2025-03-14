package image

import (
	"context"
	"net/http"
	"time"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/backingimage"
	"github.com/harvester/harvester/pkg/image/cdi"
	"github.com/harvester/harvester/pkg/image/common"
)

const (
	vmImageControllerName = "vm-image-controller"
)

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	bi := management.LonghornFactory.Longhorn().V1beta2().BackingImage()
	vmi := management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage()
	sc := management.StorageFactory.Storage().V1().StorageClass()
	pvcs := management.CoreFactory.Core().V1().PersistentVolumeClaim()
	secrets := management.CoreFactory.Core().V1().Secret()
	ctlcdi := management.CdiFactory.Cdi().V1beta1().DataVolume()

	vmio := common.GetVMIOperator(vmi, nil, http.Client{Timeout: 15 * time.Second})
	backends := map[harvesterv1.VMIBackend]backend.Backend{
		harvesterv1.VMIBackendBackingImage: backingimage.GetBackend(
			ctx, sc, sc.Cache(),
			bi, bi, bi.Cache(),
			pvcs.Cache(), secrets.Cache(),
			vmi, vmi.Cache(), vmio,
		),
		harvesterv1.VMIBackendCDI: cdi.GetBackend(ctx, ctlcdi, sc, pvcs.Cache(), vmio),
	}

	vmImageHandler := &vmImageHandler{
		vmiClient:     vmi,
		vmiController: vmi,
		vmio:          vmio,
		backends:      backends,
	}

	vmi.OnChange(ctx, vmImageControllerName, vmImageHandler.OnChanged)
	vmi.OnRemove(ctx, vmImageControllerName, vmImageHandler.OnRemove)

	for _, b := range backends {
		b.AddSidecarHandler()
	}
	return nil
}
