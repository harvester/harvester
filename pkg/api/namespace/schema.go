package namespace

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/v3/pkg/schemas"

	"github.com/harvester/harvester/pkg/config"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
)

type UpdateResourceQuotaInput struct {
	TotalSnapshotSizeQuota string `json:"totalSnapshotSizeQuota"`
}

func RegisterSchema(scaled *config.Scaled, server *server.Server, _ config.Options) error {

	handler := harvesterServer.NewHandler(&Handler{
		resourceQuotaClient: scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().ResourceQuota(),
		clientSet:           *scaled.Management.ClientSet,
		ctx:                 scaled.Ctx,
	})

	nsformatter := nsformatter{
		clientSet: *scaled.Management.ClientSet,
	}

	t := schema.Template{
		ID: "namespace",
		Customize: func(s *types.APISchema) {
			s.Formatter = nsformatter.formatter
			s.ResourceActions = map[string]schemas.Action{
				updateResourceQuotaAction: {
					Input: "updateResourceQuotaInput",
				},
				deleteResourceQuotaAction: {},
			}
			s.ActionHandlers = map[string]http.Handler{
				updateResourceQuotaAction: handler,
				deleteResourceQuotaAction: handler,
			}
		},
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
