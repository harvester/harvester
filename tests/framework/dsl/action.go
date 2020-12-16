package dsl

import (
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1alpha3 "kubevirt.io/client-go/api/v1alpha3"

	ctlkubevirtv1alpha3 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1alpha3"
	"github.com/rancher/harvester/tests/framework/env"
)

const (
	commonTimeoutInterval = 10
	commonPollingInterval = 1

	vmTimeoutInterval = 300
	vmPollingInterval = 2
)

// Cleanup executes the target cleanup execution if "KEEP_TESTING_RESOURCE" isn't "true".
func Cleanup(body interface{}, timeout ...float64) bool {
	if env.IsKeepingTestingVM() {
		return true
	}

	return ginkgo.AfterEach(body, timeout...)
}

func MustFinallyBeTrue(actual func() bool) bool {
	return gomega.Eventually(actual, commonTimeoutInterval, commonPollingInterval).Should(gomega.BeTrue())
}

func MustNotError(err error) bool {
	return gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
}

func MustEqual(actual, expected interface{}) {
	gomega.Expect(actual).Should(gomega.Equal(expected))
}

func MustNotEqual(actual, unExpected interface{}) {
	gomega.Expect(actual).ShouldNot(gomega.Equal(unExpected))
}

func MustChanged(currentValue, newValue, oldValue interface{}) {
	MustEqual(currentValue, newValue)
	MustNotEqual(currentValue, oldValue)
}

func MustRespCodeIs(expectedRespCode int, action string, err error, respCode int, respBody []byte) {
	MustNotError(err)
	if respCode != expectedRespCode {
		ginkgo.GinkgoT().Errorf("failed to %s, response with %d, %v", action, respCode, string(respBody))
	}
}

func CheckRespCodeIs(expectedRespCode int, action string, err error, respCode int, respBody []byte) bool {
	if err != nil {
		ginkgo.GinkgoT().Logf("failed to %s, %v", action, err)
		return false
	}

	if respCode != expectedRespCode {
		ginkgo.GinkgoT().Logf("failed to %s, response with %d, %v", action, respCode, string(respBody))
		return false
	}
	return true
}

func AfterVMReady(vmController ctlkubevirtv1alpha3.VirtualMachineController, vmNamespace, vmName string,
	callback func(vm *kubevirtv1alpha3.VirtualMachine) bool) {
	gomega.Eventually(func() bool {
		var vm, err = vmController.Get(vmNamespace, vmName, metav1.GetOptions{})
		if err != nil {
			ginkgo.GinkgoT().Logf("failed to get virtual machine: %v", err)
			return false
		}
		return callback(vm)
	}, vmTimeoutInterval, vmPollingInterval).Should(gomega.BeTrue())
}

func MustVMReady(vmController ctlkubevirtv1alpha3.VirtualMachineController, vmNamespace, vmName string) {
	AfterVMReady(vmController, vmNamespace, vmName, func(vm *kubevirtv1alpha3.VirtualMachine) bool {
		return vm.Status.Ready
	})
}

func HasNoneVMI(vmController ctlkubevirtv1alpha3.VirtualMachineController, vmNamespace, vmName string,
	vmiController ctlkubevirtv1alpha3.VirtualMachineInstanceController) {
	AfterVMReady(vmController, vmNamespace, vmName, func(vm *kubevirtv1alpha3.VirtualMachine) bool {
		if !vm.Status.Ready {
			_, err := vmiController.Get(vmNamespace, vmName, metav1.GetOptions{})
			if err != nil && apierrors.IsNotFound(err) {
				return true
			}
			return false
		}
		return false
	})
}

func HasNoneRunningVMI(vmController ctlkubevirtv1alpha3.VirtualMachineController, vmNamespace, vmName string,
	vmiController ctlkubevirtv1alpha3.VirtualMachineInstanceController) {
	AfterVMReady(vmController, vmNamespace, vmName, func(vm *kubevirtv1alpha3.VirtualMachine) bool {
		if !vm.Status.Ready {
			var vmi, err = vmiController.Get(vmNamespace, vmName, metav1.GetOptions{})
			if err != nil && apierrors.IsNotFound(err) {
				return true
			}
			if vmi.DeletionTimestamp != nil {
				return true
			}
			return false
		}
		return false
	})
}

func AfterVMIReady(vmController ctlkubevirtv1alpha3.VirtualMachineController, vmNamespace, vmName string,
	vmiController ctlkubevirtv1alpha3.VirtualMachineInstanceController,
	callback func(vmi *kubevirtv1alpha3.VirtualMachineInstance) bool) {
	AfterVMReady(vmController, vmNamespace, vmName, func(vm *kubevirtv1alpha3.VirtualMachine) bool {
		if vm.Status.Ready {
			var vmi, err = vmiController.Get(vmNamespace, vmName, metav1.GetOptions{})
			if err != nil {
				ginkgo.GinkgoT().Logf("failed to get virtual machine instance: %v", err)
				return false
			}
			return callback(vmi)
		}
		return false
	})
}
