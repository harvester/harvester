package vm

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/schemas"
)

const (
	vmSchemaID = "kubevirt.io.virtualmachine"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server) {
	actionHandler := vmActionHandler{
		vms:  scaled.VirtFactory.Kubevirt().V1alpha3().VirtualMachine(),
		vmis: scaled.VirtFactory.Kubevirt().V1alpha3().VirtualMachineInstance(),
	}
	t := schema.Template{
		ID: vmSchemaID,
		Customize: func(apiSchema *types.APISchema) {
			apiSchema.ActionHandlers = map[string]http.Handler{
				startVM:   &actionHandler,
				stopVM:    &actionHandler,
				restartVM: &actionHandler,
			}
			apiSchema.ResourceActions = map[string]schemas.Action{
				startVM:   {},
				stopVM:    {},
				restartVM: {},
			}
		},
		Formatter: formatter,
	}

	server.SchemaTemplates = append(server.SchemaTemplates, t)
}
