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
	// import the struct EjectCdRomActionInput to the schema, then the action could use it as input,
	// and because wrangler converts the struct typeName to lower title, so the action input should start with lower case.
	// https://github.com/rancher/wrangler/blob/master/pkg/schemas/reflection.go#L26
	server.BaseSchemas.MustImportAndCustomize(EjectCdRomActionInput{}, nil)

	vms := scaled.VirtFactory.Kubevirt().V1alpha3().VirtualMachine()
	actionHandler := vmActionHandler{
		vms:     vms,
		vmis:    scaled.VirtFactory.Kubevirt().V1alpha3().VirtualMachineInstance(),
		vmCache: vms.Cache(),
	}

	t := schema.Template{
		ID: vmSchemaID,
		Customize: func(apiSchema *types.APISchema) {
			apiSchema.ActionHandlers = map[string]http.Handler{
				startVM:    &actionHandler,
				stopVM:     &actionHandler,
				restartVM:  &actionHandler,
				ejectCdRom: &actionHandler,
			}
			apiSchema.ResourceActions = map[string]schemas.Action{
				startVM:   {},
				stopVM:    {},
				restartVM: {},
				ejectCdRom: {
					Input: "ejectCdRomActionInput",
				},
			}
		},
		Formatter: formatter,
	}

	server.SchemaTemplates = append(server.SchemaTemplates, t)
}
