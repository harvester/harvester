package api_test

import (
	"fmt"
	"net/http"
	"time"

	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubevirtv1 "kubevirt.io/api/core/v1"

	apivm "github.com/harvester/harvester/pkg/api/vm"
	"github.com/harvester/harvester/pkg/builder"
	"github.com/harvester/harvester/pkg/config"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/tests/framework/fuzz"
	"github.com/harvester/harvester/tests/framework/helper"
)

var _ = Describe("verify vm APIs", func() {

	var (
		scaled        *config.Scaled
		vmController  ctlkubevirtv1.VirtualMachineController
		vmiController ctlkubevirtv1.VirtualMachineInstanceController
		pvcController v1.PersistentVolumeClaimController
		vmNamespace   string
	)

	BeforeEach(func() {
		scaled = harvester.Scaled()
		vmController = scaled.VirtFactory.Kubevirt().V1().VirtualMachine()
		vmiController = scaled.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
		pvcController = scaled.CoreFactory.Core().V1().PersistentVolumeClaim()
		vmNamespace = testVMNamespace
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

		pvcList, err := pvcController.List(vmNamespace, metav1.ListOptions{
			LabelSelector: labels.FormatLabels(testResourceLabels)})
		if err != nil {
			GinkgoT().Logf("failed to list tested pvcs, %v", err)
			return
		}
		for _, item := range pvcList.Items {
			if err = pvcController.Delete(item.Namespace, item.Name, &metav1.DeleteOptions{}); err != nil {
				GinkgoT().Logf("failed to delete tested pvc %s/%s, %v", item.Namespace, item.Name, err)
			}
		}
	})

	Context("operate via steve API", func() {

		var vmsAPI string

		BeforeEach(func() {

			vmsAPI = helper.BuildAPIURL("v1", "kubevirt.io.virtualmachines", options.HTTPSListenPort)

		})

		Specify("verify vm api", func() {

			By("when create a virtual machine with cloud-init")
			vmName := testVMGenerateName + fuzz.String(5)
			vmCloudInit := &VMCloudInit{
				Name:     vmName,
				UserName: "fedora",
				Password: "root",
				Address:  "10.5.2.100/24",
				Gateway:  "10.5.2.1",
			}
			userData := fmt.Sprintf(testVMCloudInitUserDataTemplate, vmCloudInit.UserName, vmCloudInit.Password)
			networkData := fmt.Sprintf(testVMCloudInitNetworkDataTemplate, vmCloudInit.Address, vmCloudInit.Gateway)
			vm, err := NewDefaultTestVMBuilder(testResourceLabels).Name(vmName).
				NetworkInterface(testVMInterfaceName, testVMInterfaceModel, "", builder.NetworkInterfaceTypeMasquerade, "").
				ContainerDisk(testVMContainerDiskName, testVMDefaultDiskBus, false, 1, testVMContainerDiskImageName, testVMContainerDiskImagePullPolicy).
				CloudInitDisk(testVMCloudInitDiskName, testVMDefaultDiskBus, false, 0, builder.CloudInitSource{
					CloudInitType: builder.CloudInitTypeNoCloud,
					UserData:      userData,
					NetworkData:   networkData,
				}).Run(true).VM()
			MustNotError(err)
			respCode, respBody, err := helper.PostObject(vmsAPI, vm)
			MustRespCodeIs(http.StatusCreated, "create vm", err, respCode, respBody)

			By("then the virtual machine is created and running")
			vmiUID := MustVMIRunning(vmController, vmNamespace, vmName, vmiController)
			vm, err = vmController.Get(vmNamespace, vmName, metav1.GetOptions{})
			MustNotError(err)

			// ejectCdRom
			By("ejectCdRom should fail if there are no CdRoms in the virtual machine")
			vmURL := helper.BuildResourceURL(vmsAPI, vmNamespace, vmName)
			respCode, respBody, err = helper.PostObjectAction(vmURL, apivm.EjectCdRomActionInput{}, "ejectCdRom")
			MustRespCodeIs(http.StatusUnprocessableEntity, "ejectCdRom", err, respCode, respBody)

			// edit
			By("when edit virtual machine")
			MustFinallyBeTrue(func() bool {
				// re-get, vm object may be outdated at this point
				vm, err = vmController.Get(vmNamespace, vmName, metav1.GetOptions{})
				MustNotError(err)
				vm, err = builder.NewVMBuilder(testCreator).Update(vm).CPU(testVMUpdatedCPUCore).Memory(testVMUpdatedMemory).
					PVCDisk(testVMCDRomDiskName, testVMCDRomBus, true, false, 2, testVMDiskSize, "", &builder.PersistentVolumeClaimOption{
						VolumeMode: builder.PersistentVolumeModeFilesystem,
						AccessMode: builder.PersistentVolumeAccessModeReadWriteOnce,
					}).VM()
				MustNotError(err)
				respCode, _, err = helper.PutObject(vmURL, vm)
				MustNotError(err)
				// 409 may also occur
				// e.g.: {...the object has been modified; please apply your changes to the latest version and try again","status":409,"type":"error"}
				Expect(respCode).To(BeElementOf([]int{http.StatusOK, http.StatusConflict}))
				return respCode == http.StatusOK
			}, 10*time.Second, 3*time.Second)

			By("then the virtual machine is changed")
			AfterVMRunning(vmController, vmNamespace, vmName, func(vm *kubevirtv1.VirtualMachine) bool {
				spec := vm.Spec.Template.Spec
				MustEqual(len(spec.Domain.Devices.Disks), 3)
				MustEqual(spec.Domain.CPU.Cores, uint32(testVMUpdatedCPUCore))
				MustEqual(spec.Domain.Resources.Limits[corev1.ResourceMemory], resource.MustParse(testVMUpdatedMemory))
				return true
			})

			By("but the virtual machine instance isn't changed")
			AfterVMIRunning(vmController, vmNamespace, vmName, vmiController,
				func(vmi *kubevirtv1.VirtualMachineInstance) bool {
					MustEqual(vmiUID, string(vmi.UID))
					return true
				})

			// restart
			By("when call restart action")
			respCode, respBody, err = helper.PostAction(fmt.Sprintf("%s/%s/%s", vmsAPI, vmNamespace, vmName), "restart")
			MustRespCodeIs(http.StatusNoContent, "post restart action", err, respCode, respBody)

			By("then the virtual machine instance is changed")
			AfterVMIRestarted(vmController, vmNamespace, vmName, vmiController, vmiUID,
				func(vmi *kubevirtv1.VirtualMachineInstance) bool {
					spec := vmi.Spec
					MustEqual(len(spec.Domain.Devices.Disks), 3)
					MustEqual(spec.Domain.CPU.Cores, uint32(testVMUpdatedCPUCore))
					MustEqual(spec.Domain.Resources.Limits[corev1.ResourceMemory], resource.MustParse(testVMUpdatedMemory))
					return true
				})

			// ejectCdRom
			By("ejectCdRom should fail if request without any CdRoms")
			vmURL = helper.BuildResourceURL(vmsAPI, vmNamespace, vmName)
			respCode, respBody, err = helper.PostObjectAction(vmURL, apivm.EjectCdRomActionInput{}, "ejectCdRom")
			MustRespCodeIs(http.StatusUnprocessableEntity, "ejectCdRom", err, respCode, respBody)

			By("ejectCdRom should fail if the ejected target is not CdRom")
			vmURL = helper.BuildResourceURL(vmsAPI, vmNamespace, vmName)
			respCode, respBody, err = helper.PostObjectAction(vmURL, apivm.EjectCdRomActionInput{
				DiskNames: []string{testVMContainerDiskName},
			}, "ejectCdRom")
			MustRespCodeIs(http.StatusInternalServerError, "ejectCdRom", err, respCode, respBody)

			By("when call ejectCdRom action with correct cdrom")
			vmURL = helper.BuildResourceURL(vmsAPI, vmNamespace, vmName)
			respCode, respBody, err = helper.PostObjectAction(vmURL, apivm.EjectCdRomActionInput{
				DiskNames: []string{testVMCDRomDiskName},
			}, "ejectCdRom")
			MustRespCodeIs(http.StatusNoContent, "post ejectCdRom action", err, respCode, respBody)

			By("then the CdRom is ejected")
			AfterVMIRunning(vmController, vmNamespace, vmName, vmiController,
				func(vmi *kubevirtv1.VirtualMachineInstance) bool {
					for _, disk := range vmi.Spec.Domain.Devices.Disks {
						if disk.CDRom != nil && disk.Name == testVMCDRomDiskName {
							return false
						}
					}
					return true
				})

			// stop
			By("when call stop action")
			respCode, respBody, err = helper.PostAction(vmURL, "stop")
			MustRespCodeIs(http.StatusNoContent, "post stop action", err, respCode, respBody)

			By("then the virtual machine is stopped")
			HasNoneVMI(vmController, vmNamespace, vmName, vmiController)

			// start
			By("when call start action")
			respCode, respBody, err = helper.PostAction(vmURL, "start")
			MustRespCodeIs(http.StatusNoContent, "post start action", err, respCode, respBody)

			By("then the virtual machine is started")
			MustVMIRunning(vmController, vmNamespace, vmName, vmiController)

			// pause
			By("when call pause action")
			respCode, respBody, err = helper.PostAction(vmURL, "pause")
			MustRespCodeIs(http.StatusNoContent, "post pause action", err, respCode, respBody)

			By("then the virtual machine is paused")
			MustVMPaused(vmController, vmNamespace, vmName)

			// unpause
			By("when call unpause action")
			respCode, respBody, err = helper.PostAction(vmURL, "unpause")
			MustRespCodeIs(http.StatusNoContent, "post unpause action", err, respCode, respBody)

			By("then the virtual machine is running")
			MustVMRunning(vmController, vmNamespace, vmName)

			// delete
			By("when delete the virtual machine")
			respCode, respBody, err = helper.DeleteObject(vmURL)
			MustRespCodeIs(http.StatusOK, "delete action", err, respCode, respBody)

			By("then the virtual machine is deleted")
			MustVMDeleted(vmController, vmNamespace, vmName)
		})

		Specify("deleting a vm and its volume", func() {
			By("create a virtual machine with one spare disk")
			vmName := testVMGenerateName + fuzz.String(5)
			vm, err := NewDefaultTestVMBuilder(testResourceLabels).Name(vmName).
				NetworkInterface(testVMInterfaceName, testVMInterfaceModel, "", builder.NetworkInterfaceTypeMasquerade, "").
				PVCDisk(testVMRemoveDiskName, testVMDefaultDiskBus, false, false, 1, testVMDiskSize, testVMRemoveDiskName, &builder.PersistentVolumeClaimOption{
					VolumeMode: builder.PersistentVolumeModeFilesystem,
					AccessMode: builder.PersistentVolumeAccessModeReadWriteOnce,
				}).
				Run(true).VM()
			MustNotError(err)
			respCode, respBody, err := helper.PostObject(vmsAPI, vm)
			MustRespCodeIs(http.StatusCreated, "create vm", err, respCode, respBody)

			By("then the virtual machine is created and running")
			MustVMIRunning(vmController, vmNamespace, vmName, vmiController)
			_, err = vmController.Get(vmNamespace, vmName, metav1.GetOptions{})
			MustNotError(err)

			By("when deleting the virtual machine with removeDisks query parameter", func() {
				vmURL := helper.BuildResourceURL(vmsAPI, vmNamespace, vmName)
				queryParams := fmt.Sprintf("?removedDisks=%s", testVMRemoveDiskName)
				MustFinallyBeTrue(func() bool {
					respCode, respBody, err = helper.DeleteObject(vmURL + queryParams)
					return CheckRespCodeIs(http.StatusOK, "delete action", err, respCode, respBody)
				}, 10*time.Second, 3*time.Second)
			})

			By("then the virtual machine is deleted")
			MustVMDeleted(vmController, vmNamespace, vmName)

			By("and the spare disk is also deleted")
			MustPVCDeleted(pvcController, vmNamespace, testVMRemoveDiskName)
		})
	})
})
