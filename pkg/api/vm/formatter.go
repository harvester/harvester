package vm

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/data/convert"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/controller/master/migration"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/harvester/pkg/util"
)

const (
	startVM        = "start"
	stopVM         = "stop"
	restartVM      = "restart"
	softReboot     = "softreboot"
	pauseVM        = "pause"
	unpauseVM      = "unpause"
	ejectCdRom     = "ejectCdRom"
	migrate        = "migrate"
	abortMigration = "abortMigration"
	backupVM       = "backup"
	restoreVM      = "restore"
	createTemplate = "createTemplate"
	addVolume      = "addVolume"
	removeVolume   = "removeVolume"
	cloneVM        = "clone"
)

type vmformatter struct {
	vmiCache      ctlkubevirtv1.VirtualMachineInstanceCache
	vmBackupCache ctlharvesterv1.VirtualMachineBackupCache
}

func (vf *vmformatter) formatter(request *types.APIRequest, resource *types.RawResource) {
	// reset resource actions, because action map already be set when add actions handler,
	// but current framework can't support use formatter to remove key from action map
	resource.Actions = make(map[string]string, 1)
	if request.AccessControl.CanUpdate(request, resource.APIObject, resource.Schema) != nil {
		return
	}

	vm := &kubevirtv1.VirtualMachine{}
	err := convert.ToObj(resource.APIObject.Data(), vm)
	if err != nil {
		return
	}

	resource.AddAction(request, addVolume)
	resource.AddAction(request, removeVolume)
	resource.AddAction(request, cloneVM)

	if canEjectCdRom(vm) {
		resource.AddAction(request, ejectCdRom)
	}

	vmi := vf.getVMI(vm)
	if vf.canStart(vm, vmi) {
		resource.AddAction(request, startVM)
	}

	if vf.canStop(vm, vmi) {
		resource.AddAction(request, stopVM)
	}

	if vf.canRestart(vm, vmi) {
		resource.AddAction(request, restartVM)
	}

	if vf.canSoftReboot(vmi) {
		resource.AddAction(request, softReboot)
	}

	if vf.canPause(vmi) {
		resource.AddAction(request, pauseVM)
	}

	if vf.canUnPause(vmi) {
		resource.AddAction(request, unpauseVM)
	}

	if canMigrate(vmi) {
		resource.AddAction(request, migrate)
	}

	if canAbortMigrate(vmi) {
		resource.AddAction(request, abortMigration)
	}

	if vf.canDoBackup(vm, vmi) {
		resource.AddAction(request, backupVM)
	}

	if vf.canDoRestore(vm, vmi) {
		resource.AddAction(request, restoreVM)
	}

	if vf.canCreateTemplate(vmi) {
		resource.AddAction(request, createTemplate)
	}
}

func canEjectCdRom(vm *kubevirtv1.VirtualMachine) bool {
	if !vmReady.IsTrue(vm) {
		return false
	}

	for _, disk := range vm.Spec.Template.Spec.Domain.Devices.Disks {
		if disk.CDRom != nil {
			return true
		}
	}
	return false
}

func (vf *vmformatter) canPause(vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vmi == nil {
		return false
	}

	if vmi.Status.Phase != kubevirtv1.Running {
		return false
	}

	if vmi.Spec.LivenessProbe != nil {
		return false
	}

	return !vmiPaused.IsTrue(vmi)
}

func (vf *vmformatter) canUnPause(vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vmi == nil {
		return false
	}

	if vmi.Status.Phase != kubevirtv1.Running {
		return false
	}

	return vmiPaused.IsTrue(vmi)
}

func (vf *vmformatter) canSoftReboot(vmi *kubevirtv1.VirtualMachineInstance) bool {
	return vf.canPause(vmi)
}

func (vf *vmformatter) canStart(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vf.isVMStarting(vm) {
		return false
	}

	if vmi != nil && !vmi.IsFinal() && vmi.Status.Phase != kubevirtv1.Unknown && vmi.Status.Phase != kubevirtv1.VmPhaseUnset {
		return false
	}
	return true
}

func (vf *vmformatter) canRestart(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vf.isVMStarting(vm) {
		return false
	}

	if runStrategy, err := vm.RunStrategy(); err != nil || runStrategy == kubevirtv1.RunStrategyHalted {
		return false
	}

	return vmi != nil
}

func (vf *vmformatter) canStop(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) bool {
	runStrategy, err := vm.RunStrategy()
	if err == nil {
		switch runStrategy {
		case kubevirtv1.RunStrategyHalted:
			return false
		case kubevirtv1.RunStrategyManual, kubevirtv1.RunStrategyRerunOnFailure:
			if vmi == nil {
				return false
			}

			switch vmi.Status.Phase {
			case kubevirtv1.Pending, kubevirtv1.Scheduling, kubevirtv1.Scheduled, kubevirtv1.Running:
				return true
			default:
				return false
			}
		case kubevirtv1.RunStrategyAlways:
			return true
		default:
			// skip to other condition
		}
	}

	return true
}

func canMigrate(vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vmi != nil && vmi.IsRunning() &&
		vmi.Annotations[util.AnnotationMigrationUID] == "" {
		return true
	}
	return false
}

func isReady(vmi *kubevirtv1.VirtualMachineInstance) bool {
	for _, cond := range vmi.Status.Conditions {
		if cond.Type == kubevirtv1.VirtualMachineInstanceReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func canAbortMigrate(vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vmi != nil &&
		vmi.Annotations[util.AnnotationMigrationState] == migration.StateMigrating {
		return true
	}
	return false
}

func (vf *vmformatter) canDoBackup(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vm.Status.SnapshotInProgress != nil {
		return false
	}

	if vm.DeletionTimestamp != nil || (vmi != nil && vmi.DeletionTimestamp != nil) {
		return false
	}

	if vmi != nil && vmi.Status.Phase != kubevirtv1.Running && vmi.Status.Phase != kubevirtv1.Succeeded {
		return false
	}

	return true
}

func (vf *vmformatter) canDoRestore(vm *kubevirtv1.VirtualMachine, vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vm.Status.Ready || vm.Status.SnapshotInProgress != nil || vmi != nil {
		return false
	}
	vmBackups, err := vf.vmBackupCache.GetByIndex(indexeres.VMBackupBySourceVMUIDIndex, string(vm.UID))
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false
		}
		logrus.Errorf("Can't list VM Backups by index %s, err: %+v", indexeres.VMBackupBySourceVMUIDIndex, err)
		return false
	}
	return len(vmBackups) != 0
}

func (vf *vmformatter) isVMStarting(vm *kubevirtv1.VirtualMachine) bool {
	for _, req := range vm.Status.StateChangeRequests {
		if req.Action == kubevirtv1.StartRequest {
			return true
		}
	}
	return false
}

func (vf *vmformatter) canCreateTemplate(vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vmi != nil && vmi.DeletionTimestamp != nil {
		return false
	}

	if vmi != nil && vmi.Status.Phase != kubevirtv1.Running && vmi.Status.Phase != kubevirtv1.Succeeded {
		return false
	}

	return true
}

func (vf *vmformatter) getVMI(vm *kubevirtv1.VirtualMachine) *kubevirtv1.VirtualMachineInstance {
	if vmi, err := vf.vmiCache.Get(vm.Namespace, vm.Name); err == nil {
		return vmi
	}
	return nil
}
