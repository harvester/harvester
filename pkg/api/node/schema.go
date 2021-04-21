package node

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/schemas"

	"github.com/harvester/harvester/pkg/config"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	nodeHandler := ActionHandler{
		nodeClient: scaled.Management.CoreFactory.Core().V1().Node(),
		nodeCache:  scaled.Management.CoreFactory.Core().V1().Node().Cache(),
	}
	t := schema.Template{
		ID: "node",
		Customize: func(s *types.APISchema) {
			s.Formatter = Formatter
			s.ResourceActions = map[string]schemas.Action{
				enableMaintenanceModeAction:  {},
				disableMaintenanceModeAction: {},
			}
			s.ActionHandlers = map[string]http.Handler{
				enableMaintenanceModeAction:  nodeHandler,
				disableMaintenanceModeAction: nodeHandler,
			}
		},
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
