package virtualmachinebackup

import (
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/indexeres"
	"github.com/harvester/harvester/pkg/webhook/types"
	webhookutil "github.com/harvester/harvester/pkg/webhook/util"
)

const (
	fieldSourceName = "spec.source.name"
	fieldTypeName   = "spec.type"
)

func NewValidator(
	vms ctlkubevirtv1.VirtualMachineCache,
	setting ctlharvesterv1.SettingCache,
	vmrestores ctlharvesterv1.VirtualMachineRestoreCache,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	engineCache ctllonghornv1.EngineCache,
	scCache ctlstoragev1.StorageClassCache,
	resourceQuotaCache ctlharvesterv1.ResourceQuotaCache,
	vmimCache ctlkubevirtv1.VirtualMachineInstanceMigrationCache,
) types.Validator {
	return &virtualMachineBackupValidator{
		vms:                vms,
		setting:            setting,
		vmrestores:         vmrestores,
		pvcCache:           pvcCache,
		engineCache:        engineCache,
		scCache:            scCache,
		resourceQuotaCache: resourceQuotaCache,
		vmimCache:          vmimCache,
	}
}

type virtualMachineBackupValidator struct {
	types.DefaultValidator

	vms                ctlkubevirtv1.VirtualMachineCache
	setting            ctlharvesterv1.SettingCache
	vmrestores         ctlharvesterv1.VirtualMachineRestoreCache
	pvcCache           ctlcorev1.PersistentVolumeClaimCache
	engineCache        ctllonghornv1.EngineCache
	scCache            ctlstoragev1.StorageClassCache
	resourceQuotaCache ctlharvesterv1.ResourceQuotaCache
	vmimCache          ctlkubevirtv1.VirtualMachineInstanceMigrationCache
}

func (v *virtualMachineBackupValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.VirtualMachineBackupResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.VirtualMachineBackup{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
			admissionregv1.Delete,
		},
	}
}

func (v *virtualMachineBackupValidator) Create(_ *types.Request, newObj runtime.Object) error {
	newVMBackup := newObj.(*v1beta1.VirtualMachineBackup)

	if newVMBackup.Spec.Source.Name == "" {
		return werror.NewInvalidError("source VM name is empty", fieldSourceName)
	}

	validateFunc := v.validateStandardBackup
	if newVMBackup.Status != nil {
		validateFunc = v.validateVMBackupRecover
	}

	// Execute the selected validation.
	if err := validateFunc(newVMBackup); err != nil {
		return err
	}

	if newVMBackup.Spec.Type == v1beta1.Snapshot {
		return nil
	}

	// Additional check for backup type.
	if err := v.checkBackupTarget(); err != nil {
		return werror.NewInvalidError(err.Error(), fieldTypeName)
	}

	return nil
}

func (v *virtualMachineBackupValidator) validateStandardBackup(vmb *v1beta1.VirtualMachineBackup) error {
	// Retrieve the VM instance.
	vm, err := v.vms.Get(vmb.Namespace, vmb.Spec.Source.Name)
	if err != nil {
		return werror.NewInvalidError(err.Error(), fieldSourceName)
	}
	// Validate VM migration.
	if err := v.checkVMInstanceMigration(vm); err != nil {
		return werror.NewInvalidError(err.Error(), fieldSourceName)
	}
	// Check the total snapshot size.
	if err := v.checkTotalSnapshotSize(vm); err != nil {
		return err
	}
	// Validate backup volume snapshot class.
	if err := v.checkBackupVolumeSnapshotClass(vm, vmb); err != nil {
		return werror.NewInvalidError(err.Error(), fieldSourceName)
	}
	return nil
}

func (v *virtualMachineBackupValidator) validateVMBackupRecover(vmb *v1beta1.VirtualMachineBackup) error {
	// Perform LH backup specific validation.
	return webhookutil.IsLHBackupRelated(vmb)
}

// checkBackupVolumeSnapshotClass checks if the volumeSnapshotClassName is configured for the provisioner used by the PVCs in the VirtualMachine.
func (v *virtualMachineBackupValidator) checkBackupVolumeSnapshotClass(vm *kubevirtv1.VirtualMachine, newVMBackup *v1beta1.VirtualMachineBackup) error {
	// Load the CSI driver configuration.
	cdc, err := util.LoadCSIDriverConfig(v.setting)
	if err != nil {
		return err
	}

	// Iterate through the VM volumes.
	for _, volume := range vm.Spec.Template.Spec.Volumes {
		// Skip non-PVC volumes.
		if volume.PersistentVolumeClaim == nil {
			continue
		}

		pvcNamespace := vm.Namespace
		pvcName := volume.PersistentVolumeClaim.ClaimName

		pvc, err := v.pvcCache.Get(pvcNamespace, pvcName)
		if err != nil {
			return fmt.Errorf("failed to get PVC %s/%s: %w", pvcNamespace, pvcName, err)
		}

		// Validate both the ability and the CSI configuration.
		if err := webhookutil.ValidateProvisionerAndConfig(pvc, v.engineCache, v.scCache, newVMBackup.Spec.Type, cdc); err != nil {
			return err
		}
	}

	return nil
}

func (v *virtualMachineBackupValidator) checkBackupTarget() error {
	backupTargetSetting, err := v.setting.Get(settings.BackupTargetSettingName)
	if err != nil {
		return fmt.Errorf("can't get backup target setting, err: %w", err)
	}
	backupTarget, err := settings.DecodeBackupTarget(backupTargetSetting.Value)
	if err != nil {
		return fmt.Errorf("unmarshal backup target failed, value: %s, err: %w", backupTargetSetting.Value, err)
	}

	if backupTarget.IsDefaultBackupTarget() {
		return fmt.Errorf("backup target is not set")
	}

	return nil
}

func (v *virtualMachineBackupValidator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	newVMBackup := newObj.(*v1beta1.VirtualMachineBackup)
	oldVMBackup := oldObj.(*v1beta1.VirtualMachineBackup)

	oldAnnotations := oldVMBackup.GetAnnotations()
	newAnnotations := newVMBackup.GetAnnotations()

	if oldAnnotations[util.AnnotationSVMBackupID] != newAnnotations[util.AnnotationSVMBackupID] {
		return werror.NewBadRequest(fmt.Sprintf("annotation %s isn't allowed for updating", util.AnnotationSVMBackupID))
	}

	return nil
}

func (v *virtualMachineBackupValidator) Delete(_ *types.Request, obj runtime.Object) error {
	vmBackup := obj.(*v1beta1.VirtualMachineBackup)

	vmRestores, err := v.vmrestores.GetByIndex(indexeres.VMRestoreByVMBackupNamespaceAndName, fmt.Sprintf("%s-%s", vmBackup.Namespace, vmBackup.Name))
	if err != nil {
		return fmt.Errorf("can't get vmrestores from index %s with vmbackup %s/%s, err: %w", indexeres.VMRestoreByVMBackupNamespaceAndName, vmBackup.Namespace, vmBackup.Name, err)
	}
	for _, vmRestore := range vmRestores {
		if vmRestore.DeletionTimestamp != nil || vmRestore.Status == nil {
			continue
		}
		for _, cond := range vmRestore.Status.Conditions {
			// we use the same condition for backup and restore
			if cond.Type == v1beta1.BackupConditionProgressing {
				if cond.Status == corev1.ConditionTrue {
					return fmt.Errorf("vmrestore %s/%s is in progress", vmRestore.Namespace, vmRestore.Name)
				}
			}
		}
	}
	return nil
}

func (v *virtualMachineBackupValidator) checkTotalSnapshotSize(vm *kubevirtv1.VirtualMachine) error {
	resourceQuota, err := v.resourceQuotaCache.Get(vm.Namespace, util.DefaultResourceQuotaName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return werror.NewInternalError(fmt.Sprintf("failed to get resource quota %s/%s, err: %s", vm.Namespace, util.DefaultResourceQuotaName, err))
	}

	if resourceQuota.Spec.SnapshotLimit.VMTotalSnapshotSizeQuota != nil {
		if err := webhookutil.CheckTotalSnapshotSizeOnVM(v.pvcCache, v.engineCache, v.scCache, vm, resourceQuota.Spec.SnapshotLimit.VMTotalSnapshotSizeQuota[vm.Name]); err != nil {
			return err
		}
	}

	if err := webhookutil.CheckTotalSnapshotSizeOnNamespace(v.pvcCache, v.engineCache, v.scCache, vm.Namespace, resourceQuota.Spec.SnapshotLimit.NamespaceTotalSnapshotSizeQuota); err != nil {
		return err
	}
	return nil
}

func (v *virtualMachineBackupValidator) checkVMInstanceMigration(vm *kubevirtv1.VirtualMachine) error {
	srcVM := fmt.Sprintf("%s/%s", vm.Namespace, vm.Name)
	vmims, err := v.vmimCache.GetByIndex(indexeres.VMInstanceMigrationByVM, srcVM)
	if err != nil {
		return err
	}

	if len(vmims) == 0 {
		return nil
	}

	for _, vmim := range vmims {
		if !vmim.IsRunning() {
			continue
		}
		return fmt.Errorf("vm %s is in migration", srcVM)
	}
	return nil
}
