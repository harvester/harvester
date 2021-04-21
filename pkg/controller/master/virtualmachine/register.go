package virtualmachine

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	vmControllerSetOwnerOfDataVolumesAgentName    = "VMController.SetOwnerOfDataVolumes"
	vmControllerUnsetOwnerOfDataVolumesAgentName  = "VMController.UnsetOwnerOfDataVolumes"
	vmiControllerUnsetOwnerOfDataVolumesAgentName = "VMIController.UnsetOwnerOfDataVolumes"
	vmControllerSetDefaultManagementNetworkMac    = "VMController.SetDefaultManagementNetworkMacAddress"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	var dataVolumeClient = management.CDIFactory.Cdi().V1beta1().DataVolume()
	var dataVolumeCache = dataVolumeClient.Cache()

	// registers the vm controller
	var vmCtrl = &VMController{
		dataVolumeClient: dataVolumeClient,
		dataVolumeCache:  dataVolumeCache,
	}
	var virtualMachineClient = management.VirtFactory.Kubevirt().V1().VirtualMachine()
	virtualMachineClient.OnChange(ctx, vmControllerSetOwnerOfDataVolumesAgentName, vmCtrl.SetOwnerOfDataVolumes)
	virtualMachineClient.OnRemove(ctx, vmControllerUnsetOwnerOfDataVolumesAgentName, vmCtrl.UnsetOwnerOfDataVolumes)

	// registers the vmi controller
	var virtualMachineCache = virtualMachineClient.Cache()
	var vmiCtrl = &VMIController{
		virtualMachineCache: virtualMachineCache,
		dataVolumeClient:    dataVolumeClient,
		dataVolumeCache:     dataVolumeCache,
	}
	var virtualMachineInstanceClient = management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	virtualMachineInstanceClient.OnRemove(ctx, vmiControllerUnsetOwnerOfDataVolumesAgentName, vmiCtrl.UnsetOwnerOfDataVolumes)

	// register the vm network controller upon the VMI changes
	var (
		vmClient  = management.VirtFactory.Kubevirt().V1().VirtualMachine()
		vmCache   = management.VirtFactory.Kubevirt().V1().VirtualMachine().Cache()
		vmiClient = management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	)
	var vmNetworkCtl = &VMNetworkController{
		vmClient:  vmClient,
		vmCache:   vmCache,
		vmiClient: vmiClient,
	}
	virtualMachineInstanceClient.OnChange(ctx, vmControllerSetDefaultManagementNetworkMac, vmNetworkCtl.SetDefaultNetworkMacAddress)

	return nil
}
