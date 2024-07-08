package volume

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/v3/pkg/schemas"

	"github.com/harvester/harvester/pkg/config"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
)

const (
	pvcSchemaID = "persistentvolumeclaim"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, _ config.Options) error {
	server.BaseSchemas.MustImportAndCustomize(ExportVolumeInput{}, nil)
	server.BaseSchemas.MustImportAndCustomize(CloneVolumeInput{}, nil)
	server.BaseSchemas.MustImportAndCustomize(SnapshotVolumeInput{}, nil)
	actionHandler := &ActionHandler{
		images:      scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage(),
		pvcs:        scaled.CoreFactory.Core().V1().PersistentVolumeClaim(),
		pvcCache:    scaled.CoreFactory.Core().V1().PersistentVolumeClaim().Cache(),
		pvs:         scaled.HarvesterCoreFactory.Core().V1().PersistentVolume(),
		pvCache:     scaled.HarvesterCoreFactory.Core().V1().PersistentVolume().Cache(),
		snapshots:   scaled.SnapshotFactory.Snapshot().V1().VolumeSnapshot(),
		volumes:     scaled.LonghornFactory.Longhorn().V1beta2().Volume(),
		volumeCache: scaled.LonghornFactory.Longhorn().V1beta2().Volume().Cache(),
		vmCache:     scaled.VirtFactory.Kubevirt().V1().VirtualMachine().Cache(),
	}

	handler := harvesterServer.NewHandler(actionHandler)

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
				actionExport:       handler,
				actionCancelExpand: handler,
				actionClone:        handler,
				actionSnapshot:     handler,
			}
		},
		Formatter: Formatter,
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
