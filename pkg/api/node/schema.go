package node

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/schemas"

	"github.com/harvester/harvester/pkg/config"
)

type MaintenanceModeInput struct {
	Force string `json:"force"`
}

type ListUnhealthyVM struct {
	Message string   `json:"message"`
	VMs     []string `json:"vms"`
}

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	nodeHandler := ActionHandler{
		nodeClient:                  scaled.Management.CoreFactory.Core().V1().Node(),
		nodeCache:                   scaled.Management.CoreFactory.Core().V1().Node().Cache(),
		longhornReplicaCache:        scaled.Management.LonghornFactory.Longhorn().V1beta1().Replica().Cache(),
		longhornVolumeCache:         scaled.Management.LonghornFactory.Longhorn().V1beta1().Volume().Cache(),
		virtualMachineInstanceCache: scaled.Management.VirtFactory.Kubevirt().V1().VirtualMachineInstance().Cache(),
	}

	server.BaseSchemas.MustImportAndCustomize(MaintenanceModeInput{}, nil)

	t := schema.Template{
		ID: "node",
		Customize: func(s *types.APISchema) {
			s.Formatter = Formatter
			s.ResourceActions = map[string]schemas.Action{
				enableMaintenanceModeAction: {
					Input: "maintenanceModeInput",
				},
				disableMaintenanceModeAction: {},
				cordonAction:                 {},
				uncordonAction:               {},
				listUnhealthyVM:              {},
				maintenancePossible:          {},
			}
			s.ActionHandlers = map[string]http.Handler{
				enableMaintenanceModeAction:  nodeHandler,
				disableMaintenanceModeAction: nodeHandler,
				cordonAction:                 nodeHandler,
				uncordonAction:               nodeHandler,
				listUnhealthyVM:              nodeHandler,
				maintenancePossible:          nodeHandler,
			}
		},
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
