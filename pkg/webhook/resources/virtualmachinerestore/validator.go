package virtualmachinerestore

import (
	"fmt"
	"reflect"

	ctlv1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	validationutil "k8s.io/apimachinery/pkg/util/validation"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlbackup "github.com/harvester/harvester/pkg/controller/master/backup"
	ctlharvestercorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util/resourcequota"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
	webhookutil "github.com/harvester/harvester/pkg/webhook/util"
)

const (
	fieldSpec                     = "spec"
	fieldTargetName               = "spec.target.name"
	fieldVirtualMachineBackupName = "spec.virtualMachineBackupName"
	fieldNewVM                    = "spec.newVM"
)

func NewValidator(
	nss ctlv1.NamespaceCache,
	pods ctlv1.PodCache,
	rqs ctlharvestercorev1.ResourceQuotaCache,
	vms ctlkubevirtv1.VirtualMachineCache,
	setting ctlharvesterv1.SettingCache,
	vmBackup ctlharvesterv1.VirtualMachineBackupCache,
	vmRestore ctlharvesterv1.VirtualMachineRestoreCache,
	vmims ctlkubevirtv1.VirtualMachineInstanceMigrationCache,
	snapshotClass ctlsnapshotv1.VolumeSnapshotClassCache,
) types.Validator {
	return &restoreValidator{
		vms:           vms,
		setting:       setting,
		vmBackup:      vmBackup,
		vmRestore:     vmRestore,
		snapshotClass: snapshotClass,

		vmrCalculator: resourcequota.NewCalculator(nss, pods, rqs, vmims),
	}
}

type restoreValidator struct {
	types.DefaultValidator

	vms           ctlkubevirtv1.VirtualMachineCache
	setting       ctlharvesterv1.SettingCache
	vmBackup      ctlharvesterv1.VirtualMachineBackupCache
	vmRestore     ctlharvesterv1.VirtualMachineRestoreCache
	snapshotClass ctlsnapshotv1.VolumeSnapshotClassCache

	vmrCalculator *resourcequota.Calculator
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
			admissionregv1.Update,
		},
	}
}

func (v *restoreValidator) Create(request *types.Request, newObj runtime.Object) error {
	newRestore := newObj.(*v1beta1.VirtualMachineRestore)

	targetVM := newRestore.Spec.Target.Name
	newVM := newRestore.Spec.NewVM

	if errs := validationutil.IsDNS1123Subdomain(targetVM); len(errs) != 0 {
		return werror.NewInvalidError(fmt.Sprintf("Target VM name is invalid, err: %v", errs), fieldTargetName)
	}

	vmBackup, err := v.getVMBackup(newRestore)
	if err != nil {
		return werror.NewInvalidError(err.Error(), fieldVirtualMachineBackupName)
	}

	if err := v.checkVolumeSnapshotClass(vmBackup); err != nil {
		return werror.NewInvalidError(err.Error(), fieldVirtualMachineBackupName)
	}

	if vmBackup.Spec.Type == v1beta1.Backup {
		err = v.checkBackupTarget(vmBackup)
	} else {
		err = v.checkSnapshot(newRestore, vmBackup)
	}
	if err != nil {
		return werror.NewInvalidError(err.Error(), fieldVirtualMachineBackupName)
	}

	vm, err := v.vms.Get(newRestore.Namespace, targetVM)
	if err != nil {
		if newVM && apierrors.IsNotFound(err) {
			return v.handleNewVM(newRestore, targetVM, vmBackup)
		}
		return werror.NewInvalidError(err.Error(), fieldTargetName)
	}

	// restore a new vm but the vm is already exist
	if newVM && vm != nil {
		return werror.NewInvalidError(fmt.Sprintf("VM %s is already exists", vm.Name), fieldNewVM)
	}

	return v.handleExistVM(newVM, vm)
}

func (v *restoreValidator) Update(request *types.Request, oldObj, newObj runtime.Object) error {
	oldRestore := oldObj.(*v1beta1.VirtualMachineRestore)
	newRestore := newObj.(*v1beta1.VirtualMachineRestore)
	if !reflect.DeepEqual(oldRestore.Spec, newRestore.Spec) {
		return werror.NewInvalidError("VirtualMachineRestore spec is immutable", fieldSpec)
	}
	return nil
}

func (v *restoreValidator) handleExistVM(newVM bool, vm *kubevirtv1.VirtualMachine) error {
	// restore an existing vm but the vm is still running
	if !newVM && vm.Status.Ready {
		return werror.NewInvalidError(fmt.Sprintf("Please stop the VM %q before doing a restore", vm.Name), fieldTargetName)
	}

	if result, err := webhookutil.HasInProgressingVMRestoreOnSameTarget(v.vmRestore, vm.Namespace, vm.Name); err != nil {
		return werror.NewInternalError(fmt.Sprintf("Failed to get the VM-related restores, err: %+v", err))
	} else if result {
		return werror.NewInvalidError(fmt.Sprintf("Please wait for the previous VM restore on the %s/%s to be complete first.", vm.Namespace, vm.Name), fieldTargetName)
	}

	if err := v.vmrCalculator.CheckIfVMCanStartByResourceQuota(vm); err != nil {
		return werror.NewInternalError(fmt.Sprintf("Failed to restore the exist vm, err: %+v", err))
	}

	return nil
}

func (v *restoreValidator) handleNewVM(newRestore *v1beta1.VirtualMachineRestore, targetVM string, vmBackup *v1beta1.VirtualMachineBackup) error {
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: newRestore.Namespace,
			Name:      targetVM,
		},
		Spec: vmBackup.Status.SourceSpec.Spec,
	}
	// when the backup vm run strategy is Halt, the new vm should be Halt too, will skip the resource checking,
	// here it is set to RunStrategyRerunOnFailure for resource checking.
	runStrategy := kubevirtv1.RunStrategyRerunOnFailure
	vm.Spec.RunStrategy = &runStrategy
	if err := v.vmrCalculator.CheckIfVMCanStartByResourceQuota(vm); err != nil {
		return werror.NewInternalError(fmt.Sprintf("Failed to restore the new vm, err: %+v", err))
	}
	return nil
}

func (v *restoreValidator) getVMBackup(vmRestore *v1beta1.VirtualMachineRestore) (*v1beta1.VirtualMachineBackup, error) {
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

func (v *restoreValidator) checkSnapshot(vmRestore *v1beta1.VirtualMachineRestore, vmBackup *v1beta1.VirtualMachineBackup) error {
	if vmRestore.Namespace != vmBackup.Namespace {
		return fmt.Errorf("Restore to other namespace with backup type snapshot is not supported")
	}
	if !vmRestore.Spec.NewVM && vmRestore.Spec.DeletionPolicy != v1beta1.VirtualMachineRestoreRetain {
		// We don't allow users to use "delete" policy for replacing a VM when the backup type is snapshot.
		// This will also remove the VMBackup when VMRestore is finished.
		return fmt.Errorf("Delete policy with backup type snapshot for replacing VM is not supported")
	}
	return nil
}

func (v *restoreValidator) checkBackupTarget(vmBackup *v1beta1.VirtualMachineBackup) error {
	backupTargetSetting, err := v.setting.Get(settings.BackupTargetSettingName)
	if err != nil {
		return fmt.Errorf("Can't get backup target setting, err: %w", err)
	}
	backupTarget, err := settings.DecodeBackupTarget(backupTargetSetting.Value)
	if err != nil {
		return fmt.Errorf("Unmarshal backup target failed, value: %s, err: %w", backupTargetSetting.Value, err)
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
	for csiDriverName, volumeSnapshotClassName := range vmBackup.Status.CSIDriverVolumeSnapshotClassNames {
		_, err := v.snapshotClass.Get(volumeSnapshotClassName)
		if err != nil {
			return fmt.Errorf("can't get volumeSnapshotClass %s for driver %s", volumeSnapshotClassName, csiDriverName)
		}
	}
	return nil
}
