package vm

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/generated/clientset/versioned/scheme"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/wrangler/pkg/schemas"

	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

const (
	vmSchemaID = "kubevirt.io.virtualmachine"
)

var (
	kubevirtSubResouceGroupVersion = k8sschema.GroupVersion{Group: "subresources.kubevirt.io", Version: "v1alpha3"}
)

func RegisterSchema(scaled *config.Scaled, server *server.Server) error {
	// import the struct EjectCdRomActionInput to the schema, then the action could use it as input,
	// and because wrangler converts the struct typeName to lower title, so the action input should start with lower case.
	// https://github.com/rancher/wrangler/blob/master/pkg/schemas/reflection.go#L26
	server.BaseSchemas.MustImportAndCustomize(EjectCdRomActionInput{}, nil)

	vms := scaled.VirtFactory.Kubevirt().V1alpha3().VirtualMachine()
	vmis := scaled.VirtFactory.Kubevirt().V1alpha3().VirtualMachineInstance()

	copyConfig := rest.CopyConfig(server.RestConfig)
	copyConfig.GroupVersion = &kubevirtSubResouceGroupVersion
	copyConfig.APIPath = "/apis"
	copyConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	restClient, err := rest.RESTClientFor(copyConfig)
	if err != nil {
		return err
	}

	actionHandler := vmActionHandler{
		vms:        vms,
		vmis:       vmis,
		vmCache:    vms.Cache(),
		restClient: restClient,
	}

	vmformatter := vmformatter{
		vmiCache: vmis.Cache(),
	}

	t := schema.Template{
		ID: vmSchemaID,
		Customize: func(apiSchema *types.APISchema) {
			apiSchema.ActionHandlers = map[string]http.Handler{
				startVM:    &actionHandler,
				stopVM:     &actionHandler,
				restartVM:  &actionHandler,
				ejectCdRom: &actionHandler,
				pauseVM:    &actionHandler,
				unpauseVM:  &actionHandler,
			}
			apiSchema.ResourceActions = map[string]schemas.Action{
				startVM:   {},
				stopVM:    {},
				restartVM: {},
				pauseVM:   {},
				unpauseVM: {},
				ejectCdRom: {
					Input: "ejectCdRomActionInput",
				},
			}
		},
		Formatter: vmformatter.formatter,
	}

	server.SchemaTemplates = append(server.SchemaTemplates, t)
	return nil
}
