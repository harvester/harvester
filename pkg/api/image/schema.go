package image

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/schemas"

	"github.com/harvester/harvester/pkg/config"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	t := schema.Template{
		ID: "harvesterhci.io.virtualmachineimage",
		Customize: func(s *types.APISchema) {
			s.Formatter = Formatter
			s.ResourceActions = map[string]schemas.Action{
				actionUpload: {},
			}
			s.ActionHandlers = map[string]http.Handler{
				actionUpload: ImageHandler{
					httpClient:                  http.Client{},
					Images:                      scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage(),
					ImageCache:                  scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage().Cache(),
					BackingImageDataSources:     scaled.LonghornFactory.Longhorn().V1beta1().BackingImageDataSource(),
					BackingImageDataSourceCache: scaled.LonghornFactory.Longhorn().V1beta1().BackingImageDataSource().Cache(),
				},
			}
		},
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
