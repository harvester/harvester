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

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/scheme"
	virtv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
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
	server.BaseSchemas.MustImportAndCustomize(MigrateInput{}, nil)
	server.BaseSchemas.MustImportAndCustomize(CreateTemplateInput{}, nil)
	server.BaseSchemas.MustImportAndCustomize(AddVolumeInput{}, nil)
	server.BaseSchemas.MustImportAndCustomize(RemoveVolumeInput{}, nil)

	vms := scaled.VirtFactory.Kubevirt().V1().VirtualMachine()
	vmis := scaled.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	vmims := scaled.VirtFactory.Kubevirt().V1().VirtualMachineInstanceMigration()
	backups := scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
	restores := scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineRestore()
	settings := scaled.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	nodes := scaled.CoreFactory.Core().V1().Node()
	pvcs := scaled.CoreFactory.Core().V1().PersistentVolumeClaim()
	secrets := scaled.CoreFactory.Core().V1().Secret()
	vmt := scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplate()
	vmtv := scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplateVersion()

	copyConfig := rest.CopyConfig(server.RESTConfig)
	copyConfig.GroupVersion = &kubevirtSubResouceGroupVersion
	copyConfig.APIPath = "/apis"
	copyConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	virtSubresourceClient, err := rest.RESTClientFor(copyConfig)
	if err != nil {
		return err
	}
	virtv1Client, err := virtv1.NewForConfig(copyConfig)
	if err != nil {
		return err
	}
	actionHandler := vmActionHandler{
		namespace:                 options.Namespace,
		vms:                       vms,
		vmCache:                   vms.Cache(),
		vmis:                      vmis,
		vmiCache:                  vmis.Cache(),
		vmims:                     vmims,
		vmimCache:                 vmims.Cache(),
		vmTemplateClient:          vmt,
		vmTemplateVersionClient:   vmtv,
		backups:                   backups,
		backupCache:               backups.Cache(),
		restores:                  restores,
		settingCache:              settings.Cache(),
		nodeCache:                 nodes.Cache(),
		pvcCache:                  pvcs.Cache(),
		secretClient:              secrets,
		secretCache:               secrets.Cache(),
		virtSubresourceRestClient: virtSubresourceClient,
		virtRestClient:            virtv1Client.RESTClient(),
	}

	vmformatter := vmformatter{
		vmiCache: vmis.Cache(),
	}

	vmStore := &vmStore{
		Store:    proxy.NewProxyStore(server.ClientFactory, nil, server.AccessSetLookup),
		vms:      scaled.VirtFactory.Kubevirt().V1().VirtualMachine(),
		vmCache:  scaled.VirtFactory.Kubevirt().V1().VirtualMachine().Cache(),
		pvcs:     scaled.CoreFactory.Core().V1().PersistentVolumeClaim(),
		pvcCache: scaled.CoreFactory.Core().V1().PersistentVolumeClaim().Cache(),
	}

	t := schema.Template{
		ID: vmSchemaID,
		Customize: func(apiSchema *types.APISchema) {
			apiSchema.ActionHandlers = map[string]http.Handler{
				startVM:        &actionHandler,
				stopVM:         &actionHandler,
				restartVM:      &actionHandler,
				softReboot:     &actionHandler,
				ejectCdRom:     &actionHandler,
				pauseVM:        &actionHandler,
				unpauseVM:      &actionHandler,
				migrate:        &actionHandler,
				abortMigration: &actionHandler,
				backupVM:       &actionHandler,
				restoreVM:      &actionHandler,
				createTemplate: &actionHandler,
				addVolume:      &actionHandler,
				removeVolume:   &actionHandler,
			}
			apiSchema.ResourceActions = map[string]schemas.Action{
				startVM:    {},
				stopVM:     {},
				restartVM:  {},
				softReboot: {},
				pauseVM:    {},
				unpauseVM:  {},
				migrate: {
					Input: "migrateInput",
				},
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
				createTemplate: {
					Input: "createTemplateInput",
				},
				addVolume: {
					Input: "addVolumeInput",
				},
				removeVolume: {
					Input: "removeVolumeInput",
				},
			}
		},
		Formatter: vmformatter.formatter,
		Store:     vmStore,
	}

	server.SchemaFactory.AddTemplate(t)
	return nil
}
