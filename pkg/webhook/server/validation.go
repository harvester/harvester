package server

import (
	"net/http"

	"github.com/rancher/wrangler/pkg/webhook"

	"github.com/harvester/harvester/pkg/webhook/clients"
	"github.com/harvester/harvester/pkg/webhook/config"
	"github.com/harvester/harvester/pkg/webhook/resources/datavolume"
	"github.com/harvester/harvester/pkg/webhook/resources/keypair"
	"github.com/harvester/harvester/pkg/webhook/resources/network"
	"github.com/harvester/harvester/pkg/webhook/resources/restore"
	"github.com/harvester/harvester/pkg/webhook/resources/templateversion"
	"github.com/harvester/harvester/pkg/webhook/resources/upgrade"
	"github.com/harvester/harvester/pkg/webhook/resources/user"
	"github.com/harvester/harvester/pkg/webhook/resources/virtualmachineimage"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func Validation(clients *clients.Clients, options *config.Options) (http.Handler, []types.Resource, error) {
	resources := []types.Resource{}
	validators := []types.Validator{
		network.NewValidator(clients.CNIFactory.K8s().V1().NetworkAttachmentDefinition().Cache(), clients.KubevirtFactory.Kubevirt().V1().VirtualMachine().Cache()),
		datavolume.NewValidator(clients.CDIFactory.Cdi().V1beta1().DataVolume().Cache()),
		keypair.NewValidator(clients.HarvesterFactory.Harvesterhci().V1beta1().KeyPair().Cache()),
		virtualmachineimage.NewValidator(clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage().Cache()),
		upgrade.NewValidator(clients.HarvesterFactory.Harvesterhci().V1beta1().Upgrade().Cache()),
		restore.NewValidator(clients.KubevirtFactory.Kubevirt().V1().VirtualMachine().Cache()),
		templateversion.NewValidator(
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplate().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplateVersion().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().KeyPair().Cache()),
		user.NewValidator(clients.HarvesterFactory.Harvesterhci().V1beta1().User().Cache()),
	}

	router := webhook.NewRouter()
	for _, v := range validators {
		addHandler(router, types.AdmissionTypeValidation, types.NewValidatorAdapter(v), options)
		resources = append(resources, v.Resource())
	}

	return router, resources, nil
}
