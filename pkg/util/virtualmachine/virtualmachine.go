package virtualmachine

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
)

// IsVMStopped checks VM is stopped or not. It will check two cases
// 1. VM is stopped from GUI
// 2. VM is stopped inside VM
// These two cases are all stopped case.
func IsVMStopped(
	vm *kubevirtv1.VirtualMachine,
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache,
) (bool, error) {
	strategy, err := vm.RunStrategy()
	if err != nil {
		return false, fmt.Errorf("error getting run strategy for vm %s in namespace %s: %v", vm.Name, vm.Namespace, err)
	}

	if vm.Status.PrintableStatus != kubevirtv1.VirtualMachineStatusStopped {
		return false, nil
	}

	// VM is stopped from GUI
	if strategy == kubevirtv1.RunStrategyHalted {
		return true, nil
	}

	// When vm is stopped inside VM, the vmi will not be deleted.
	// The status of vmi will be "Succeeded".
	vmi, err := vmiCache.Get(vm.Namespace, vm.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Debugf("VM %s/%s is deleted", vm.Namespace, vm.Name)
			return true, nil
		}
		return false, fmt.Errorf("error getting vmi %s in namespace %s: %v", vm.Name, vm.Namespace, err)
	}

	if vmi.IsFinal() {
		logrus.Debugf("VM %s/%s is stopped inside VM", vm.Namespace, vm.Name)
		return true, nil
	}

	return false, nil
}

func SupportCPUAndMemoryHotplug(vm *kubevirtv1.VirtualMachine) bool {
	if vm == nil || vm.Annotations == nil {
		return false
	}

	return strings.ToLower(vm.Annotations[util.AnnotationEnableCPUAndMemoryHotplug]) == "true"
}
