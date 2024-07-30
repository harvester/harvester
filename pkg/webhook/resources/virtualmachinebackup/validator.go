package virtualmachinebackup

import (
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/indexeres"
	"github.com/harvester/harvester/pkg/webhook/types"
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
) types.Validator {
	return &virtualMachineBackupValidator{
		vms:        vms,
		setting:    setting,
		vmrestores: vmrestores,
		pvcCache:   pvcCache,
	}
}

type virtualMachineBackupValidator struct {
	types.DefaultValidator

	vms        ctlkubevirtv1.VirtualMachineCache
	setting    ctlharvesterv1.SettingCache
	vmrestores ctlharvesterv1.VirtualMachineRestoreCache
	pvcCache   ctlcorev1.PersistentVolumeClaimCache
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
			admissionregv1.Delete,
		},
	}
}

func (v *virtualMachineBackupValidator) Create(_ *types.Request, newObj runtime.Object) error {
	newVMBackup := newObj.(*v1beta1.VirtualMachineBackup)

	if newVMBackup.Spec.Source.Name == "" {
		return werror.NewInvalidError("source VM name is empty", fieldSourceName)
	}

	var err error

	// If VMBackup is from metadata in backup target, we don't check whether the VM is existent,
	// because the related VM may not exist in a new cluster.
	if newVMBackup.Status == nil {
		vm, err := v.vms.Get(newVMBackup.Namespace, newVMBackup.Spec.Source.Name)
		if err != nil {
			return werror.NewInvalidError(err.Error(), fieldSourceName)
		}
		if err = v.checkBackupVolumeSnapshotClass(vm, newVMBackup); err != nil {
			return werror.NewInvalidError(err.Error(), fieldSourceName)
		}
	}

	if newVMBackup.Spec.Type == v1beta1.Backup {
		err = v.checkBackupTarget()
	}
	if err != nil {
		return werror.NewInvalidError(err.Error(), fieldTypeName)
	}

	return nil
}

// checkBackupVolumeSnapshotClass checks if the volumeSnapshotClassName is configured for the provisioner used by the PVCs in the VirtualMachine.
func (v *virtualMachineBackupValidator) checkBackupVolumeSnapshotClass(vm *kubevirtv1.VirtualMachine, newVMBackup *v1beta1.VirtualMachineBackup) error {
	csiDriverConfig, err := util.LoadCSIDriverConfig(v.setting)
	if err != nil {
		return err
	}

	for _, volume := range vm.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}

		pvcNamespace := vm.Namespace
		pvcName := volume.PersistentVolumeClaim.ClaimName

		pvc, err := v.pvcCache.Get(pvcNamespace, pvcName)
		if err != nil {
			return fmt.Errorf("failed to get PVC %s/%s, err: %w", pvcNamespace, pvcName, err)
		}

		// Get the provisioner used by the PVC and find its configuration in the CSI driver configuration.
		provisioner := util.GetProvisionedPVCProvisioner(pvc)
		c, ok := csiDriverConfig[provisioner]
		if !ok {
			return fmt.Errorf("provisioner %s is not configured in the %s setting", provisioner, settings.CSIDriverConfigSettingName)
		}

		// Determine which configuration value is required based on the type of backup.
		var requiredValue string
		switch newVMBackup.Spec.Type {
		case v1beta1.Backup:
			requiredValue = c.BackupVolumeSnapshotClassName
		case v1beta1.Snapshot:
			requiredValue = c.VolumeSnapshotClassName
		}

		// If the required value is missing, return an error.
		if requiredValue == "" {
			return fmt.Errorf("%s's %s is not configured for provisioner %s in the %s setting",
				newVMBackup.Spec.Type, "VolumeSnapshotClassName", provisioner, settings.CSIDriverConfigSettingName)
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
