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

func supportNicHotActionCommon(vm *kubevirtv1.VirtualMachine) (bool, error) {
	if vm == nil {
		return false, fmt.Errorf("vm shouldn't be nil")
	}

	// for stability, guest cluster VMs aren't allowed until integration with Rancher Manger is done
	if vm.Labels != nil && vm.Labels[util.LabelVMCreator] == util.VirtualMachineCreatorNodeDriver {
		return false, fmt.Errorf("%s/%s doesn't support both HotPlugNic and HotUnplugNic as it is a guest cluster node", vm.Namespace, vm.Name)
	}

	// to prevent unexpected RestartRequired condition due to missing macAddress in VM spec,
	// caused by our existing implmentation for preserving MAC addresses, VMs without macAddress defined in VM spec are not allowed
	// until backfilling MAC addresses to VM spec while stopping VM by the new reconciliation logic
	for _, iface := range vm.Spec.Template.Spec.Domain.Devices.Interfaces {
		if iface.MacAddress == "" {
			return false, fmt.Errorf("%s/%s doesn't support both HotPlugNic and HotUnplugNic as macAddress is missing for some interfaces in the VM spec", vm.Namespace, vm.Name)
		}
	}

	return true, nil
}

func SupportHotplugNic(vm *kubevirtv1.VirtualMachine) (bool, error) {
	return supportNicHotActionCommon(vm)
}

func IsInterfaceHotUnpluggable(iface kubevirtv1.Interface) (bool, error) {
	if iface.State == kubevirtv1.InterfaceStateAbsent {
		return false, fmt.Errorf("%s was already registered for hot-unplugging", iface.Name)
	}

	if iface.Bridge == nil {
		return false, fmt.Errorf("%s is not in bridge mode", iface.Name)
	}

	if iface.Model != "" && iface.Model != kubevirtv1.VirtIO {
		return false, fmt.Errorf("%s is not using virtio model", iface.Name)
	}

	return true, nil
}

func SupportHotUnplugNic(vm *kubevirtv1.VirtualMachine) (bool, error) {
	if _, err := supportNicHotActionCommon(vm); err != nil {
		return false, err
	}

	ifaces := vm.Spec.Template.Spec.Domain.Devices.Interfaces
	if len(ifaces) <= 1 {
		return false, fmt.Errorf("%s/%s doesn't support HotUnplugNic as it has only one nic", vm.Namespace, vm.Name)
	}

	errMsgs := make([]string, 0)
	for _, iface := range ifaces {
		ok, err := IsInterfaceHotUnpluggable(iface);
		if ok {
			// as long as there is at least one hot-unpluggable interface
			return true, nil
		}
		if err != nil {
			errMsgs = append(errMsgs, err.Error())
		}
	}

	return false, fmt.Errorf("%s/%s doesn't support HotUnplugNic as none of its interfaces is hot-unpluggable: %s", vm.Namespace, vm.Name, strings.Join(errMsgs, ", "))
}
