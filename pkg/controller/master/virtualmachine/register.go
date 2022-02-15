package virtualmachine

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	vmControllerCreatePVCsFromAnnotationControllerName = "VMController.CreatePVCsFromAnnotation"
	vmControllerSetOwnerOfPVCsControllerName           = "VMController.SetOwnerOfPVCs"
	vmControllerUnsetOwnerOfPVCsControllerName         = "VMController.UnsetOwnerOfPVCs"
	vmiControllerUnsetOwnerOfPVCsControllerName        = "VMIController.UnsetOwnerOfPVCs"
	vmControllerSetDefaultManagementNetworkMac         = "VMController.SetDefaultManagementNetworkMacAddress"
	vmControllerSetAnnotationsVolumeStatusName         = "VMController.SetVmAnnotationsVolumeStatus"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	var pvcClient = management.CoreFactory.Core().V1().PersistentVolumeClaim()
	var pvcCache = pvcClient.Cache()

	// registers the vm controller
	var vmCtrl = &VMController{
		pvcClient: pvcClient,
		pvcCache:  pvcCache,
	}
	var virtualMachineClient = management.VirtFactory.Kubevirt().V1().VirtualMachine()
	virtualMachineClient.OnChange(ctx, vmControllerCreatePVCsFromAnnotationControllerName, vmCtrl.createPVCsFromAnnotation)
	virtualMachineClient.OnChange(ctx, vmControllerSetOwnerOfPVCsControllerName, vmCtrl.SetOwnerOfPVCs)
	virtualMachineClient.OnRemove(ctx, vmControllerUnsetOwnerOfPVCsControllerName, vmCtrl.UnsetOwnerOfPVCs)

	// registers the vmi controller
	var virtualMachineCache = virtualMachineClient.Cache()
	var vmiCtrl = &VMIController{
		virtualMachineCache: virtualMachineCache,
		pvcClient:           pvcClient,
		pvcCache:            pvcCache,
	}
	var virtualMachineInstanceClient = management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	virtualMachineInstanceClient.OnRemove(ctx, vmiControllerUnsetOwnerOfPVCsControllerName, vmiCtrl.UnsetOwnerOfPVCs)

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

	// register the vm volume status controller, which tries to update LH Volume status to PVC and VM
	var volumeClient = management.LonghornFactory.Longhorn().V1beta1().Volume()
	var vmVolumeStatusController = &VMVolumeStatusController{
		vmClient:  vmClient,
		vmCache:   vmCache,
		pvcClient: pvcClient,
		pvcCache:  pvcCache,
	}
	volumeClient.OnChange(ctx, vmControllerSetAnnotationsVolumeStatusName, vmVolumeStatusController.OnLHVolumeChange)
	volumeClient.OnRemove(ctx, vmControllerSetAnnotationsVolumeStatusName, vmVolumeStatusController.OnLHVolumeRemove)
	pvcClient.OnChange(ctx, vmControllerSetAnnotationsVolumeStatusName, vmVolumeStatusController.OnPVCChange)
	pvcClient.OnRemove(ctx, vmControllerSetAnnotationsVolumeStatusName, vmVolumeStatusController.OnPVCRemove)

	return nil
}
