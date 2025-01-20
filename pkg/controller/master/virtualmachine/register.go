package virtualmachine

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/util/resourcequota"
)

const (
	vmControllerCreatePVCsFromAnnotationControllerName           = "VMController.CreatePVCsFromAnnotation"
	vmControllerSetDefaultManagementNetworkMac                   = "VMController.SetDefaultManagementNetworkMacAddress"
	vmControllerStoreRunStrategyControllerName                   = "VMController.StoreRunStrategyToAnnotation"
	vmControllerSyncLabelsToVmi                                  = "VMController.SyncLabelsToVmi"
	vmControllerSetHaltIfInsufficientResourceQuotaControllerName = "VMController.SetHaltIfInsufficientResourceQuota"
	vmControllerRemoveDeprecatedFinalizerControllerName          = "VMController.RemoveDeprecatedFinalizer"
	vmControllerCleanupPVCAndSnapshotFinalizerName               = "VMController.CleanupPVCAndSnapshot"

	vmiControllerReconcileFromHostLabelsControllerName     = "VMIController.ReconcileFromHostLabels"
	vmiControllerRemoveDeprecatedFinalizerControllerName   = "VMIController.RemoveDeprecatedFinalizer"
	vmiControllerSetHaltIfOccurExceededQuotaControllerName = "VMIController.StopVMIfExceededQuota"
	vmiControllerSyncHarvesterVMILabelsToPodName           = "VMIController.SyncHarvesterVMILabelsToPod"

	// this finalizer is special one which was added by our controller, not wrangler.
	// https://github.com/harvester/harvester/blob/78b0f20abb118b5d0fba564e18867b90a1d3c0ee/pkg/controller/master/virtualmachine/vm_controller.go#L97-L101
	deprecatedHarvesterUnsetOwnerOfPVCsFinalizer = "harvesterhci.io/VMController.UnsetOwnerOfPVCs"
	deprecatedVMUnsetOwnerOfPVCsFinalizer        = "VMController.UnsetOwnerOfPVCs"
	deprecatedVMIUnsetOwnerOfPVCsFinalizer       = "VMIController.UnsetOwnerOfPVCs"
)

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	var (
		nodeClient     = management.CoreFactory.Core().V1().Node()
		nodeCache      = nodeClient.Cache()
		podClient      = management.CoreFactory.Core().V1().Pod()
		podCache       = podClient.Cache()
		pvcClient      = management.CoreFactory.Core().V1().PersistentVolumeClaim()
		pvcCache       = pvcClient.Cache()
		vmClient       = management.VirtFactory.Kubevirt().V1().VirtualMachine()
		vmCache        = vmClient.Cache()
		vmiClient      = management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
		vmiCache       = vmiClient.Cache()
		vmBackupClient = management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
		vmBackupCache  = vmBackupClient.Cache()
		crClient       = management.ControllerRevisionFactory.Apps().V1().ControllerRevision()
		crClientCache  = crClient.Cache()

		snapshotClient = management.SnapshotFactory.Snapshot().V1().VolumeSnapshot()

		nsCache       = management.CoreFactory.Core().V1().Namespace().Cache()
		rqCache       = management.HarvesterCoreFactory.Core().V1().ResourceQuota().Cache()
		vmimCache     = management.VirtFactory.Kubevirt().V1().VirtualMachineInstanceMigration().Cache()
		snapshotCache = snapshotClient.Cache()

		recorder = management.NewRecorder(vmControllerSetHaltIfInsufficientResourceQuotaControllerName, "", "")
	)

	// registers the vm controller
	var vmCtrl = &VMController{
		pvcClient:      pvcClient,
		pvcCache:       pvcCache,
		vmClient:       vmClient,
		vmController:   vmClient,
		vmiClient:      vmiClient,
		vmiCache:       vmiCache,
		vmBackupClient: vmBackupClient,
		vmBackupCache:  vmBackupCache,
		snapshotClient: snapshotClient,
		snapshotCache:  snapshotCache,
		recorder:       recorder,

		vmrCalculator: resourcequota.NewCalculator(nsCache, podCache, rqCache, vmimCache),
	}
	vmClient.OnChange(ctx, vmControllerCreatePVCsFromAnnotationControllerName, vmCtrl.createPVCsFromAnnotation)
	vmClient.OnChange(ctx, vmControllerStoreRunStrategyControllerName, vmCtrl.StoreRunStrategy)
	vmClient.OnChange(ctx, vmControllerSyncLabelsToVmi, vmCtrl.SyncLabelsToVmi)
	vmClient.OnChange(ctx, vmControllerSetHaltIfInsufficientResourceQuotaControllerName, vmCtrl.SetHaltIfInsufficientResourceQuota)
	vmClient.OnChange(ctx, vmControllerRemoveDeprecatedFinalizerControllerName, vmCtrl.removeDeprecatedFinalizer)
	vmClient.OnRemove(ctx, vmControllerCleanupPVCAndSnapshotFinalizerName, vmCtrl.cleanupPVCAndSnapshot)

	// registers the vmi controller
	var vmiCtrl = &VMIController{
		podClient:           podClient,
		podCache:            podCache,
		vmClient:            vmClient,
		virtualMachineCache: vmCache,
		vmiClient:           vmiClient,
		nodeCache:           nodeCache,
		pvcClient:           pvcClient,
		recorder:            recorder,
	}
	vmiClient.OnChange(ctx, vmiControllerReconcileFromHostLabelsControllerName, vmiCtrl.ReconcileFromHostLabels)
	vmiClient.OnChange(ctx, vmiControllerSetHaltIfOccurExceededQuotaControllerName, vmiCtrl.StopVMIfExceededQuota)
	vmiClient.OnChange(ctx, vmiControllerRemoveDeprecatedFinalizerControllerName, vmiCtrl.removeDeprecatedFinalizer)
	vmiClient.OnChange(ctx, vmiControllerSyncHarvesterVMILabelsToPodName, vmiCtrl.SyncHarvesterVMILabelsToPod)

	// register the vm network controller upon the VMI changes
	var vmNetworkCtl = &VMNetworkController{
		vmClient:  vmClient,
		vmCache:   vmCache,
		vmiClient: vmiClient,
		crClient:  crClient,
		crCache:   crClientCache,
	}
	vmiClient.OnChange(ctx, vmControllerSetDefaultManagementNetworkMac, vmNetworkCtl.SetDefaultNetworkMacAddress)

	return nil
}
