package volume

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/schemas"

	"github.com/harvester/harvester/pkg/config"
)

const (
	pvcSchemaID = "persistentvolumeclaim"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	server.BaseSchemas.MustImportAndCustomize(ExportVolumeInput{}, nil)
	actionHandler := ActionHandler{
		images:   scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage(),
		pvcs:     scaled.CoreFactory.Core().V1().PersistentVolumeClaim(),
		pvcCache: scaled.CoreFactory.Core().V1().PersistentVolumeClaim().Cache(),
		pvs:      scaled.HarvesterCoreFactory.Core().V1().PersistentVolume(),
		pvCache:  scaled.HarvesterCoreFactory.Core().V1().PersistentVolume().Cache(),
	}
	t := schema.Template{
		ID: pvcSchemaID,
		Customize: func(s *types.APISchema) {
			s.ResourceActions = map[string]schemas.Action{
				actionExport: {
					Input: "exportVolumeInput",
				},
				actionCancelExpand: {},
			}
			s.ActionHandlers = map[string]http.Handler{
				actionExport:       &actionHandler,
				actionCancelExpand: &actionHandler,
			}
		},
		Formatter: Formatter,
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
