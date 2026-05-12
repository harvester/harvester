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
	"github.com/harvester/harvester/pkg/backup/common"
	ctlharvestercorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctlsnapshotv1 "github.com/harvester/harvester/pkg/generated/controllers/snapshot.storage.k8s.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
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
	vscCache ctlsnapshotv1.VolumeSnapshotClassCache,
	networkAttachmentDefinitionsCache ctlcniv1.NetworkAttachmentDefinitionCache,
) types.Validator {
	vmbo := common.GetVMBackupOperator(nil, vmBackup, vscCache, vms, nil, nil, nil, nil, nil)
	return &restoreValidator{
		vms:                               vms,
		setting:                           setting,
		vmBackup:                          vmBackup,
		vmRestore:                         vmRestore,
		svmbackup:                         svmbackup,
		vscCache:                          vscCache,
		networkAttachmentDefinitionsCache: networkAttachmentDefinitionsCache,

		vmrCalculator: resourcequota.NewCalculator(nss, pods, rqs, vmims, setting),
		vmbo:          vmbo,
		vmro:          restorecommon.GetVMRestoreOperator(nil, vmRestore, vms, nil, nil, nil, nil, nil, vmBackup, vmbo, nil),
	}
}

type restoreValidator struct {
	types.DefaultValidator

	vms                               ctlkubevirtv1.VirtualMachineCache
	setting                           ctlharvesterv1.SettingCache
	vmBackup                          ctlharvesterv1.VirtualMachineBackupCache
	vmRestore                         ctlharvesterv1.VirtualMachineRestoreCache
	svmbackup                         ctlharvesterv1.ScheduleVMBackupCache
	vscCache                          ctlsnapshotv1.VolumeSnapshotClassCache
	networkAttachmentDefinitionsCache ctlcniv1.NetworkAttachmentDefinitionCache

	vmrCalculator *resourcequota.Calculator
	vmbo          common.VMBackupOperator
	vmro          restorecommon.VMRestoreOperator
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
	vmr := newObj.(*v1beta1.VirtualMachineRestore)

	if errs := validationutil.IsDNS1123Subdomain(vmr.Spec.Target.Name); len(errs) != 0 {
		return werror.NewInvalidError(fmt.Sprintf("Target VM name is invalid, err: %v", errs), fieldTargetName)
	}

	vmb, err := v.getVMBackup(vmr)
	if err != nil {
		return werror.NewInvalidError(err.Error(), fieldVirtualMachineBackupName)
	}

	svmbackup := util.ResolveSVMBackupRef(v.svmbackup, vmb)
	if svmbackup != nil && !svmbackup.Spec.Suspend {
		return werror.NewInternalError(fmt.Sprintf("Source schedule %s/%s is running", svmbackup.Namespace, svmbackup.Name))
	}

	if err := v.checkVolumeSnapshotClass(vmb); err != nil {
		return werror.NewInvalidError(err.Error(), fieldVirtualMachineBackupName)
	}

	if err := v.checkNetwork(vmb); err != nil {
		return werror.NewInvalidError(err.Error(), fieldVirtualMachineBackupName)
	}

	if err := v.checkVMBackupType(vmr, vmb); err != nil {
		return err
	}

	return v.checkNewVMField(vmr, vmb)
}

func (v *restoreValidator) Update(_ *types.Request, oldObj, newObj runtime.Object) error {
	oldRestore := oldObj.(*v1beta1.VirtualMachineRestore)
	newRestore := newObj.(*v1beta1.VirtualMachineRestore)
	if !reflect.DeepEqual(oldRestore.Spec, newRestore.Spec) {
		return werror.NewInvalidError("VirtualMachineRestore spec is immutable", fieldSpec)
	}
	return nil
}

func (v *restoreValidator) checkNewVMField(vmr *v1beta1.VirtualMachineRestore, vmb *v1beta1.VirtualMachineBackup) error {
	targetNamespace := v.vmro.GetNamespace(vmr)
	targetName := v.vmro.GetTargetName(vmr)

	vm, err := v.vms.Get(targetNamespace, targetName)
	if err != nil && !apierrors.IsNotFound(err) {
		return werror.NewInternalError(fmt.Sprintf("failed to get the VM %s/%s, err: %+v", targetNamespace, targetName, err))
	}

	vmExists := err == nil && vm != nil

	if v.vmro.IsNewVM(vmr) {
		return v.validateNewVMRestore(vmExists, vm, vmr, vmb)
	}

	return v.validateExistingVMRestore(vmExists, vm, targetName)
}

func (v *restoreValidator) validateNewVMRestore(vmExists bool, vm *kubevirtv1.VirtualMachine, vmr *v1beta1.VirtualMachineRestore, vmb *v1beta1.VirtualMachineBackup) error {
	if vmExists {
		return werror.NewInvalidError(fmt.Sprintf("VM %s is already exists", vm.Name), fieldNewVM)
	}
	return v.handleNewVM(vmr, vmb)
}

func (v *restoreValidator) validateExistingVMRestore(vmExists bool, vm *kubevirtv1.VirtualMachine, targetName string) error {
	if !vmExists {
		return werror.NewInvalidError(fmt.Sprintf("can't replace nonexistent vm %s", targetName), fieldTargetName)
	}
	return v.handleExistVM(vm)
}

func (v *restoreValidator) handleExistVM(vm *kubevirtv1.VirtualMachine) error {
	if err := v.validateVMNotRunning(vm); err != nil {
		return err
	}

	if err := v.validateNoInProgressRestore(vm); err != nil {
		return err
	}

	return v.validateResourceQuota(vm)
}

func (v *restoreValidator) validateVMNotRunning(vm *kubevirtv1.VirtualMachine) error {
	if vm.Status.Ready {
		return werror.NewInvalidError(fmt.Sprintf("Please stop the VM %q before doing a restore", vm.Name), fieldTargetName)
	}
	return nil
}

func (v *restoreValidator) validateNoInProgressRestore(vm *kubevirtv1.VirtualMachine) error {
	hasInProgress, err := webhookutil.HasActiveRestore(v.vmRestore, v.vmro, vm.Namespace, vm.Name)
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("Failed to get the VM-related restores, err: %+v", err))
	}

	if hasInProgress {
		return werror.NewInvalidError(fmt.Sprintf("Please wait for the previous VM restore on the %s/%s to be complete first.", vm.Namespace, vm.Name), fieldTargetName)
	}

	return nil
}

func (v *restoreValidator) validateResourceQuota(vm *kubevirtv1.VirtualMachine) error {
	if err := v.vmrCalculator.CheckIfVMCanStartByResourceQuota(vm); err != nil {
		return werror.NewInternalError(fmt.Sprintf("Failed to restore the exist vm, err: %+v", err))
	}
	return nil
}

func (v *restoreValidator) handleNewVM(vmr *v1beta1.VirtualMachineRestore, vmb *v1beta1.VirtualMachineBackup) error {
	sourceSpec := v.vmbo.GetSourceSpec(vmb)
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: v.vmro.GetNamespace(vmr),
			Name:      v.vmro.GetTargetName(vmr),
		},
		Spec: sourceSpec.Spec,
	}
	// when the backup vm run strategy is Halt, the new vm should be Halt too, will skip the resource checking,
	// here it is set to RunStrategyRerunOnFailure for resource checking.
	runStrategy := kubevirtv1.RunStrategyRerunOnFailure
	vm.Spec.RunStrategy = &runStrategy
	if err := v.vmrCalculator.CheckIfVMCanStartByResourceQuota(vm); err != nil {
		return werror.NewInternalError(fmt.Sprintf("Failed to restore the new vm, err: %+v", err))
	}

	if !v.vmro.IsKeepMacAddress(vmr) {
		return nil
	}

	// we don't have a global macaddress checker,
	// so we just check the source vm name and vmrestores which using the same vmbackup.
	// this can be removed after https://github.com/harvester/harvester/issues/4893
	otherVMRestores, err := v.vmRestore.GetByIndex(indexeres.VMRestoreByVMBackupNamespaceAndName, fmt.Sprintf("%s-%s", v.vmro.GetVMBackupNamespace(vmr), v.vmro.GetVMBackupName(vmr)))
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get vmrestore by index %s, err: %+v", indexeres.VMRestoreByVMBackupNamespaceAndName, err))
	}
	for _, otherVMRestore := range otherVMRestores {
		if v.vmro.IsNewVM(otherVMRestore) && v.vmro.IsKeepMacAddress(otherVMRestore) {
			return werror.NewInvalidError(fmt.Sprintf("can't restore the new vm with the same macaddress as the vmrestore %s/%s", v.vmro.GetNamespace(otherVMRestore), v.vmro.GetName(otherVMRestore)), fieldKeepMacAddress)
		}
	}

	sourceVM, err := v.vms.Get(v.vmbo.GetNamespace(vmb), v.vmbo.GetSourceName(vmb))
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get the VM %s/%s, err: %+v", v.vmbo.GetNamespace(vmb), v.vmbo.GetSourceName(vmb), err))
	}

	status := v.vmbo.GetStatus(vmb)
	if sourceVM != nil && status != nil && status.SourceUID != nil && sourceVM.UID == *status.SourceUID {
		return werror.NewInvalidError("can't restore the new vm with the same macaddress because the source vm is still existent", fieldKeepMacAddress)
	}

	return nil
}

func (v *restoreValidator) getVMBackup(vmr *v1beta1.VirtualMachineRestore) (*v1beta1.VirtualMachineBackup, error) {
	vmb, err := v.vmro.GetVMBackup(vmr)
	if err != nil {
		return nil, fmt.Errorf("can't get vmbackup %s/%s, err: %w", vmr.Spec.VirtualMachineBackupNamespace, vmr.Spec.VirtualMachineBackupName, err)
	}

	if v.vmbo.GetDeletionTimestap(vmb) != nil {
		return nil, fmt.Errorf("vmbackup %s/%s is deleted", v.vmbo.GetNamespace(vmb), v.vmbo.GetName(vmb))
	}

	return vmb, nil
}

func (v *restoreValidator) checkSnapshot(vmr *v1beta1.VirtualMachineRestore, vmb *v1beta1.VirtualMachineBackup) error {
	if v.vmro.GetNamespace(vmr) != v.vmbo.GetNamespace(vmb) {
		return fmt.Errorf("restore to other namespace with backup type snapshot is not supported")
	}
	if !v.vmro.IsNewVM(vmr) && !v.vmro.IsRetainPolicy(vmr) {
		// We don't allow users to use "delete" policy for replacing a VM when the backup type is snapshot.
		// This will also remove the VMBackup when VMRestore is finished.
		return fmt.Errorf("delete policy with backup type snapshot for replacing VM is not supported")
	}
	return nil
}

func (v *restoreValidator) checkNetwork(vmBackup *v1beta1.VirtualMachineBackup) error {
	sourceSpec := v.vmbo.GetSourceSpec(vmBackup)
	for _, network := range sourceSpec.Spec.Template.Spec.Networks {
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

func (v *restoreValidator) checkVMBackupType(vmr *v1beta1.VirtualMachineRestore, vmb *v1beta1.VirtualMachineBackup) error {
	var err error
	backupType := v.vmbo.GetType(vmb)
	switch backupType {
	case v1beta1.Backup:
		err = v.checkBackup(vmr, vmb)
		if err == nil {
			// Because of the misleading items https://github.com/harvester/harvester/issues/7755#issue-2896409886,
			// User may have VMBackups with non-LH source volume. We should prevent this VMBackup from restoring
			err = webhookutil.IsLHBackupRelated(vmb, v.vmbo)
		}
	case v1beta1.Snapshot:
		err = v.checkSnapshot(vmr, vmb)
	}
	if err != nil {
		return werror.NewInvalidError(err.Error(), fieldVirtualMachineBackupName)
	}
	return nil
}

func (v *restoreValidator) checkBackup(vmr *v1beta1.VirtualMachineRestore, vmb *v1beta1.VirtualMachineBackup) error {
	if err := v.checkBackupTarget(vmb); err != nil {
		return err
	}

	if v.vmro.IsNewVMOrHasRetainPolicy(vmr) {
		return nil
	}

	// if deletion policy is delete, check whether there is snapshot using same pvc
	vbs := v.vmbo.GetVolBackups(vmb)
	for _, vb := range vbs {
		pvcNamespaceAndName := fmt.Sprintf("%s/%s", v.vmbo.GetVolBackupPVCNameSpace(&vb), v.vmbo.GetVolBackupPVCName(&vb))
		vss, err := v.vmBackup.GetByIndex(indexeres.VMBackupSnapshotByPVCNamespaceAndName, pvcNamespaceAndName)
		if err != nil {
			return err
		}

		var vsNames []string
		for _, vs := range vss {
			vsNames = append(vsNames, v.vmbo.GetName(vs))
		}

		if len(vsNames) != 0 {
			return fmt.Errorf("can't use delete policy, the volume %q is used by the VM Snapshot(s) %s", pvcNamespaceAndName, strings.Join(vsNames, ", "))
		}
	}
	return nil
}

func (v *restoreValidator) checkBackupTarget(vmb *v1beta1.VirtualMachineBackup) error {
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

	if !backuputil.IsBackupTargetSame(v.vmbo.GetBackupTarget(vmb), backupTarget) {
		return fmt.Errorf("backup target %+v is not matched in vmBackup %s/%s", backupTarget, v.vmbo.GetNamespace(vmb), v.vmbo.GetName(vmb))
	}

	return nil
}

func (v *restoreValidator) checkVolumeSnapshotClass(vmBackup *v1beta1.VirtualMachineBackup) error {
	for csiDriverName, vscName := range v.vmbo.GetCSIDriverVSCNames(vmBackup) {
		if _, err := v.vscCache.Get(vscName); err != nil {
			return fmt.Errorf("can't get volumeSnapshotClass %s for driver %s", vscName, csiDriverName)
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
