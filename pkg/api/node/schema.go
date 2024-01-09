package node

import (
	"fmt"
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/schemas"
	"k8s.io/client-go/dynamic"

	"github.com/harvester/harvester/pkg/config"
)

type MaintenanceModeInput struct {
	Force string `json:"force"`
}

type ListUnhealthyVM struct {
	Message string   `json:"message"`
	VMs     []string `json:"vms"`
}

type PowerActionInput struct {
	Operation string `json:"operation"`
}

func RegisterSchema(scaled *config.Scaled, server *server.Server, _ config.Options) error {

	dynamicClient, err := dynamic.NewForConfig(scaled.Management.RestConfig)
	if err != nil {
		return fmt.Errorf("error creating dyanmic client: %v", err)
	}
	nodeHandler := ActionHandler{
		nodeClient:                  scaled.Management.CoreFactory.Core().V1().Node(),
		nodeCache:                   scaled.Management.CoreFactory.Core().V1().Node().Cache(),
		longhornReplicaCache:        scaled.Management.LonghornFactory.Longhorn().V1beta2().Replica().Cache(),
		longhornVolumeCache:         scaled.Management.LonghornFactory.Longhorn().V1beta2().Volume().Cache(),
		virtualMachineInstanceCache: scaled.Management.VirtFactory.Kubevirt().V1().VirtualMachineInstance().Cache(),
		addonCache:                  scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Addon().Cache(),
		dynamicClient:               dynamicClient,
		ctx:                         scaled.Ctx,
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
				powerAction: {
					Input: "powerActionInput",
				},
				powerActionPossible: {},
			}
			s.ActionHandlers = map[string]http.Handler{
				enableMaintenanceModeAction:  nodeHandler,
				disableMaintenanceModeAction: nodeHandler,
				cordonAction:                 nodeHandler,
				uncordonAction:               nodeHandler,
				listUnhealthyVM:              nodeHandler,
				maintenancePossible:          nodeHandler,
				powerAction:                  nodeHandler,
				powerActionPossible:          nodeHandler,
			}
		},
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
