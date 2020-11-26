package api

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/pointer"

	kubevirtv1alpha3 "kubevirt.io/client-go/api/v1alpha3"

	apivm "github.com/rancher/harvester/pkg/api/vm"
	"github.com/rancher/harvester/pkg/config"
	ctlkubevirtv1alpha3 "github.com/rancher/harvester/pkg/generated/controllers/kubevirt.io/v1alpha3"
	. "github.com/rancher/harvester/tests/framework/dsl"
	"github.com/rancher/harvester/tests/framework/helper"
)

const (
	testVMGenerateName = "test-"

	testContainerDiskImageName       = "kubevirt/cirros-registry-disk-demo:latest"
	testContainerDiskImagePullPolicy = corev1.PullIfNotPresent

	testVMDiskOneName = "datavolume1"
	testVMDiskOneBus  = "virtio"

	testVMDiskTwoName = "datavolume2"
	testVMDiskTwoBus  = "sata"

	testVMCPUCores = 1
	testVMMemory   = "256Mi"

	testVMInterfaceOneName  = "default"
	testVMInterfaceOneModel = "virtio"

	testVMNetworkOneName = "default"
)

var _ = Describe("verify vm apis", func() {

	var (
		vmNamespace   string
		scaled        *config.Scaled
		vmController  ctlkubevirtv1alpha3.VirtualMachineController
		vmiController ctlkubevirtv1alpha3.VirtualMachineInstanceController
	)

	BeforeEach(func() {
		vmNamespace = "default"
		scaled = harvester.Scaled()
		vmController = scaled.VirtFactory.Kubevirt().V1alpha3().VirtualMachine()
		vmiController = scaled.VirtFactory.Kubevirt().V1alpha3().VirtualMachineInstance()

	})

	Cleanup(func() {
		scaled := config.ScaledWithContext(harvester.Context)
		vmController := scaled.VirtFactory.Kubevirt().V1alpha3().VirtualMachine()
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

	Context("operate via steve api", func() {

		var vmsAPI string

		BeforeEach(func() {

			vmsAPI = helper.BuildAPIURL("v1", "kubevirt.io.virtualmachines")

		})

		Specify("verify the start action", func() {

			By("given a stopped virtual machine")
			vm, err := vmController.Create(NewTestingVirtualMachine(vmNamespace, false, false))
			MustNotError(err)
			vmName := vm.Name
			HasNoneVMI(vmController, vmNamespace, vmName, vmiController)

			By("when call start action")
			var (
				respCode int
				respBody []byte
			)
			err = helper.NewHTTPClient().
				POST(fmt.Sprintf("%s/%s/%s?action=start", vmsAPI, vmNamespace, vmName)).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			MustRespCodeIs(http.StatusNoContent, "post start action", err, respCode, respBody)

			By("then the virtual machine is started")
			MustVMReady(vmController, vmNamespace, vmName)
		})

		Specify("verify the stop action", func() {

			By("given a started virtual machine")
			vm, err := vmController.Create(NewTestingVirtualMachine(vmNamespace, true, false))
			MustNotError(err)
			vmName := vm.Name
			MustVMReady(vmController, vmNamespace, vmName)

			By("when call stop action")
			var (
				respCode int
				respBody []byte
			)
			err = helper.NewHTTPClient().
				POST(fmt.Sprintf("%s/%s/%s?action=stop", vmsAPI, vmNamespace, vmName)).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			MustRespCodeIs(http.StatusNoContent, "post stop action", err, respCode, respBody)

			By("then the virtual machine is stopped")
			HasNoneRunningVMI(vmController, vmNamespace, vmName, vmiController)
		})

		Specify("verify the restart action", func() {

			By("given a started virtual machine")
			vm, err := vmController.Create(NewTestingVirtualMachine(vmNamespace, true, false))
			MustNotError(err)
			vmName := vm.Name
			var vmiUID string
			AfterVMIReady(vmController, vmNamespace, vmName, vmiController,
				func(vmi *kubevirtv1alpha3.VirtualMachineInstance) bool {
					if vmi.IsRunning() {
						vmiUID = string(vmi.UID)
						return true
					}
					return false
				})

			By("when call restart action")
			var (
				respCode int
				respBody []byte
			)
			err = helper.NewHTTPClient().
				POST(fmt.Sprintf("%s/%s/%s?action=restart", vmsAPI, vmNamespace, vmName)).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			MustRespCodeIs(http.StatusNoContent, "post restart action", err, respCode, respBody)

			By("then the virtual machine is restarted")
			AfterVMIReady(vmController, vmNamespace, vmName, vmiController,
				func(vmi *kubevirtv1alpha3.VirtualMachineInstance) bool {
					return vmi.IsRunning() && vmiUID != string(vmi.UID)
				})
		})

		Context("verify the ejectCdRom action", func() {

			It("should fail if there are not CdRoms in the virtual machine", func() {

				By("given a started virtual machine without any CdRoms")
				vm, err := vmController.Create(NewTestingVirtualMachine(vmNamespace, true, false))
				MustNotError(err)
				vmName := vm.Name
				MustVMReady(vmController, vmNamespace, vmName)

				By("when call ejectCdRom action")
				var (
					respCode int
					respBody []byte
					reqBody  apivm.EjectCdRomActionInput
				)
				err = helper.NewHTTPClient().
					POST(fmt.Sprintf("%s/%s/%s?action=ejectCdRom", vmsAPI, vmNamespace, vmName)).
					SetJSON(reqBody).
					BindBody(&respBody).
					Code(&respCode).
					Do()
				MustRespCodeIs(http.StatusUnprocessableEntity, "ejectCdRom", err, respCode, respBody)
			})

			It("should fail if request without any CdRoms", func() {

				By("given a started virtual machine with one CdRom")
				vm, err := vmController.Create(NewTestingVirtualMachine(vmNamespace, true, true))
				MustNotError(err)
				vmName := vm.Name
				MustVMReady(vmController, vmNamespace, vmName)

				By("when call ejectCdRom action without any CdRoms")
				var (
					respCode int
					respBody []byte
					reqBody  apivm.EjectCdRomActionInput
				)
				err = helper.NewHTTPClient().
					POST(fmt.Sprintf("%s/%s/%s?action=ejectCdRom", vmsAPI, vmNamespace, vmName)).
					SetJSON(reqBody).
					BindBody(&respBody).
					Code(&respCode).
					Do()
				MustRespCodeIs(http.StatusUnprocessableEntity, "ejectCdRom", err, respCode, respBody)

			})

			It("should fail if the ejected target is not CdRom", func() {

				By("given a started virtual machine with one CdRom")
				vm, err := vmController.Create(NewTestingVirtualMachine(vmNamespace, true, true))
				MustNotError(err)
				vmName := vm.Name
				MustVMReady(vmController, vmNamespace, vmName)

				By("when call ejectCdRom action with a not existed CdRom")
				var (
					respCode int
					respBody []byte
					reqBody  = apivm.EjectCdRomActionInput{
						DiskNames: []string{testVMDiskOneName},
					}
				)
				err = helper.NewHTTPClient().
					POST(fmt.Sprintf("%s/%s/%s?action=ejectCdRom", vmsAPI, vmNamespace, vmName)).
					SetJSON(reqBody).
					BindBody(&respBody).
					Code(&respCode).
					Do()
				MustRespCodeIs(http.StatusInternalServerError, "ejectCdRom", err, respCode, respBody)
			})

			It("should eject the CdRom", func() {

				By("given a started virtual machine with one CdRom")
				vm, err := vmController.Create(NewTestingVirtualMachine(vmNamespace, true, true))
				MustNotError(err)
				vmName := vm.Name
				MustVMReady(vmController, vmNamespace, vmName)

				By("when call ejectCdRom action")
				var (
					respCode int
					respBody []byte
					reqBody  = apivm.EjectCdRomActionInput{
						DiskNames: []string{testVMDiskTwoName},
					}
				)
				err = helper.NewHTTPClient().
					POST(fmt.Sprintf("%s/%s/%s?action=ejectCdRom", vmsAPI, vmNamespace, vmName)).
					SetJSON(reqBody).
					BindBody(&respBody).
					Code(&respCode).
					Do()
				MustRespCodeIs(http.StatusNoContent, "post ejectCdRom action", err, respCode, respBody)

				By("then the CdRom is ejected")
				AfterVMIReady(vmController, vmNamespace, vmName, vmiController,
					func(vmi *kubevirtv1alpha3.VirtualMachineInstance) bool {
						if vmi.IsRunning() {
							for _, disk := range vmi.Spec.Domain.Devices.Disks {
								if disk.CDRom != nil && disk.Name == testVMDiskTwoName {
									return false
								}
							}
							return true
						}
						return false
					})
			})

		})

	})

})

func NewTestingVirtualMachine(namespace string, started, withCDRom bool) *kubevirtv1alpha3.VirtualMachine {
	var createDisks = func() []kubevirtv1alpha3.Disk {
		var disks = []kubevirtv1alpha3.Disk{
			{
				Name: testVMDiskOneName,
				DiskDevice: kubevirtv1alpha3.DiskDevice{
					Disk: &kubevirtv1alpha3.DiskTarget{
						Bus: testVMDiskOneBus,
					},
				},
			},
		}

		if withCDRom {
			disks = append(disks, kubevirtv1alpha3.Disk{
				Name: testVMDiskTwoName,
				DiskDevice: kubevirtv1alpha3.DiskDevice{
					CDRom: &kubevirtv1alpha3.CDRomTarget{
						Bus: testVMDiskTwoBus,
					},
				},
			})
		}
		return disks
	}

	var createVolumes = func() []kubevirtv1alpha3.Volume {
		var volumes = []kubevirtv1alpha3.Volume{
			{
				Name: testVMDiskOneName,
				VolumeSource: kubevirtv1alpha3.VolumeSource{
					ContainerDisk: &kubevirtv1alpha3.ContainerDiskSource{
						Image:           testContainerDiskImageName,
						ImagePullPolicy: testContainerDiskImagePullPolicy,
					},
				},
			},
		}

		if withCDRom {
			volumes = append(volumes, kubevirtv1alpha3.Volume{
				Name: testVMDiskTwoName,
				VolumeSource: kubevirtv1alpha3.VolumeSource{
					ContainerDisk: &kubevirtv1alpha3.ContainerDiskSource{
						Image:           testContainerDiskImageName,
						ImagePullPolicy: testContainerDiskImagePullPolicy,
					},
				},
			})
		}

		return volumes
	}

	return &kubevirtv1alpha3.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: testVMGenerateName,
			Labels:       testResourceLabels,
		},
		Spec: kubevirtv1alpha3.VirtualMachineSpec{
			Running: pointer.BoolPtr(started),
			Template: &kubevirtv1alpha3.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1alpha3.VirtualMachineInstanceSpec{
					Domain: kubevirtv1alpha3.DomainSpec{
						CPU: &kubevirtv1alpha3.CPU{
							Cores: testVMCPUCores,
						},
						Devices: kubevirtv1alpha3.Devices{
							Disks: createDisks(),
							Interfaces: []kubevirtv1alpha3.Interface{
								{
									Name:  testVMInterfaceOneName,
									Model: testVMInterfaceOneModel,
									InterfaceBindingMethod: kubevirtv1alpha3.InterfaceBindingMethod{
										Bridge: &kubevirtv1alpha3.InterfaceBridge{},
									},
								},
							},
						},
						Resources: kubevirtv1alpha3.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse(testVMMemory),
							},
						},
					},
					Networks: []kubevirtv1alpha3.Network{
						{
							Name: testVMNetworkOneName,
							NetworkSource: kubevirtv1alpha3.NetworkSource{
								Pod: &kubevirtv1alpha3.PodNetwork{},
							},
						},
					},
					Volumes: createVolumes(),
				},
			},
		},
	}
}
