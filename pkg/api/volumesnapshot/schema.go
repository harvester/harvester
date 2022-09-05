package volumesnapshot

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/schemas"

	"github.com/harvester/harvester/pkg/config"
)

const (
	volumesnapshotSchemaID = "snapshot.storage.k8s.io.volumesnapshot"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	server.BaseSchemas.MustImportAndCustomize(RestoreSnapshotInput{}, nil)
	actionHandler := ActionHandler{
		pvcs:              scaled.CoreFactory.Core().V1().PersistentVolumeClaim(),
		pvcCache:          scaled.CoreFactory.Core().V1().PersistentVolumeClaim().Cache(),
		snapshotCache:     scaled.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshot().Cache(),
		storageClassCache: scaled.StorageFactory.Storage().V1().StorageClass().Cache(),
	}
	t := schema.Template{
		ID: volumesnapshotSchemaID,
		Customize: func(s *types.APISchema) {
			s.ResourceActions = map[string]schemas.Action{
				actionRestore: {
					Input: "restoreSnapshotInput",
				},
			}
			s.ActionHandlers = map[string]http.Handler{
				actionRestore: &actionHandler,
			}
		},
		Formatter: Formatter,
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
