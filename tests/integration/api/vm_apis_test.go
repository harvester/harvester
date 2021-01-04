package api_test

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1alpha3 "kubevirt.io/client-go/api/v1alpha3"

	apivm "github.com/rancher/harvester/pkg/api/vm"
	"github.com/rancher/harvester/pkg/config"
	ctlkubevirtv1alpha3 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1alpha3"
	. "github.com/rancher/harvester/tests/framework/dsl"
	"github.com/rancher/harvester/tests/framework/fuzz"
	"github.com/rancher/harvester/tests/framework/helper"
)

var _ = Describe("verify vm APIs", func() {

	var (
		scaled        *config.Scaled
		vmController  ctlkubevirtv1alpha3.VirtualMachineController
		vmiController ctlkubevirtv1alpha3.VirtualMachineInstanceController
		vmNamespace   string
		vmBuilder     *VMBuilder
	)

	BeforeEach(func() {
		scaled = harvester.Scaled()
		vmController = scaled.VirtFactory.Kubevirt().V1alpha3().VirtualMachine()
		vmiController = scaled.VirtFactory.Kubevirt().V1alpha3().VirtualMachineInstance()
		vmNamespace = testVMNamespace
		vmBuilder = NewDefaultTestVMBuilder()
	})

	Cleanup(func() {
		vmList, err := vmController.List(vmNamespace, metav1.ListOptions{
			LabelSelector: labels.FormatLabels(testResourceLabels)})
		if err != nil {
			GinkgoT().Logf("failed to list tested vms, %v", err)
			return
		}
		for _, item := range vmList.Items {
			if err = vmController.Delete(item.Namespace, item.Name, &metav1.DeleteOptions{}); err != nil {
				GinkgoT().Logf("failed to delete tested vm %s/%s, %v", item.Namespace, item.Name, err)
			}
		}
	})

	Context("operate via steve API", func() {

		var vmsAPI string

		BeforeEach(func() {

			vmsAPI = helper.BuildAPIURL("v1", "kubevirt.io.virtualmachines")

		})

		Context("verify the create action", func() {
			It("should fail with name missing", func() {
				vm := vmBuilder.Name("").Blank().VM()
				respCode, respBody, err := helper.PostObject(vmsAPI, vm)
				MustRespCodeIs(http.StatusUnprocessableEntity, "create vm", err, respCode, respBody)
			})

			It("create a virtual machine with cloud-init", func() {
				vmName := testVMGenerateName + fuzz.String(5)
				vmCloudInit := &VMCloudInit{
					Name:     vmName,
					UserName: "fedora",
					Password: "root",
					Address:  "10.5.2.100/24",
					Gateway:  "10.5.2.1",
				}
				vm := vmBuilder.Name(vmName).
					Container().
					Blank().
					CloudInit(vmCloudInit).
					Run()
				respCode, respBody, err := helper.PostObject(vmsAPI, vm)
				MustRespCodeIs(http.StatusCreated, "create vm", err, respCode, respBody)

				By("then the virtual machine is created and running")
				MustVMIRunning(vmController, vmNamespace, vmName, vmiController)
			})
		})

		Specify("verify the start action", func() {

			By("given a stopped virtual machine")
			vm, err := vmController.Create(vmBuilder.Container().VM())
			MustNotError(err)
			vmName := vm.Name
			HasNoneVMI(vmController, vmNamespace, vmName, vmiController)

			By("when call start action")
			vmURL := helper.BuildResourceURL(vmsAPI, vmNamespace, vmName)
			respCode, respBody, err := helper.PostAction(vmURL, "start")
			MustRespCodeIs(http.StatusNoContent, "post start action", err, respCode, respBody)

			By("then the virtual machine is started")
			MustVMIRunning(vmController, vmNamespace, vmName, vmiController)
		})

		Specify("verify the stop action", func() {

			By("given a started virtual machine")
			vm, err := vmController.Create(vmBuilder.Container().Run())
			MustNotError(err)
			vmName := vm.Name
			MustVMIRunning(vmController, vmNamespace, vmName, vmiController)

			By("when call stop action")
			vmURL := helper.BuildResourceURL(vmsAPI, vmNamespace, vmName)
			respCode, respBody, err := helper.PostAction(vmURL, "stop")
			MustRespCodeIs(http.StatusNoContent, "post stop action", err, respCode, respBody)

			By("then the virtual machine is stopped")
			HasNoneRunningVMI(vmController, vmNamespace, vmName, vmiController)
		})

		Specify("verify the edit and restart action", func() {

			By("given a started virtual machine")
			vm, err := vmController.Create(vmBuilder.Container().Run())
			MustNotError(err)
			vmName := vm.Name
			vmiUID := MustVMIRunning(vmController, vmNamespace, vmName, vmiController)
			vm, err = vmController.Get(vmNamespace, vmName, metav1.GetOptions{})
			MustNotError(err)

			By("when edit virtual machine")
			updatedCPUCore := uint32(2)
			updatedMemory := "200Mi"
			vm = NewVMBuilder(vm).
				CPU(updatedCPUCore).
				Memory(updatedMemory).
				Blank().
				VM()
			vmURL := helper.BuildResourceURL(vmsAPI, vmNamespace, vmName)
			respCode, respBody, err := helper.PutObject(vmURL, vm)
			MustRespCodeIs(http.StatusOK, "put edit action", err, respCode, respBody)

			By("then the virtual machine is changed")
			AfterVMRunning(vmController, vmNamespace, vmName, func(vm *kubevirtv1alpha3.VirtualMachine) bool {
				spec := vm.Spec.Template.Spec
				MustEqual(len(spec.Domain.Devices.Disks), 2)
				MustEqual(spec.Domain.CPU.Cores, updatedCPUCore)
				MustEqual(spec.Domain.Resources.Requests[corev1.ResourceMemory], resource.MustParse(updatedMemory))
				return true
			})

			By("but the virtual machine instance isn't changed")
			AfterVMIRunning(vmController, vmNamespace, vmName, vmiController,
				func(vmi *kubevirtv1alpha3.VirtualMachineInstance) bool {
					MustEqual(vmiUID, string(vmi.UID))
					return true
				})

			By("when call restart action")
			respCode, respBody, err = helper.PostAction(fmt.Sprintf("%s/%s/%s", vmsAPI, vmNamespace, vmName), "restart")
			MustRespCodeIs(http.StatusNoContent, "post restart action", err, respCode, respBody)

			By("then the virtual machine instance is changed")
			AfterVMIRestarted(vmController, vmNamespace, vmName, vmiController, vmiUID,
				func(vmi *kubevirtv1alpha3.VirtualMachineInstance) bool {
					spec := vm.Spec.Template.Spec
					MustEqual(len(spec.Domain.Devices.Disks), 2)
					MustEqual(spec.Domain.CPU.Cores, updatedCPUCore)
					MustEqual(spec.Domain.Resources.Requests[corev1.ResourceMemory], resource.MustParse(updatedMemory))
					return true
				})
		})

		Specify("verify the pause and unpause action", func() {
			By("given a started virtual machine")
			vm, err := vmController.Create(vmBuilder.Container().Run())
			MustNotError(err)
			vmName := vm.Name
			MustVMIRunning(vmController, vmNamespace, vmName, vmiController)

			By("when call pause action")
			vmURL := helper.BuildResourceURL(vmsAPI, vmNamespace, vmName)
			respCode, respBody, err := helper.PostAction(vmURL, "pause")
			MustRespCodeIs(http.StatusNoContent, "post pause action", err, respCode, respBody)

			By("then the virtual machine is paused")
			MustVMPaused(vmController, vmNamespace, vmName)

			By("when call unpause action")
			respCode, respBody, err = helper.PostAction(vmURL, "unpause")
			MustRespCodeIs(http.StatusNoContent, "post unpause action", err, respCode, respBody)

			By("then the virtual machine is running")
			MustVMRunning(vmController, vmNamespace, vmName)
		})

		Specify("verify the delete action", func() {
			By("given a started virtual machine")
			vm, err := vmController.Create(vmBuilder.Container().Run())
			MustNotError(err)
			vmName := vm.Name
			MustVMIRunning(vmController, vmNamespace, vmName, vmiController)

			By("when delete the virtual machine")
			vmURL := helper.BuildResourceURL(vmsAPI, vmNamespace, vmName)
			respCode, respBody, err := helper.DeleteObject(vmURL)
			MustRespCodeIs(http.StatusOK, "delete action", err, respCode, respBody)

			By("then the virtual machine is deleted")
			MustVMDeleted(vmController, vmNamespace, vmName)
		})

		Context("verify the ejectCdRom action", func() {

			It("should fail if there are not CdRoms in the virtual machine", func() {

				By("given a started virtual machine without any CdRoms")
				vm, err := vmController.Create(vmBuilder.Container().Run())
				MustNotError(err)
				vmName := vm.Name
				MustVMIRunning(vmController, vmNamespace, vmName, vmiController)

				By("when call ejectCdRom action")
				vmURL := helper.BuildResourceURL(vmsAPI, vmNamespace, vmName)
				respCode, respBody, err := helper.PostObjectAction(vmURL, apivm.EjectCdRomActionInput{}, "ejectCdRom")
				MustRespCodeIs(http.StatusUnprocessableEntity, "ejectCdRom", err, respCode, respBody)
			})

			It("should fail if request without any CdRoms", func() {

				By("given a started virtual machine with one CdRom")
				vm, err := vmController.Create(vmBuilder.Container().CDRom().Run())
				MustNotError(err)
				vmName := vm.Name
				MustVMIRunning(vmController, vmNamespace, vmName, vmiController)

				By("when call ejectCdRom action without any CdRoms")
				vmURL := helper.BuildResourceURL(vmsAPI, vmNamespace, vmName)
				respCode, respBody, err := helper.PostObjectAction(vmURL, apivm.EjectCdRomActionInput{}, "ejectCdRom")
				MustRespCodeIs(http.StatusUnprocessableEntity, "ejectCdRom", err, respCode, respBody)

			})

			It("should fail if the ejected target is not CdRom", func() {

				By("given a started virtual machine with one CdRom")
				vm, err := vmController.Create(vmBuilder.Container().CDRom().Run())
				MustNotError(err)
				vmName := vm.Name
				MustVMIRunning(vmController, vmNamespace, vmName, vmiController)

				By("when call ejectCdRom action with a not existed CdRom")
				vmURL := helper.BuildResourceURL(vmsAPI, vmNamespace, vmName)
				respCode, respBody, err := helper.PostObjectAction(vmURL, apivm.EjectCdRomActionInput{
					DiskNames: []string{testVMContainerDiskName},
				}, "ejectCdRom")
				MustRespCodeIs(http.StatusInternalServerError, "ejectCdRom", err, respCode, respBody)
			})

			It("should eject the CdRom", func() {

				By("given a started virtual machine with one CdRom")
				vm, err := vmController.Create(vmBuilder.Container().CDRom().Run())
				MustNotError(err)
				vmName := vm.Name
				MustVMIRunning(vmController, vmNamespace, vmName, vmiController)

				By("when call ejectCdRom action")
				vmURL := helper.BuildResourceURL(vmsAPI, vmNamespace, vmName)
				respCode, respBody, err := helper.PostObjectAction(vmURL, apivm.EjectCdRomActionInput{
					DiskNames: []string{testVMCDRomDiskName},
				}, "ejectCdRom")
				MustRespCodeIs(http.StatusNoContent, "post ejectCdRom action", err, respCode, respBody)

				By("then the CdRom is ejected")
				AfterVMIRunning(vmController, vmNamespace, vmName, vmiController,
					func(vmi *kubevirtv1alpha3.VirtualMachineInstance) bool {
						for _, disk := range vmi.Spec.Domain.Devices.Disks {
							if disk.CDRom != nil && disk.Name == testVMCDRomDiskName {
								return false
							}
						}
						return true
					})
			})

		})

	})

})
