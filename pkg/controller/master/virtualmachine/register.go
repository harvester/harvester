package virtualmachine

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/util/resourcequota"
)

const (
	vmControllerCreatePVCsFromAnnotationControllerName           = "VMController.CreatePVCsFromAnnotation"
	vmiControllerReconcileFromHostLabelsControllerName           = "VMIController.ReconcileFromHostLabels"
	vmControllerSetDefaultManagementNetworkMac                   = "VMController.SetDefaultManagementNetworkMacAddress"
	vmControllerStoreRunStrategyControllerName                   = "VMController.StoreRunStrategyToAnnotation"
	vmControllerSyncLabelsToVmi                                  = "VMController.SyncLabelsToVmi"
	vmControllerSetHaltIfInsufficientResourceQuotaControllerName = "VMController.SetHaltIfInsufficientResourceQuota"
	vmControllerRemoveDeprecatedFinalizerControllerName          = "VMController.RemoveDeprecatedFinalizer"
	vmiControllerRemoveDeprecatedFinalizerControllerName         = "VMIController.RemoveDeprecatedFinalizer"
	vmiControllerSetHaltIfOccurExceededQuotaControllerName       = "VMIController.StopVMIfExceededQuota"

	vmControllerCleanupPVCAndSnapshotFinalizerName = "VMController.CleanupPVCAndSnapshot"
	// this finalizer is special one which was added by our controller, not wrangler.
	// https://github.com/harvester/harvester/blob/78b0f20abb118b5d0fba564e18867b90a1d3c0ee/pkg/controller/master/virtualmachine/vm_controller.go#L97-L101
	deprecatedHarvesterUnsetOwnerOfPVCsFinalizer = "harvesterhci.io/VMController.UnsetOwnerOfPVCs"
	deprecatedVMUnsetOwnerOfPVCsFinalizer        = "VMController.UnsetOwnerOfPVCs"
	deprecatedVMIUnsetOwnerOfPVCsFinalizer       = "VMIController.UnsetOwnerOfPVCs"
)

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	var (
		dataVolumeClient = management.CdiFactory.Cdi().V1beta1().DataVolume()
		nsCache          = management.CoreFactory.Core().V1().Namespace().Cache()
		podCache         = management.CoreFactory.Core().V1().Pod().Cache()
		podClient        = management.CoreFactory.Core().V1().Pod()
		rqCache          = management.HarvesterCoreFactory.Core().V1().ResourceQuota().Cache()
		pvcClient        = management.CoreFactory.Core().V1().PersistentVolumeClaim()
		pvcCache         = pvcClient.Cache()
		vmClient         = management.VirtFactory.Kubevirt().V1().VirtualMachine()
		vmCache          = vmClient.Cache()
		nodeClient       = management.CoreFactory.Core().V1().Node()
		nodeCache        = nodeClient.Cache()
		vmiClient        = management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
		vmiCache         = vmiClient.Cache()
		vmImgClient      = management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage()
		vmImgCache       = vmImgClient.Cache()
		vmBackupClient   = management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
		vmBackupCache    = vmBackupClient.Cache()
		snapshotClient   = management.SnapshotFactory.Snapshot().V1().VolumeSnapshot()
		vmimCache        = management.VirtFactory.Kubevirt().V1().VirtualMachineInstanceMigration().Cache()
		snapshotCache    = snapshotClient.Cache()
		scClient         = management.StorageFactory.Storage().V1().StorageClass()
		scCache          = scClient.Cache()
		settingCache     = management.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache()
		recorder         = management.NewRecorder(vmControllerSetHaltIfInsufficientResourceQuotaControllerName, "", "")
	)

	// registers the vm controller
	var vmCtrl = &VMController{
		dataVolumeClient: dataVolumeClient,
		podClient:        podClient,
		pvcClient:        pvcClient,
		pvcCache:         pvcCache,
		vmClient:         vmClient,
		vmController:     vmClient,
		vmiClient:        vmiClient,
		vmiCache:         vmiCache,
		vmImgClient:      vmImgClient,
		vmImgCache:       vmImgCache,
		vmBackupClient:   vmBackupClient,
		vmBackupCache:    vmBackupCache,
		snapshotClient:   snapshotClient,
		snapshotCache:    snapshotCache,
		scClient:         scClient,
		scCache:          scCache,
		settingCache:     settingCache,
		recorder:         recorder,

		vmrCalculator: resourcequota.NewCalculator(nsCache, podCache, rqCache, vmimCache, settingCache),
	}
	var virtualMachineClient = management.VirtFactory.Kubevirt().V1().VirtualMachine()
	virtualMachineClient.OnChange(ctx, vmControllerCreatePVCsFromAnnotationControllerName, vmCtrl.createPVCsFromAnnotation)
	virtualMachineClient.OnChange(ctx, vmControllerStoreRunStrategyControllerName, vmCtrl.StoreRunStrategy)
	virtualMachineClient.OnChange(ctx, vmControllerSyncLabelsToVmi, vmCtrl.SyncLabelsToVmi)
	virtualMachineClient.OnChange(ctx, vmControllerSetHaltIfInsufficientResourceQuotaControllerName, vmCtrl.SetHaltIfInsufficientResourceQuota)
	virtualMachineClient.OnChange(ctx, vmControllerRemoveDeprecatedFinalizerControllerName, vmCtrl.removeDeprecatedFinalizer)
	virtualMachineClient.OnRemove(ctx, vmControllerCleanupPVCAndSnapshotFinalizerName, vmCtrl.cleanupPVCAndSnapshot)

	// registers the vmi controller
	var virtualMachineCache = virtualMachineClient.Cache()
	var virtualMachineInstanceClient = management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	var vmiCtrl = &VMIController{
		vmClient:            vmClient,
		virtualMachineCache: virtualMachineCache,
		vmiClient:           virtualMachineInstanceClient,
		nodeCache:           nodeCache,
		pvcClient:           pvcClient,
		recorder:            recorder,
	}
	virtualMachineInstanceClient.OnChange(ctx, vmiControllerReconcileFromHostLabelsControllerName, vmiCtrl.ReconcileFromHostLabels)
	virtualMachineInstanceClient.OnChange(ctx, vmiControllerSetHaltIfOccurExceededQuotaControllerName, vmiCtrl.StopVMIfExceededQuota)
	virtualMachineInstanceClient.OnChange(ctx, vmiControllerRemoveDeprecatedFinalizerControllerName, vmiCtrl.removeDeprecatedFinalizer)

	// register the vm network controller upon the VMI changes
	var vmNetworkCtl = &VMNetworkController{
		vmClient:  vmClient,
		vmCache:   vmCache,
		vmiClient: virtualMachineInstanceClient,
	}
	virtualMachineInstanceClient.OnChange(ctx, vmControllerSetDefaultManagementNetworkMac, vmNetworkCtl.SetDefaultNetworkMacAddress)

	return nil
}
