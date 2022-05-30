package restore

import (
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlbackup "github.com/harvester/harvester/pkg/controller/master/backup"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	fieldTargetName               = "spec.target.name"
	fieldVirtualMachineBackupName = "spec.virtualMachineBackupName"
	fieldNewVM                    = "spec.newVM"
)

func NewValidator(
	vms ctlkubevirtv1.VirtualMachineCache,
	setting ctlharvesterv1.SettingCache,
	vmBackup ctlharvesterv1.VirtualMachineBackupCache,
	snapshotClass ctlsnapshotv1.VolumeSnapshotClassCache,
) types.Validator {
	return &restoreValidator{
		vms:           vms,
		setting:       setting,
		vmBackup:      vmBackup,
		snapshotClass: snapshotClass,
	}
}

type restoreValidator struct {
	types.DefaultValidator

	vms           ctlkubevirtv1.VirtualMachineCache
	setting       ctlharvesterv1.SettingCache
	vmBackup      ctlharvesterv1.VirtualMachineBackupCache
	snapshotClass ctlsnapshotv1.VolumeSnapshotClassCache
}

func (v *restoreValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.VirtualMachineRestoreResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.VirtualMachineRestore{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}

func (v *restoreValidator) Create(request *types.Request, newObj runtime.Object) error {
	newRestore := newObj.(*v1beta1.VirtualMachineRestore)

	targetVM := newRestore.Spec.Target.Name
	newVM := newRestore.Spec.NewVM

	if targetVM == "" {
		return werror.NewInvalidError("target VM name is empty", fieldTargetName)
	}

	vmBackup, err := v.getVmBackup(newRestore)
	if err != nil {
		return werror.NewInvalidError(err.Error(), fieldVirtualMachineBackupName)
	}

	if err := v.checkBackupTarget(vmBackup); err != nil {
		return werror.NewInvalidError(err.Error(), fieldVirtualMachineBackupName)
	}

	if err := v.checkVolumeSnapshotClass(vmBackup); err != nil {
		return werror.NewInvalidError(err.Error(), fieldVirtualMachineBackupName)
	}

	vm, err := v.vms.Get(newRestore.Namespace, targetVM)
	if err != nil {
		if newVM && apierrors.IsNotFound(err) {
			return nil
		}
		return werror.NewInvalidError(err.Error(), fieldTargetName)
	}

	// restore a new vm but the vm is already exist
	if newVM && vm != nil {
		return werror.NewInvalidError(fmt.Sprintf("VM %s is already exists", vm.Name), fieldNewVM)
	}

	// restore an existing vm but the vm is still running
	if !newVM && vm.Status.Ready {
		return werror.NewInvalidError(fmt.Sprintf("please stop the VM %q before doing a restore", vm.Name), fieldTargetName)
	}

	return nil
}

func (v *restoreValidator) getVmBackup(vmRestore *v1beta1.VirtualMachineRestore) (*v1beta1.VirtualMachineBackup, error) {
	vmBackup, err := v.vmBackup.Get(vmRestore.Spec.VirtualMachineBackupNamespace, vmRestore.Spec.VirtualMachineBackupName)
	if err != nil {
		return nil, fmt.Errorf("can't get vmbackup %s/%s, err: %w", vmRestore.Spec.VirtualMachineBackupNamespace, vmRestore.Spec.VirtualMachineBackupName, err)
	}

	if vmBackup.DeletionTimestamp != nil {
		return nil, fmt.Errorf("vmbackup %s/%s is deleted", vmBackup.Namespace, vmBackup.Name)
	}

	if !ctlbackup.IsBackupReady(vmBackup) {
		return nil, fmt.Errorf("VMBackup %s/%s is not ready", vmBackup.Namespace, vmBackup.Name)
	}

	return vmBackup, nil
}

func (v *restoreValidator) checkBackupTarget(vmBackup *v1beta1.VirtualMachineBackup) error {
	if vmBackup.Spec.Type == v1beta1.Snapshot {
		return nil
	}

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

	if !ctlbackup.IsBackupTargetSame(vmBackup.Status.BackupTarget, backupTarget) {
		return fmt.Errorf("backup target %+v is not matched in vmBackup %s/%s", backupTarget, vmBackup.Namespace, vmBackup.Name)
	}

	return nil
}

func (v *restoreValidator) checkVolumeSnapshotClass(vmBackup *v1beta1.VirtualMachineBackup) error {
	for csiDriverName, volumeSnapshotClassName := range vmBackup.Status.CSIDriverVolumeSnapshotClassNameMap {
		_, err := v.snapshotClass.Get(volumeSnapshotClassName)
		if err != nil {
			return fmt.Errorf("can't get volumeSnapshotClass %s for driver %s", volumeSnapshotClassName, csiDriverName)
		}
	}
	return nil
}
