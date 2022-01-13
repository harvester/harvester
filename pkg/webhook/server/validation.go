package server

import (
	"net/http"

	"github.com/rancher/wrangler/pkg/webhook"

	"github.com/harvester/harvester/pkg/webhook/clients"
	"github.com/harvester/harvester/pkg/webhook/config"
	"github.com/harvester/harvester/pkg/webhook/resources/keypair"
	"github.com/harvester/harvester/pkg/webhook/resources/network"
	"github.com/harvester/harvester/pkg/webhook/resources/node"
	"github.com/harvester/harvester/pkg/webhook/resources/persistentvolumeclaim"
	"github.com/harvester/harvester/pkg/webhook/resources/restore"
	"github.com/harvester/harvester/pkg/webhook/resources/setting"
	"github.com/harvester/harvester/pkg/webhook/resources/templateversion"
	"github.com/harvester/harvester/pkg/webhook/resources/upgrade"
	"github.com/harvester/harvester/pkg/webhook/resources/virtualmachine"
	"github.com/harvester/harvester/pkg/webhook/resources/virtualmachineimage"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func Validation(clients *clients.Clients, options *config.Options) (http.Handler, []types.Resource, error) {
	resources := []types.Resource{}
	validators := []types.Validator{
		node.NewValidator(clients.Core.Node().Cache()),
		network.NewValidator(clients.CNIFactory.K8s().V1().NetworkAttachmentDefinition().Cache(), clients.KubevirtFactory.Kubevirt().V1().VirtualMachine().Cache()),
		persistentvolumeclaim.NewValidator(clients.Core.PersistentVolumeClaim().Cache(), clients.KubevirtFactory.Kubevirt().V1().VirtualMachine().Cache()),
		keypair.NewValidator(clients.HarvesterFactory.Harvesterhci().V1beta1().KeyPair().Cache()),
		virtualmachine.NewValidator(
			clients.Core.PersistentVolumeClaim().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup().Cache()),
		virtualmachineimage.NewValidator(
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage().Cache(),
			clients.Core.PersistentVolumeClaim().Cache(),
			clients.K8s.AuthorizationV1().SelfSubjectAccessReviews()),
		upgrade.NewValidator(clients.HarvesterFactory.Harvesterhci().V1beta1().Upgrade().Cache()),
		restore.NewValidator(
			clients.KubevirtFactory.Kubevirt().V1().VirtualMachine().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup().Cache(),
		),
		setting.NewValidator(
			clients.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup().Cache(),
		),
		templateversion.NewValidator(
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplate().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplateVersion().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().KeyPair().Cache()),
	}

	router := webhook.NewRouter()
	for _, v := range validators {
		addHandler(router, types.AdmissionTypeValidation, types.NewValidatorAdapter(v), options)
		resources = append(resources, v.Resource())
	}

	return router, resources, nil
}
