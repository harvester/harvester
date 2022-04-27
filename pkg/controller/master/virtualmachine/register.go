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
	vmControllerStoreRunStrategyControllerName         = "VMController.StoreRunStrategyToAnnotation"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	var (
		pvcClient = management.CoreFactory.Core().V1().PersistentVolumeClaim()
		pvcCache  = pvcClient.Cache()
		vmClient  = management.VirtFactory.Kubevirt().V1().VirtualMachine()
		vmCache   = vmClient.Cache()
	)

	// registers the vm controller
	var vmCtrl = &VMController{
		pvcClient: pvcClient,
		pvcCache:  pvcCache,
		vmClient:  vmClient,
		vmCache:   vmCache,
	}
	var virtualMachineClient = management.VirtFactory.Kubevirt().V1().VirtualMachine()
	virtualMachineClient.OnChange(ctx, vmControllerCreatePVCsFromAnnotationControllerName, vmCtrl.createPVCsFromAnnotation)
	virtualMachineClient.OnChange(ctx, vmControllerSetOwnerOfPVCsControllerName, vmCtrl.SetOwnerOfPVCs)
	virtualMachineClient.OnChange(ctx, vmControllerStoreRunStrategyControllerName, vmCtrl.StoreRunStrategy)
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
	var vmiClient = management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	var vmNetworkCtl = &VMNetworkController{
		vmClient:  vmClient,
		vmCache:   vmCache,
		vmiClient: vmiClient,
	}
	virtualMachineInstanceClient.OnChange(ctx, vmControllerSetDefaultManagementNetworkMac, vmNetworkCtl.SetDefaultNetworkMacAddress)

	return nil
}
