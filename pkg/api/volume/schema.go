package volume

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/v3/pkg/schemas"

	"github.com/harvester/harvester/pkg/config"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
	"github.com/harvester/harvester/pkg/util"
)

const (
	pvcSchemaID = "persistentvolumeclaim"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, _ config.Options) error {
	server.BaseSchemas.MustImportAndCustomize(ExportVolumeInput{}, nil)
	server.BaseSchemas.MustImportAndCustomize(CloneVolumeInput{}, nil)
	server.BaseSchemas.MustImportAndCustomize(SnapshotVolumeInput{}, nil)
	server.BaseSchemas.MustImportAndCustomize(DataMigrationInput{}, nil)
	actionHandler := &ActionHandler{
		images:      scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage(),
		pods:        scaled.CoreFactory.Core().V1().Pod().Cache(),
		pvcs:        scaled.CoreFactory.Core().V1().PersistentVolumeClaim(),
		pvcCache:    scaled.CoreFactory.Core().V1().PersistentVolumeClaim().Cache(),
		pvs:         scaled.HarvesterCoreFactory.Core().V1().PersistentVolume(),
		pvCache:     scaled.HarvesterCoreFactory.Core().V1().PersistentVolume().Cache(),
		scCache:     scaled.StorageFactory.Storage().V1().StorageClass().Cache(),
		snapshots:   scaled.SnapshotFactory.Snapshot().V1().VolumeSnapshot(),
		volumes:     scaled.LonghornFactory.Longhorn().V1beta2().Volume(),
		volumeCache: scaled.LonghornFactory.Longhorn().V1beta2().Volume().Cache(),
		vmCache:     scaled.VirtFactory.Kubevirt().V1().VirtualMachine().Cache(),
		dataVolumes: scaled.CdiFactory.Cdi().V1beta1().DataVolume(),
	}
	actionHandler.pods.AddIndexer(util.IndexPodByPVC, util.IndexPodByPVCFunc)

	handler := harvesterServer.NewHandler(actionHandler)

	formatter := &volFormatter{
		scCache: scaled.StorageFactory.Storage().V1().StorageClass().Cache(),
		vmCache: scaled.VirtFactory.Kubevirt().V1().VirtualMachine().Cache(),
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
				actionDataMigration: {
					Input: "dataMigrationInput",
				},
			}
			s.ActionHandlers = map[string]http.Handler{
				actionExport:        handler,
				actionCancelExpand:  handler,
				actionClone:         handler,
				actionSnapshot:      handler,
				actionDataMigration: handler,
			}
		},
		Formatter: formatter.Formatter,
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
