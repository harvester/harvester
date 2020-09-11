package vm

import (
	"github.com/rancher/apiserver/pkg/types"
	ctlkubevirtv1alpha3 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1alpha3"
	"github.com/rancher/steve/pkg/resources/common"
	"github.com/rancher/wrangler/pkg/data/convert"

	v1alpha3 "kubevirt.io/client-go/api/v1alpha3"
)

const (
	startVM    = "start"
	stopVM     = "stop"
	restartVM  = "restart"
	pauseVM    = "pause"
	unpauseVM  = "unpause"
	ejectCdRom = "ejectCdRom"
)

type vmformatter struct {
	vmiCache ctlkubevirtv1alpha3.VirtualMachineInstanceCache
}

func (vf *vmformatter) formatter(request *types.APIRequest, resource *types.RawResource) {
	common.Formatter(request, resource)
	// reset resource actions, because action map already be set when add actions handler,
	// but current framework can't support use formatter to remove key from action map
	resource.Actions = make(map[string]string, 1)

	vm := &v1alpha3.VirtualMachine{}
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
}

func canEjectCdRom(vm *v1alpha3.VirtualMachine) bool {
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

func (vf *vmformatter) canPause(vmi *v1alpha3.VirtualMachineInstance) bool {
	if vmi.Status.Phase != v1alpha3.Running {
		return false
	}

	if vmi.Spec.LivenessProbe != nil {
		return false
	}

	return !vmiPaused.IsTrue(vmi)
}

func (vf *vmformatter) canUnPause(vmi *v1alpha3.VirtualMachineInstance) bool {
	if vmi.Status.Phase != v1alpha3.Running {
		return false
	}

	return vmiPaused.IsTrue(vmi)
}

func (vf *vmformatter) canStart(vm *v1alpha3.VirtualMachine, vmi *v1alpha3.VirtualMachineInstance) bool {
	if vf.isVMRenaming(vm) {
		return false
	}

	if vmi != nil && !vmi.IsFinal() && vmi.Status.Phase != v1alpha3.Unknown && vmi.Status.Phase != v1alpha3.VmPhaseUnset {
		return false
	}
	return true
}

func (vf *vmformatter) canRestart(vm *v1alpha3.VirtualMachine, vmi *v1alpha3.VirtualMachineInstance) bool {
	if vf.isVMRenaming(vm) {
		return false
	}

	if runStrategy, err := vm.RunStrategy(); err != nil || runStrategy == v1alpha3.RunStrategyHalted {
		return false
	}

	return vmi != nil
}

func (vf *vmformatter) canStop(vmi *v1alpha3.VirtualMachineInstance) bool {
	if vmi == nil || vmi.IsFinal() || vmi.Status.Phase == v1alpha3.Unknown || vmi.Status.Phase == v1alpha3.VmPhaseUnset {
		return false
	}

	return true
}

func (vf *vmformatter) isVMRenaming(vm *v1alpha3.VirtualMachine) bool {
	for _, req := range vm.Status.StateChangeRequests {
		if req.Action == v1alpha3.RenameRequest {
			return true
		}
	}
	return false
}

func (vf *vmformatter) getVMI(vm *v1alpha3.VirtualMachine) *v1alpha3.VirtualMachineInstance {
	if vmi, err := vf.vmiCache.Get(vm.Namespace, vm.Name); err == nil {
		return vmi
	}
	return nil
}
