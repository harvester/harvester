package dsl

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
)

func MustVMPaused(controller ctlkubevirtv1.VirtualMachineController, namespace, name string) {
	AfterVMExist(controller, namespace, name, func(vm *kubevirtv1.VirtualMachine) bool {
		for _, condition := range vm.Status.Conditions {
			if condition.Type == "Paused" && condition.Status == "True" {
				return true
			}
		}
		return false
	})
}

func MustVMDeleted(controller ctlkubevirtv1.VirtualMachineController, namespace, name string) {
	gomega.Eventually(func() bool {
		_, err := controller.Get(namespace, name, metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			return true
		}
		ginkgo.GinkgoT().Logf("virtual machine %s is still exist: %v", name, err)
		return false
	}, vmTimeoutInterval, vmPollingInterval).Should(gomega.BeTrue())
}

func AfterVMExist(controller ctlkubevirtv1.VirtualMachineController, namespace, name string,
	callback func(vm *kubevirtv1.VirtualMachine) bool) {
	gomega.Eventually(func() bool {
		var vm, err = controller.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			ginkgo.GinkgoT().Logf("failed to get virtual machine: %v", err)
			return false
		}
		return callback(vm)
	}, vmTimeoutInterval, vmPollingInterval).Should(gomega.BeTrue())
}

func MustVMExist(controller ctlkubevirtv1.VirtualMachineController, namespace, name string) {
	AfterVMExist(controller, namespace, name, func(vm *kubevirtv1.VirtualMachine) bool {
		return true
	})
}

func AfterVMReady(controller ctlkubevirtv1.VirtualMachineController, namespace, name string,
	callback func(vm *kubevirtv1.VirtualMachine) bool) {
	AfterVMExist(controller, namespace, name, func(vm *kubevirtv1.VirtualMachine) bool {
		if !vm.Status.Ready {
			return false
		}
		return callback(vm)
	})
}

func MustVMReady(controller ctlkubevirtv1.VirtualMachineController, namespace, name string) {
	AfterVMReady(controller, namespace, name, func(vm *kubevirtv1.VirtualMachine) bool {
		return true
	})
}

func AfterVMRunning(controller ctlkubevirtv1.VirtualMachineController, namespace, name string,
	callback func(vm *kubevirtv1.VirtualMachine) bool) {
	AfterVMReady(controller, namespace, name, func(vm *kubevirtv1.VirtualMachine) bool {
		for _, condition := range vm.Status.Conditions {
			if condition.Type == "Paused" && condition.Status == "True" {
				return false
			}
		}
		return callback(vm)
	})
}

func MustVMRunning(controller ctlkubevirtv1.VirtualMachineController, namespace, name string) {
	AfterVMRunning(controller, namespace, name, func(vm *kubevirtv1.VirtualMachine) bool {
		return true
	})
}

func AfterVMNotReady(controller ctlkubevirtv1.VirtualMachineController, namespace, name string, callback func(vm *kubevirtv1.VirtualMachine) bool) {
	AfterVMExist(controller, namespace, name, func(vm *kubevirtv1.VirtualMachine) bool {
		if vm.Status.Ready {
			return false
		}
		return callback(vm)
	})
}

func HasNoneVMI(controller ctlkubevirtv1.VirtualMachineController, namespace, name string,
	vmiController ctlkubevirtv1.VirtualMachineInstanceController) {
	AfterVMNotReady(controller, namespace, name, func(vm *kubevirtv1.VirtualMachine) bool {
		_, err := vmiController.Get(namespace, name, metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			return true
		}
		return false
	})
}

func HasNoneRunningVMI(controller ctlkubevirtv1.VirtualMachineController, namespace, name string,
	vmiController ctlkubevirtv1.VirtualMachineInstanceController) {
	AfterVMNotReady(controller, namespace, name, func(vm *kubevirtv1.VirtualMachine) bool {
		var vmi, err = vmiController.Get(namespace, name, metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			return true
		}
		if vmi.DeletionTimestamp != nil {
			return true
		}
		return false
	})
}

func AfterVMIExist(controller ctlkubevirtv1.VirtualMachineController, namespace, name string,
	vmiController ctlkubevirtv1.VirtualMachineInstanceController,
	callback func(vmi *kubevirtv1.VirtualMachineInstance) bool) {
	AfterVMReady(controller, namespace, name, func(vm *kubevirtv1.VirtualMachine) bool {
		var vmi, err = vmiController.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			ginkgo.GinkgoT().Logf("failed to get virtual machine instance: %v", err)
			return false
		}
		return callback(vmi)
	})
}

func AfterVMIRunning(controller ctlkubevirtv1.VirtualMachineController, namespace, name string,
	vmiController ctlkubevirtv1.VirtualMachineInstanceController,
	callback func(vmi *kubevirtv1.VirtualMachineInstance) bool) {
	AfterVMIExist(controller, namespace, name, vmiController,
		func(vmi *kubevirtv1.VirtualMachineInstance) bool {
			if !vmi.IsRunning() {
				return false
			}
			return callback(vmi)
		})
}

func MustVMIRunning(controller ctlkubevirtv1.VirtualMachineController, namespace, name string,
	vmiController ctlkubevirtv1.VirtualMachineInstanceController) string {
	var vmiUID string
	AfterVMIRunning(controller, namespace, name, vmiController,
		func(vmi *kubevirtv1.VirtualMachineInstance) bool {
			vmiUID = string(vmi.UID)
			return true
		})
	return vmiUID
}

func AfterVMIRestarted(controller ctlkubevirtv1.VirtualMachineController, namespace, name string,
	vmiController ctlkubevirtv1.VirtualMachineInstanceController, vmiUID string,
	callback func(vmi *kubevirtv1.VirtualMachineInstance) bool) {
	AfterVMIRunning(controller, namespace, name, vmiController,
		func(vmi *kubevirtv1.VirtualMachineInstance) bool {
			if vmiUID == string(vmi.UID) {
				return false
			}
			return callback(vmi)
		})
}

func MustVMIRestarted(controller ctlkubevirtv1.VirtualMachineController, namespace, name string,
	vmiController ctlkubevirtv1.VirtualMachineInstanceController, vmiUID string) {
	AfterVMIRestarted(controller, namespace, name, vmiController, vmiUID,
		func(vmi *kubevirtv1.VirtualMachineInstance) bool {
			return true
		})
}
