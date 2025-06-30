package image

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/v3/pkg/schemas"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/backingimage"
	"github.com/harvester/harvester/pkg/image/cdi"
	"github.com/harvester/harvester/pkg/image/common"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, _ config.Options) error {
	vmi := scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage()
	bids := scaled.LonghornFactory.Longhorn().V1beta2().BackingImageDataSource()
	bi := scaled.LonghornFactory.Longhorn().V1beta2().BackingImage()
	ctlcdi := scaled.CdiFactory.Cdi().V1beta1().DataVolume()
	ctlcdiupload := scaled.CdiUploadFactory.Upload().V1beta1().UploadTokenRequest()
	vmImageDownloader := scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImageDownloader()
	scClient := scaled.StorageFactory.Storage().V1().StorageClass()

	vmio := common.GetVMIOperator(vmi, vmi.Cache(), http.Client{})
	downloaders := map[harvesterv1.VMIBackend]backend.Downloader{
		harvesterv1.VMIBackendBackingImage: backingimage.GetDownloader(bi.Cache(), http.Client{}, vmio),
		harvesterv1.VMIBackendCDI:          cdi.GetDownloader(vmImageDownloader, http.Client{}, vmio),
	}
	uploaders := map[harvesterv1.VMIBackend]backend.Uploader{
		harvesterv1.VMIBackendBackingImage: backingimage.GetUploader(bi.Cache(), bids, http.Client{}, vmio),
		harvesterv1.VMIBackendCDI:          cdi.GetUploader(ctlcdi, scClient, ctlcdiupload, http.Client{}, vmio),
	}

	imgHandler := Handler{
		vmiClient:   vmi,
		vmio:        vmio,
		downloaders: downloaders,
		uploaders:   uploaders,
	}

	t := schema.Template{
		ID: "harvesterhci.io.virtualmachineimage",
		Customize: func(s *types.APISchema) {
			s.Formatter = Formatter
			s.ResourceActions = map[string]schemas.Action{
				actionUpload: {},
			}
			/*
			 * ActionHandlers would let people define their own `POST` method.
			 * That would add to `actions` on API and would be filled as key-value
			 * pair in the current HTTP requests.
			 */
			s.ActionHandlers = map[string]http.Handler{
				actionUpload: imgHandler,
			}
			/*
			 * LinkHandlers would let people define their own `GET` method.
			 * That would add to `links` on API and would be filled as key-value
			 * pair in the current HTTP requests.
			 *
			 * Detail about `ActionHandlers` and `LinkHandlers` could be found
			 * with rancher/apiserver
			 */
			s.LinkHandlers = map[string]http.Handler{
				actionDownload:       imgHandler,
				actionDownloadCancel: imgHandler,
			}
		},
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
