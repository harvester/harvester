package volume

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/schemas"

	"github.com/harvester/harvester/pkg/api/vm"
	"github.com/harvester/harvester/pkg/config"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	server.BaseSchemas.MustImportAndCustomize(vm.ExportVolumeInput{}, nil)
	t := schema.Template{
		ID: "persistentvolumeclaim",
		Customize: func(s *types.APISchema) {
			s.Formatter = Formatter
			s.ResourceActions = map[string]schemas.Action{
				actionExport: {
					Input: "exportVolumeInput",
				},
			}
			s.ActionHandlers = map[string]http.Handler{
				actionExport: ExportActionHandler{
					images: scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage(),
				},
			}
		},
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
