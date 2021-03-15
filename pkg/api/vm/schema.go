package vm

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"
	"github.com/rancher/wrangler/pkg/schemas"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/generated/clientset/versioned/scheme"
)

const (
	vmSchemaID = "kubevirt.io.virtualmachine"
)

var (
	kubevirtSubResouceGroupVersion = k8sschema.GroupVersion{Group: "subresources.kubevirt.io", Version: "v1"}
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	// import the struct EjectCdRomActionInput to the schema, then the action could use it as input,
	// and because wrangler converts the struct typeName to lower title, so the action input should start with lower case.
	// https://github.com/rancher/wrangler/blob/master/pkg/schemas/reflection.go#L26
	server.BaseSchemas.MustImportAndCustomize(EjectCdRomActionInput{}, nil)
	server.BaseSchemas.MustImportAndCustomize(BackupInput{}, nil)
	server.BaseSchemas.MustImportAndCustomize(RestoreInput{}, nil)

	vms := scaled.VirtFactory.Kubevirt().V1().VirtualMachine()
	vmis := scaled.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	vmims := scaled.VirtFactory.Kubevirt().V1().VirtualMachineInstanceMigration()
	backups := scaled.HarvesterFactory.Harvester().V1alpha1().VirtualMachineBackup()
	restores := scaled.HarvesterFactory.Harvester().V1alpha1().VirtualMachineRestore()
	settings := scaled.HarvesterFactory.Harvester().V1alpha1().Setting()

	copyConfig := rest.CopyConfig(server.RESTConfig)
	copyConfig.GroupVersion = &kubevirtSubResouceGroupVersion
	copyConfig.APIPath = "/apis"
	copyConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	restClient, err := rest.RESTClientFor(copyConfig)
	if err != nil {
		return err
	}

	actionHandler := vmActionHandler{
		vms:          vms,
		vmCache:      vms.Cache(),
		vmis:         vmis,
		vmiCache:     vmis.Cache(),
		vmims:        vmims,
		vmimCache:    vmims.Cache(),
		backups:      backups,
		backupCache:  backups.Cache(),
		restores:     restores,
		settingCache: settings.Cache(),
		restClient:   restClient,
	}

	vmformatter := vmformatter{
		vmiCache: vmis.Cache(),
	}

	vmStore := &vmStore{
		Store:            proxy.NewProxyStore(server.ClientFactory, nil, server.AccessSetLookup),
		vms:              scaled.VirtFactory.Kubevirt().V1().VirtualMachine(),
		vmCache:          scaled.VirtFactory.Kubevirt().V1().VirtualMachine().Cache(),
		dataVolumes:      scaled.CDIFactory.Cdi().V1beta1().DataVolume(),
		dataVolumesCache: scaled.CDIFactory.Cdi().V1beta1().DataVolume().Cache(),
	}

	t := schema.Template{
		ID: vmSchemaID,
		Customize: func(apiSchema *types.APISchema) {
			apiSchema.ActionHandlers = map[string]http.Handler{
				startVM:        &actionHandler,
				stopVM:         &actionHandler,
				restartVM:      &actionHandler,
				ejectCdRom:     &actionHandler,
				pauseVM:        &actionHandler,
				unpauseVM:      &actionHandler,
				migrate:        &actionHandler,
				abortMigration: &actionHandler,
				backupVM:       &actionHandler,
				restoreVM:      &actionHandler,
			}
			apiSchema.ResourceActions = map[string]schemas.Action{
				startVM:        {},
				stopVM:         {},
				restartVM:      {},
				pauseVM:        {},
				unpauseVM:      {},
				migrate:        {},
				abortMigration: {},
				ejectCdRom: {
					Input: "ejectCdRomActionInput",
				},
				backupVM: {
					Input: "backupInput",
				},
				restoreVM: {
					Input: "restoreInput",
				},
			}
		},
		Formatter: vmformatter.formatter,
		Store:     vmStore,
	}

	server.SchemaFactory.AddTemplate(t)
	return nil
}
