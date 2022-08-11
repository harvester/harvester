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
	server.BaseSchemas.MustImportAndCustomize(CloneVolumeInput{}, nil)
	server.BaseSchemas.MustImportAndCustomize(SnapshotVolumeInput{}, nil)
	actionHandler := ActionHandler{
		images:      scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage(),
		pvcs:        scaled.CoreFactory.Core().V1().PersistentVolumeClaim(),
		pvcCache:    scaled.CoreFactory.Core().V1().PersistentVolumeClaim().Cache(),
		pvs:         scaled.HarvesterCoreFactory.Core().V1().PersistentVolume(),
		pvCache:     scaled.HarvesterCoreFactory.Core().V1().PersistentVolume().Cache(),
		snapshots:   scaled.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshot(),
		volumes:     scaled.LonghornFactory.Longhorn().V1beta1().Volume(),
		volumeCache: scaled.LonghornFactory.Longhorn().V1beta1().Volume().Cache(),
	}

	t := schema.Template{
		ID: pvcSchemaID,
		Customize: func(s *types.APISchema) {
			s.ResourceActions = map[string]schemas.Action{
				actionExport: {
					Input: "exportVolumeInput",
				},
				actionCancelExpand: {},
				actionClone: {
					Input: "cloneVolumeInput",
				},
				actionSnapshot: {
					Input: "snapshotVolumeInput",
				},
			}
			s.ActionHandlers = map[string]http.Handler{
				actionExport:       &actionHandler,
				actionCancelExpand: &actionHandler,
				actionClone:        &actionHandler,
				actionSnapshot:     &actionHandler,
			}
		},
		Formatter: Formatter,
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
