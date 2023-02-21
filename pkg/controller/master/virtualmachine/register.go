package virtualmachine

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	vmControllerCreatePVCsFromAnnotationControllerName = "VMController.CreatePVCsFromAnnotation"
	vmiControllerUnsetOwnerOfPVCsControllerName        = "VMIController.UnsetOwnerOfPVCs"
	vmiControllerReconcileFromHostLabelsControllerName = "VMIController.ReconcileFromHostLabels"
	vmControllerSetDefaultManagementNetworkMac         = "VMController.SetDefaultManagementNetworkMacAddress"
	vmControllerStoreRunStrategyControllerName         = "VMController.StoreRunStrategyToAnnotation"
	vmControllerSyncLabelsToVmi                        = "VMController.SyncLabelsToVmi"
	vmControllerManagePVCOwnerControllerName           = "VMController.ManageOwnerOfPVCs"
	harvesterUnsetOwnerOfPVCsFinalizer                 = "harvesterhci.io/VMController.UnsetOwnerOfPVCs"
	oldWranglerFinalizer                               = "wrangler.cattle.io/VMController.UnsetOwnerOfPVCs"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	var (
		pvcClient      = management.CoreFactory.Core().V1().PersistentVolumeClaim()
		pvcCache       = pvcClient.Cache()
		vmClient       = management.VirtFactory.Kubevirt().V1().VirtualMachine()
		vmCache        = vmClient.Cache()
		nodeClient     = management.CoreFactory.Core().V1().Node()
		nodeCache      = nodeClient.Cache()
		vmiClient      = management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
		vmiCache       = vmiClient.Cache()
		vmBackupClient = management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
		vmBackupCache  = vmBackupClient.Cache()
		snapshotClient = management.SnapshotFactory.Snapshot().V1beta1().VolumeSnapshot()
		snapshotCache  = snapshotClient.Cache()
	)

	// registers the vm controller
	var vmCtrl = &VMController{
		pvcClient:      pvcClient,
		pvcCache:       pvcCache,
		vmClient:       vmClient,
		vmCache:        vmCache,
		vmiClient:      vmiClient,
		vmiCache:       vmiCache,
		vmBackupClient: vmBackupClient,
		vmBackupCache:  vmBackupCache,
		snapshotClient: snapshotClient,
		snapshotCache:  snapshotCache,
	}
	var virtualMachineClient = management.VirtFactory.Kubevirt().V1().VirtualMachine()
	virtualMachineClient.OnChange(ctx, vmControllerCreatePVCsFromAnnotationControllerName, vmCtrl.createPVCsFromAnnotation)
	virtualMachineClient.OnChange(ctx, vmControllerManagePVCOwnerControllerName, vmCtrl.ManageOwnerOfPVCs)
	virtualMachineClient.OnChange(ctx, vmControllerStoreRunStrategyControllerName, vmCtrl.StoreRunStrategy)
	virtualMachineClient.OnChange(ctx, vmControllerSyncLabelsToVmi, vmCtrl.SyncLabelsToVmi)

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
