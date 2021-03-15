package vm

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/data/convert"
	kv1 "kubevirt.io/client-go/api/v1"

	ctlkubevirtv1 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

const (
	startVM        = "start"
	stopVM         = "stop"
	restartVM      = "restart"
	pauseVM        = "pause"
	unpauseVM      = "unpause"
	ejectCdRom     = "ejectCdRom"
	migrate        = "migrate"
	abortMigration = "abortMigration"
	backupVM       = "backup"
	restoreVM      = "restore"
)

type vmformatter struct {
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache
}

func (vf *vmformatter) formatter(request *types.APIRequest, resource *types.RawResource) {
	// reset resource actions, because action map already be set when add actions handler,
	// but current framework can't support use formatter to remove key from action map
	resource.Actions = make(map[string]string, 1)

	vm := &kv1.VirtualMachine{}
	err := convert.ToObj(resource.APIObject.Data(), vm)
	if err != nil {
		return
	}

	if canEjectCdRom(vm) {
		resource.AddAction(request, ejectCdRom)
	}

	vmi := vf.getVMI(vm)
	if vf.canStart(vm, vmi) {
		resource.AddAction(request, startVM)
	}

	if vf.canStop(vmi) {
		resource.AddAction(request, stopVM)
	}

	if vf.canRestart(vm, vmi) {
		resource.AddAction(request, restartVM)
	}

	if vf.canPause(vmi) {
		resource.AddAction(request, pauseVM)
	}

	if vf.canUnPause(vmi) {
		resource.AddAction(request, unpauseVM)
	}

	if vf.canMigrate(vmi) {
		resource.AddAction(request, migrate)
	}

	if vf.canAbortMigrate(vmi) {
		resource.AddAction(request, abortMigration)
	}

	if vf.canDoBackup(vm) {
		resource.AddAction(request, backupVM)
	}

	if vf.canDoRestore(vm, vmi) {
		resource.AddAction(request, restoreVM)
	}
}

func canEjectCdRom(vm *kv1.VirtualMachine) bool {
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

func (vf *vmformatter) canPause(vmi *kv1.VirtualMachineInstance) bool {
	if vmi == nil {
		return false
	}

	if vmi.Status.Phase != kv1.Running {
		return false
	}

	if vmi.Spec.LivenessProbe != nil {
		return false
	}

	return !vmiPaused.IsTrue(vmi)
}

func (vf *vmformatter) canUnPause(vmi *kv1.VirtualMachineInstance) bool {
	if vmi == nil {
		return false
	}

	if vmi.Status.Phase != kv1.Running {
		return false
	}

	return vmiPaused.IsTrue(vmi)
}

func (vf *vmformatter) canStart(vm *kv1.VirtualMachine, vmi *kv1.VirtualMachineInstance) bool {
	if vf.isVMRenaming(vm) {
		return false
	}

	if vmi != nil && !vmi.IsFinal() && vmi.Status.Phase != kv1.Unknown && vmi.Status.Phase != kv1.VmPhaseUnset {
		return false
	}
	return true
}

func (vf *vmformatter) canRestart(vm *kv1.VirtualMachine, vmi *kv1.VirtualMachineInstance) bool {
	if vf.isVMRenaming(vm) {
		return false
	}

	if runStrategy, err := vm.RunStrategy(); err != nil || runStrategy == kv1.RunStrategyHalted {
		return false
	}

	return vmi != nil
}

func (vf *vmformatter) canStop(vmi *kv1.VirtualMachineInstance) bool {
	if vmi == nil || vmi.IsFinal() || vmi.Status.Phase == kv1.Unknown || vmi.Status.Phase == kv1.VmPhaseUnset {
		return false
	}

	return true
}

func (vf *vmformatter) canMigrate(vmi *kv1.VirtualMachineInstance) bool {
	if vmi != nil && vmi.IsRunning() && (vmi.Status.MigrationState == nil || vmi.Status.MigrationState.Completed) {
		return true
	}
	return false
}

func (vf *vmformatter) canAbortMigrate(vmi *kv1.VirtualMachineInstance) bool {
	if vmi != nil && vmi.Status.MigrationState != nil && !vmi.Status.MigrationState.Completed {
		return true
	}
	return false
}

func (vf *vmformatter) canDoBackup(vm *kv1.VirtualMachine) bool {
	if !vm.Status.Created || vm.Status.SnapshotInProgress != nil {
		return false
	}
	return true
}

func (vf *vmformatter) canDoRestore(vm *kv1.VirtualMachine, vmi *kv1.VirtualMachineInstance) bool {
	if vm.Status.Ready || vm.Status.SnapshotInProgress != nil || vmi != nil {
		return false
	}
	return true
}

func (vf *vmformatter) isVMRenaming(vm *kv1.VirtualMachine) bool {
	for _, req := range vm.Status.StateChangeRequests {
		if req.Action == kv1.RenameRequest {
			return true
		}
	}
	return false
}

func (vf *vmformatter) getVMI(vm *kv1.VirtualMachine) *kv1.VirtualMachineInstance {
	if vmi, err := vf.vmiCache.Get(vm.Namespace, vm.Name); err == nil {
		return vmi
	}
	return nil
}
