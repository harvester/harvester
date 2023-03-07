package server

import (
	"net/http"

	"github.com/rancher/wrangler/pkg/webhook"

	"github.com/harvester/harvester/pkg/webhook/clients"
	"github.com/harvester/harvester/pkg/webhook/config"
	"github.com/harvester/harvester/pkg/webhook/resources/bundle"
	"github.com/harvester/harvester/pkg/webhook/resources/bundledeployment"
	"github.com/harvester/harvester/pkg/webhook/resources/keypair"
	"github.com/harvester/harvester/pkg/webhook/resources/managedchart"
	"github.com/harvester/harvester/pkg/webhook/resources/node"
	"github.com/harvester/harvester/pkg/webhook/resources/persistentvolumeclaim"
	"github.com/harvester/harvester/pkg/webhook/resources/setting"
	"github.com/harvester/harvester/pkg/webhook/resources/storageclass"
	"github.com/harvester/harvester/pkg/webhook/resources/templateversion"
	"github.com/harvester/harvester/pkg/webhook/resources/upgrade"
	"github.com/harvester/harvester/pkg/webhook/resources/virtualmachine"
	"github.com/harvester/harvester/pkg/webhook/resources/virtualmachinebackup"
	"github.com/harvester/harvester/pkg/webhook/resources/virtualmachineimage"
	"github.com/harvester/harvester/pkg/webhook/resources/virtualmachinerestore"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func Validation(clients *clients.Clients, options *config.Options) (http.Handler, []types.Resource, error) {
	resources := []types.Resource{}
	validators := []types.Validator{
		node.NewValidator(clients.Core.Node().Cache()),
		persistentvolumeclaim.NewValidator(clients.Core.PersistentVolumeClaim().Cache(), clients.KubevirtFactory.Kubevirt().V1().VirtualMachine().Cache()),
		keypair.NewValidator(clients.HarvesterFactory.Harvesterhci().V1beta1().KeyPair().Cache()),
		virtualmachine.NewValidator(
			clients.Core.PersistentVolumeClaim().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup().Cache()),
		virtualmachineimage.NewValidator(
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage().Cache(),
			clients.Core.PersistentVolumeClaim().Cache(),
			clients.K8s.AuthorizationV1().SelfSubjectAccessReviews(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplateVersion().Cache()),
		upgrade.NewValidator(
			clients.HarvesterFactory.Harvesterhci().V1beta1().Upgrade().Cache(),
			clients.Core.Node().Cache(),
			clients.LonghornFactory.Longhorn().V1beta1().Volume().Cache(),
			clients.ClusterFactory.Cluster().V1alpha4().Cluster().Cache(),
			clients.ClusterFactory.Cluster().V1alpha4().Machine().Cache(),
			clients.RancherManagementFactory.Management().V3().ManagedChart().Cache(),
		),
		virtualmachinebackup.NewValidator(
			clients.KubevirtFactory.Kubevirt().V1().VirtualMachine().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineRestore().Cache(),
		),
		virtualmachinerestore.NewValidator(
			clients.KubevirtFactory.Kubevirt().V1().VirtualMachine().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineRestore().Cache(),
			clients.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshotClass().Cache(),
		),
		setting.NewValidator(
			clients.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup().Cache(),
			clients.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshotClass().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineRestore().Cache(),
			clients.KubevirtFactory.Kubevirt().V1().VirtualMachineInstance().Cache(),
		),
		templateversion.NewValidator(
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplate().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplateVersion().Cache(),
			clients.HarvesterFactory.Harvesterhci().V1beta1().KeyPair().Cache()),
		managedchart.NewValidator(),
		bundle.NewValidator(),
		bundledeployment.NewValidator(
			clients.FleetFactory.Fleet().V1alpha1().Cluster().Cache(),
		),
		storageclass.NewValidator(clients.StorageFactory.Storage().V1().StorageClass().Cache()),
	}

	router := webhook.NewRouter()
	for _, v := range validators {
		addHandler(router, types.AdmissionTypeValidation, types.NewValidatorAdapter(v), options)
		resources = append(resources, v.Resource())
	}

	return router, resources, nil
}
