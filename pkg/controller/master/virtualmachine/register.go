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
	vmiControllerReconcileFromHostLabelsControllerName = "VMIController.ReconcileFromHostLabels"
	vmControllerSetDefaultManagementNetworkMac         = "VMController.SetDefaultManagementNetworkMacAddress"
	vmControllerStoreRunStrategyControllerName         = "VMController.StoreRunStrategyToAnnotation"
	vmControllerSyncLabelsToVmi                        = "VMController.SyncLabelsToVmi"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	var (
		pvcClient  = management.CoreFactory.Core().V1().PersistentVolumeClaim()
		pvcCache   = pvcClient.Cache()
		vmClient   = management.VirtFactory.Kubevirt().V1().VirtualMachine()
		vmCache    = vmClient.Cache()
		nodeClient = management.CoreFactory.Core().V1().Node()
		nodeCache  = nodeClient.Cache()
		vmiClient  = management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
		vmiCache   = vmiClient.Cache()
	)

	// registers the vm controller
	var vmCtrl = &VMController{
		pvcClient: pvcClient,
		pvcCache:  pvcCache,
		vmClient:  vmClient,
		vmCache:   vmCache,
		vmiClient: vmiClient,
		vmiCache:  vmiCache,
	}
	var virtualMachineClient = management.VirtFactory.Kubevirt().V1().VirtualMachine()
	virtualMachineClient.OnChange(ctx, vmControllerCreatePVCsFromAnnotationControllerName, vmCtrl.createPVCsFromAnnotation)
	virtualMachineClient.OnChange(ctx, vmControllerSetOwnerOfPVCsControllerName, vmCtrl.SetOwnerOfPVCs)
	virtualMachineClient.OnChange(ctx, vmControllerStoreRunStrategyControllerName, vmCtrl.StoreRunStrategy)
	virtualMachineClient.OnChange(ctx, vmControllerSyncLabelsToVmi, vmCtrl.SyncLabelsToVmi)
	virtualMachineClient.OnRemove(ctx, vmControllerUnsetOwnerOfPVCsControllerName, vmCtrl.UnsetOwnerOfPVCs)

	// registers the vmi controller
	var virtualMachineCache = virtualMachineClient.Cache()
	var virtualMachineInstanceClient = management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	var vmiCtrl = &VMIController{
		virtualMachineCache: virtualMachineCache,
		vmiClient:           virtualMachineInstanceClient,
		nodeCache:           nodeCache,
		pvcClient:           pvcClient,
		pvcCache:            pvcCache,
	}
	virtualMachineInstanceClient.OnRemove(ctx, vmiControllerUnsetOwnerOfPVCsControllerName, vmiCtrl.UnsetOwnerOfPVCs)
	virtualMachineInstanceClient.OnChange(ctx, vmiControllerReconcileFromHostLabelsControllerName, vmiCtrl.ReconcileFromHostLabels)

	// register the vm network controller upon the VMI changes
	var vmNetworkCtl = &VMNetworkController{
		vmClient:  vmClient,
		vmCache:   vmCache,
		vmiClient: virtualMachineInstanceClient,
	}
	virtualMachineInstanceClient.OnChange(ctx, vmControllerSetDefaultManagementNetworkMac, vmNetworkCtl.SetDefaultNetworkMacAddress)

	return nil
}
