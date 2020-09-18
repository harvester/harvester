package vm

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/resources/common"
	"github.com/rancher/wrangler/pkg/data/convert"

	v1alpha3 "kubevirt.io/client-go/api/v1alpha3"
)

const (
	startVM    = "startVM"
	stopVM     = "stopVM"
	restartVM  = "restartVM"
	ejectCdRom = "ejectCdRom"
)

func formatter(request *types.APIRequest, resource *types.RawResource) {
	common.Formatter(request, resource)
	// reset resource actions, because action map already be set when add actions handler,
	// but current framework can't support use formatter to remove key from action map
	resource.Actions = make(map[string]string, 1)
	if resource.APIObject.Data().Bool("spec", "running") {
		resource.AddAction(request, stopVM)
		resource.AddAction(request, restartVM)

		var vm v1alpha3.VirtualMachine
		err := convert.ToObj(resource.APIObject.Data(), &vm)
		if err == nil && canEjectCdRom(&vm) {
			resource.AddAction(request, ejectCdRom)
		}

	} else {
		resource.AddAction(request, startVM)
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
