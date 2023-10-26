package virtualmachine

import (
	"context"

	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/scheme"
	"github.com/harvester/harvester/pkg/util/resourcequota"
)

const (
	vmControllerCreatePVCsFromAnnotationControllerName           = "VMController.CreatePVCsFromAnnotation"
	vmControllerRestartFromPausedIOErrorControllerName           = "VMController.RestartFromPausedIOError"
	vmiControllerUnsetOwnerOfPVCsControllerName                  = "VMIController.UnsetOwnerOfPVCs"
	vmiControllerReconcileFromHostLabelsControllerName           = "VMIController.ReconcileFromHostLabels"
	vmControllerSetDefaultManagementNetworkMac                   = "VMController.SetDefaultManagementNetworkMacAddress"
	vmControllerStoreRunStrategyControllerName                   = "VMController.StoreRunStrategyToAnnotation"
	vmControllerSyncLabelsToVmi                                  = "VMController.SyncLabelsToVmi"
	vmControllerManagePVCOwnerControllerName                     = "VMController.ManageOwnerOfPVCs"
	vmControllerSetHaltIfInsufficientResourceQuotaControllerName = "VMController.SetHaltIfInsufficientResourceQuota"
	vmiControllerSetHaltIfOccurExceededQuotaControllerName       = "VMIController.StopVMIfExceededQuota"
	settingControllerVMForceRestartPolicyControllerName          = "SettingController.VMForceRestartPolicy"
	harvesterUnsetOwnerOfPVCsFinalizer                           = "harvesterhci.io/VMController.UnsetOwnerOfPVCs"
	oldWranglerFinalizer                                         = "wrangler.cattle.io/VMController.UnsetOwnerOfPVCs"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	var (
		nsCache             = management.CoreFactory.Core().V1().Namespace().Cache()
		podCache            = management.CoreFactory.Core().V1().Pod().Cache()
		rqCache             = management.HarvesterCoreFactory.Core().V1().ResourceQuota().Cache()
		pvCache             = management.CoreFactory.Core().V1().PersistentVolume().Cache()
		pvcClient           = management.CoreFactory.Core().V1().PersistentVolumeClaim()
		pvcCache            = pvcClient.Cache()
		vmClient            = management.VirtFactory.Kubevirt().V1().VirtualMachine()
		vmCache             = vmClient.Cache()
		nodeClient          = management.CoreFactory.Core().V1().Node()
		nodeCache           = nodeClient.Cache()
		vmiClient           = management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
		vmiCache            = vmiClient.Cache()
		vmBackupClient      = management.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
		vmBackupCache       = vmBackupClient.Cache()
		snapshotClient      = management.SnapshotFactory.Snapshot().V1().VolumeSnapshot()
		vmimCache           = management.VirtFactory.Kubevirt().V1().VirtualMachineInstanceMigration().Cache()
		snapshotCache       = snapshotClient.Cache()
		longhornVolumeCache = management.LonghornFactory.Longhorn().V1beta2().Volume().Cache()
		recorder            = management.NewRecorder(vmControllerSetHaltIfInsufficientResourceQuotaControllerName, "", "")
	)

	virtSubsrcConfig := rest.CopyConfig(management.RestConfig)
	virtSubsrcConfig.GroupVersion = &k8sschema.GroupVersion{Group: "subresources.kubevirt.io", Version: "v1"}
	virtSubsrcConfig.APIPath = "/apis"
	virtSubsrcConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	virtSubresourceClient, err := rest.RESTClientFor(virtSubsrcConfig)
	if err != nil {
		return err
	}

	// registers the vm controller
	var vmCtrl = &VMController{
		pvCache:             pvCache,
		pvcClient:           pvcClient,
		pvcCache:            pvcCache,
		vmClient:            vmClient,
		vmController:        vmClient,
		vmiClient:           vmiClient,
		vmiCache:            vmiCache,
		vmBackupClient:      vmBackupClient,
		vmBackupCache:       vmBackupCache,
		snapshotClient:      snapshotClient,
		snapshotCache:       snapshotCache,
		longhornVolumeCache: longhornVolumeCache,
		recorder:            recorder,

		vmrCalculator:             resourcequota.NewCalculator(nsCache, podCache, rqCache, vmimCache),
		virtSubresourceRestClient: virtSubresourceClient,
	}
	var virtualMachineClient = management.VirtFactory.Kubevirt().V1().VirtualMachine()
	virtualMachineClient.OnChange(ctx, vmControllerCreatePVCsFromAnnotationControllerName, vmCtrl.createPVCsFromAnnotation)
	virtualMachineClient.OnChange(ctx, vmControllerManagePVCOwnerControllerName, vmCtrl.ManageOwnerOfPVCs)
	virtualMachineClient.OnChange(ctx, vmControllerStoreRunStrategyControllerName, vmCtrl.StoreRunStrategy)
	virtualMachineClient.OnChange(ctx, vmControllerSyncLabelsToVmi, vmCtrl.SyncLabelsToVmi)
	virtualMachineClient.OnChange(ctx, vmControllerSetHaltIfInsufficientResourceQuotaControllerName, vmCtrl.SetHaltIfInsufficientResourceQuota)
	virtualMachineClient.OnChange(ctx, vmControllerRestartFromPausedIOErrorControllerName, vmCtrl.RestartFromPausedIOError)

	// registers the vmi controller
	var virtualMachineCache = virtualMachineClient.Cache()
	var virtualMachineInstanceClient = management.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
	var vmiCtrl = &VMIController{
		vmClient:            vmClient,
		virtualMachineCache: virtualMachineCache,
		vmiClient:           virtualMachineInstanceClient,
		nodeCache:           nodeCache,
		pvcClient:           pvcClient,
		pvcCache:            pvcCache,
		recorder:            recorder,
	}
	virtualMachineInstanceClient.OnRemove(ctx, vmiControllerUnsetOwnerOfPVCsControllerName, vmiCtrl.UnsetOwnerOfPVCs)
	virtualMachineInstanceClient.OnChange(ctx, vmiControllerReconcileFromHostLabelsControllerName, vmiCtrl.ReconcileFromHostLabels)
	virtualMachineInstanceClient.OnChange(ctx, vmiControllerSetHaltIfOccurExceededQuotaControllerName, vmiCtrl.StopVMIfExceededQuota)

	// register the vm network controller upon the VMI changes
	var vmNetworkCtl = &VMNetworkController{
		vmClient:  vmClient,
		vmCache:   vmCache,
		vmiClient: virtualMachineInstanceClient,
	}
	virtualMachineInstanceClient.OnChange(ctx, vmControllerSetDefaultManagementNetworkMac, vmNetworkCtl.SetDefaultNetworkMacAddress)

	var settingCtl = &SettingController{
		vmController: vmClient,
		vmCache:      vmCache,
	}
	var settingClient = management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	settingClient.OnChange(ctx, settingControllerVMForceRestartPolicyControllerName, settingCtl.OnVMForceRestartPolicyChanged)
	return nil
}
