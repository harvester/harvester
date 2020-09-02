package virtualmachine

import (
	"context"

	"github.com/rancher/harvester/pkg/config"
)

const (
	vmControllerSetOwnerOfDataVolumesAgentName   = "VMController.SetOwnerOfDataVolumes"
	vmControllerUnsetOwnerOfDataVolumesAgentName = "VMController.UnsetOwnerOfDataVolumes"

	vmiControllerUnsetOwnerOfDataVolumesAgentName = "VMIController.UnsetOwnerOfDataVolumes"
)

func Register(ctx context.Context, management *config.Management) error {
	var dataVolumeClient = management.CDIFactory.Cdi().V1beta1().DataVolume()
	var dataVolumeCache = dataVolumeClient.Cache()

	// registers the vm controller
	var vmCtrl = &VMController{
		dataVolumeClient: dataVolumeClient,
		dataVolumeCache:  dataVolumeCache,
	}
	var virtualMachineClient = management.VirtFactory.Kubevirt().V1alpha3().VirtualMachine()
	virtualMachineClient.OnChange(ctx, vmControllerSetOwnerOfDataVolumesAgentName, vmCtrl.SetOwnerOfDataVolumes)
	virtualMachineClient.OnRemove(ctx, vmControllerUnsetOwnerOfDataVolumesAgentName, vmCtrl.UnsetOwnerOfDataVolumes)

	// registers the vmi controller
	var virtualMachineCache = virtualMachineClient.Cache()
	var vmiCtrl = &VMIController{
		virtualMachineCache: virtualMachineCache,
		dataVolumeClient:    dataVolumeClient,
		dataVolumeCache:     dataVolumeCache,
	}
	var virtualMachineInstanceClient = management.VirtFactory.Kubevirt().V1alpha3().VirtualMachineInstance()
	virtualMachineInstanceClient.OnRemove(ctx, vmiControllerUnsetOwnerOfDataVolumesAgentName, vmiCtrl.UnsetOwnerOfDataVolumes)

	return nil
}
