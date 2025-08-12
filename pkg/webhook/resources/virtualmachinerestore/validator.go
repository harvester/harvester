package virtualmachinerestore

import (
	"fmt"
	"reflect"
	"strings"

	ctlv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
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
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
	"github.com/harvester/harvester/pkg/util/resourcequota"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/indexeres"
	"github.com/harvester/harvester/pkg/webhook/types"
	webhookutil "github.com/harvester/harvester/pkg/webhook/util"
)

const (
	fieldSpec                     = "spec"
	fieldTargetName               = "spec.target.name"
	fieldVirtualMachineBackupName = "spec.virtualMachineBackupName"
	fieldNewVM                    = "spec.newVM"
	fieldKeepMacAddress           = "spec.keepMacAddress"
)

func NewValidator(
	nss ctlv1.NamespaceCache,
	pods ctlv1.PodCache,
	rqs ctlharvestercorev1.ResourceQuotaCache,
	vms ctlkubevirtv1.VirtualMachineCache,
	setting ctlharvesterv1.SettingCache,
	vmBackup ctlharvesterv1.VirtualMachineBackupCache,
	vmRestore ctlharvesterv1.VirtualMachineRestoreCache,
	svmbackup ctlharvesterv1.ScheduleVMBackupCache,
	vmims ctlkubevirtv1.VirtualMachineInstanceMigrationCache,
	snapshotClass ctlsnapshotv1.VolumeSnapshotClassCache,
	networkAttachmentDefinitionsCache ctlcniv1.NetworkAttachmentDefinitionCache,
) types.Validator {
	return &restoreValidator{
		vms:                               vms,
		setting:                           setting,
		vmBackup:                          vmBackup,
		vmRestore:                         vmRestore,
		svmbackup:                         svmbackup,
		snapshotClass:                     snapshotClass,
		networkAttachmentDefinitionsCache: networkAttachmentDefinitionsCache,

		vmrCalculator: resourcequota.NewCalculator(nss, pods, rqs, vmims, setting),
	}
}

type restoreValidator struct {
	types.DefaultValidator

	vms                               ctlkubevirtv1.VirtualMachineCache
	setting                           ctlharvesterv1.SettingCache
	vmBackup                          ctlharvesterv1.VirtualMachineBackupCache
	vmRestore                         ctlharvesterv1.VirtualMachineRestoreCache
	svmbackup                         ctlharvesterv1.ScheduleVMBackupCache
	snapshotClass                     ctlsnapshotv1.VolumeSnapshotClassCache
	networkAttachmentDefinitionsCache ctlcniv1.NetworkAttachmentDefinitionCache

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
			admissionregv1.Delete,
		},
	}
}

func (v *restoreValidator) Create(_ *types.Request, newObj runtime.Object) error {
	newRestore := newObj.(*v1beta1.VirtualMachineRestore)

	if errs := validationutil.IsDNS1123Subdomain(newRestore.Spec.Target.Name); len(errs) != 0 {
		return werror.NewInvalidError(fmt.Sprintf("Target VM name is invalid, err: %v", errs), fieldTargetName)
	}

	vmBackup, err := v.getVMBackup(newRestore)
	if err != nil {
		return werror.NewInvalidError(err.Error(), fieldVirtualMachineBackupName)
	}

	svmbackup := util.ResolveSVMBackupRef(v.svmbackup, vmBackup)
	if svmbackup != nil && !svmbackup.Spec.Suspend {
		return werror.NewInternalError(fmt.Sprintf("Source schedule %s/%s is running", svmbackup.Namespace, svmbackup.Name))
	}

	if err := v.checkVolumeSnapshotClass(vmBackup); err != nil {
		return werror.NewInvalidError(err.Error(), fieldVirtualMachineBackupName)
	}

	if err := v.checkNetwork(vmBackup); err != nil {
		return werror.NewInvalidError(err.Error(), fieldVirtualMachineBackupName)
	}

	if err := v.checkVMBackupType(newRestore, vmBackup); err != nil {
		return err
	}

	return v.checkNewVMField(newRestore, vmBackup)
}

func (v *restoreValidator) Update(_ *types.Request, oldObj, newObj runtime.Object) error {
	oldRestore := oldObj.(*v1beta1.VirtualMachineRestore)
	newRestore := newObj.(*v1beta1.VirtualMachineRestore)
	if !reflect.DeepEqual(oldRestore.Spec, newRestore.Spec) {
		return werror.NewInvalidError("VirtualMachineRestore spec is immutable", fieldSpec)
	}
	return nil
}

func (v *restoreValidator) checkNewVMField(vmRestore *v1beta1.VirtualMachineRestore, vmBackup *v1beta1.VirtualMachineBackup) error {
	vm, err := v.vms.Get(vmRestore.Namespace, vmRestore.Spec.Target.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		return werror.NewInternalError(fmt.Sprintf("failed to get the VM %s/%s, err: %+v", vmRestore.Namespace, vmRestore.Spec.Target.Name, err))
	}
	vmNotFound := apierrors.IsNotFound(err)

	switch vmRestore.Spec.NewVM {
	case true:
		// restore a new vm but the vm is already exist
		if vm != nil {
			return werror.NewInvalidError(fmt.Sprintf("VM %s is already exists", vm.Name), fieldNewVM)
		}
		return v.handleNewVM(vmRestore, vmBackup)
	case false:
		// replace an existing vm but there is no related vm
		if vmNotFound {
			return werror.NewInvalidError(fmt.Sprintf("can't replace nonexistent vm %s", vmRestore.Spec.Target.Name), fieldTargetName)
		}
		return v.handleExistVM(vm)
	}
	return nil
}

func (v *restoreValidator) handleExistVM(vm *kubevirtv1.VirtualMachine) error {
	// restore an existing vm but the vm is still running
	if vm.Status.Ready {
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

func (v *restoreValidator) handleNewVM(vmRestore *v1beta1.VirtualMachineRestore, vmBackup *v1beta1.VirtualMachineBackup) error {
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: vmRestore.Namespace,
			Name:      vmRestore.Spec.Target.Name,
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

	if !vmRestore.Spec.KeepMacAddress {
		return nil
	}

	// we don't have a global macaddress checker,
	// so we just check the source vm name and vmrestores which using the same vmbackup.
	// this can be removed after https://github.com/harvester/harvester/issues/4893
	otherVMRestores, err := v.vmRestore.GetByIndex(indexeres.VMRestoreByVMBackupNamespaceAndName, fmt.Sprintf("%s-%s", vmRestore.Spec.VirtualMachineBackupNamespace, vmRestore.Spec.VirtualMachineBackupName))
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get vmrestore by index %s, err: %+v", indexeres.VMRestoreByVMBackupNamespaceAndName, err))
	}
	for _, otherVMRestore := range otherVMRestores {
		if otherVMRestore.Spec.NewVM && otherVMRestore.Spec.KeepMacAddress {
			return werror.NewInvalidError(fmt.Sprintf("can't restore the new vm with the same macaddress as the vmrestore %s/%s", otherVMRestore.Namespace, otherVMRestore.Name), fieldKeepMacAddress)
		}
	}
	sourceVM, err := v.vms.Get(vmBackup.Namespace, vmBackup.Spec.Source.Name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get the VM %s/%s, err: %+v", vmBackup.Namespace, vmBackup.Spec.Source.Name, err))
	}
	if sourceVM != nil && vmBackup.Status.SourceUID != nil && sourceVM.UID == *vmBackup.Status.SourceUID {
		return werror.NewInvalidError("can't restore the new vm with the same macaddress because the source vm is still existent", fieldKeepMacAddress)
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
		return fmt.Errorf("restore to other namespace with backup type snapshot is not supported")
	}
	if !vmRestore.Spec.NewVM && vmRestore.Spec.DeletionPolicy != v1beta1.VirtualMachineRestoreRetain {
		// We don't allow users to use "delete" policy for replacing a VM when the backup type is snapshot.
		// This will also remove the VMBackup when VMRestore is finished.
		return fmt.Errorf("delete policy with backup type snapshot for replacing VM is not supported")
	}
	return nil
}

func (v *restoreValidator) checkNetwork(vmBackup *v1beta1.VirtualMachineBackup) error {
	for _, network := range vmBackup.Status.SourceSpec.Spec.Template.Spec.Networks {
		if network.Multus != nil {
			namespace, name := ref.Parse(network.Multus.NetworkName)
			_, err := v.networkAttachmentDefinitionsCache.Get(namespace, name)
			if err != nil {
				return fmt.Errorf("failed to get network attachment definition %s, err: %v", network.Multus.NetworkName, err)
			}
		}
	}
	return nil
}

func (v *restoreValidator) checkVMBackupType(vmRestore *v1beta1.VirtualMachineRestore, vmBackup *v1beta1.VirtualMachineBackup) error {
	var err error
	switch vmBackup.Spec.Type {
	case v1beta1.Backup:
		err = v.checkBackup(vmRestore, vmBackup)
		if err == nil {
			// Because of the misleading items https://github.com/harvester/harvester/issues/7755#issue-2896409886,
			// User may have VMBackups with non-LH source volume. We should prevent this VMBackup from restoring
			err = webhookutil.IsLHBackupRelated(vmBackup)
		}
	case v1beta1.Snapshot:
		err = v.checkSnapshot(vmRestore, vmBackup)
	}
	if err != nil {
		return werror.NewInvalidError(err.Error(), fieldVirtualMachineBackupName)
	}
	return nil
}

func (v *restoreValidator) checkBackup(vmRestore *v1beta1.VirtualMachineRestore, vmBackup *v1beta1.VirtualMachineBackup) error {
	if err := v.checkBackupTarget(vmBackup); err != nil {
		return err
	}

	if ctlbackup.IsNewVMOrHasRetainPolicy(vmRestore) {
		return nil
	}

	// if deletion policy is delete, check whether there is snapshot using same pvc
	for _, volumeBackup := range vmBackup.Status.VolumeBackups {
		pvcNamespaceAndName := fmt.Sprintf("%s/%s", volumeBackup.PersistentVolumeClaim.ObjectMeta.Namespace, volumeBackup.PersistentVolumeClaim.ObjectMeta.Name)
		vmBackupSnapshots, err := v.vmBackup.GetByIndex(indexeres.VMBackupSnapshotByPVCNamespaceAndName, pvcNamespaceAndName)
		if err != nil {
			return err
		}

		var vmBackupSnapshotNames []string
		for _, vmBackupSnapshot := range vmBackupSnapshots {
			vmBackupSnapshotNames = append(vmBackupSnapshotNames, vmBackupSnapshot.Name)
		}

		if len(vmBackupSnapshotNames) != 0 {
			return fmt.Errorf("can't use delete policy, the volume %q is used by the VM Snapshot(s) %s", pvcNamespaceAndName, strings.Join(vmBackupSnapshotNames, ", "))
		}
	}
	return nil
}

func (v *restoreValidator) checkBackupTarget(vmBackup *v1beta1.VirtualMachineBackup) error {
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

	if !backuputil.IsBackupTargetSame(vmBackup.Status.BackupTarget, backupTarget) {
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

func (v *restoreValidator) Delete(_ *types.Request, newObj runtime.Object) error {
	vmRestore := newObj.(*v1beta1.VirtualMachineRestore)
	vm, err := v.vms.Get(vmRestore.Namespace, vmRestore.Spec.Target.Name)
	if apierrors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return werror.NewInvalidError(err.Error(), fieldTargetName)
	}

	if vm.DeletionTimestamp == nil {
		return werror.NewInvalidError(fmt.Sprintf("The restore can't be removed because the restored VM %s exists", vm.Name), fieldTargetName)
	}

	return nil
}
