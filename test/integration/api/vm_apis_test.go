package api

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/guonaihong/gout"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/pointer"
	kubevirtapis "kubevirt.io/client-go/api/v1alpha3"

	"github.com/rancher/harvester/pkg/api/vm"
	"github.com/rancher/harvester/pkg/config"
	. "github.com/rancher/harvester/test/framework/envtest/dsl"
)

var _ = Describe("verify vm apis", func() {

	var vmNamespace string

	BeforeEach(func() {

		vmNamespace = "default"

	})

	Cleanup(func() {

		var vmCli = harvester.Scaled().VirtFactory.Kubevirt().V1alpha3().VirtualMachine()
		var list, err = vmCli.List(vmNamespace, metav1.ListOptions{LabelSelector: labels.FormatLabels(map[string]string{"test.harvester.cattle.io": "for-test"})})
		if err != nil {
			GinkgoT().Logf("failed to list tested vms, %v", err)
			return
		}
		for _, item := range list.Items {
			var err = vmCli.Delete(item.Namespace, item.Name, &metav1.DeleteOptions{})
			if err != nil {
				GinkgoT().Logf("failed to delete tested vm %s/%s, %v", item.Namespace, item.Name, err)
			}
		}

	})

	Context("operate via steve api", func() {

		var vmsAPI string

		BeforeEach(func() {

			vmsAPI = fmt.Sprintf("https://localhost:%d/v1/kubevirt.io.virtualmachines", config.HTTPSListenPort)

		})

		Specify("verify the start action", func() {

			By("given a stopped virtual machine")
			var vmCr = createTestingVirtualMachineCR(vmNamespace, false, false)
			var vmCli = harvester.Scaled().VirtFactory.Kubevirt().V1alpha3().VirtualMachine()
			vmCr, err := vmCli.Create(vmCr)
			Expect(err).ShouldNot(HaveOccurred())
			var vmCrName = vmCr.Name
			var vmiCli = harvester.Scaled().VirtFactory.Kubevirt().V1alpha3().VirtualMachineInstance()
			Eventually(func() bool {
				var vmCr, err = vmCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
				if err != nil {
					GinkgoT().Logf("failed to get virtual machine: %v", err)
					return false
				}
				if !vmCr.Status.Ready {
					var _, err = vmiCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
					if err != nil && apierrors.IsNotFound(err) {
						return true
					}
					return false
				}
				return false
			}, 300, 2).Should(BeTrue())

			By("when call start action")
			var (
				respCode int
				respBody []byte
			)
			err = gout.
				New(&http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					},
				}).
				POST(fmt.Sprintf("%s/%s/%s?action=start", vmsAPI, vmNamespace, vmCrName)).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			Expect(err).ShouldNot(HaveOccurred())
			if respCode != http.StatusNoContent {
				GinkgoT().Errorf("failed to post start action, response with %d, %v", respCode, string(respBody))
			}

			By("then the virtual machine is started")
			Eventually(func() bool {
				var vmCr, err = vmCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
				if err != nil {
					GinkgoT().Logf("failed to get virtual machine: %v", err)
					return false
				}
				return vmCr.Status.Ready
			}, 300, 2).Should(BeTrue())
		})

		Specify("verify the stop action", func() {

			By("given a started virtual machine")
			var vmCr = createTestingVirtualMachineCR(vmNamespace, true, false)
			var vmCli = harvester.Scaled().VirtFactory.Kubevirt().V1alpha3().VirtualMachine()
			vmCr, err := vmCli.Create(vmCr)
			Expect(err).ShouldNot(HaveOccurred())
			var vmCrName = vmCr.Name
			var vmiCli = harvester.Scaled().VirtFactory.Kubevirt().V1alpha3().VirtualMachineInstance()
			Eventually(func() bool {
				var vmCr, err = vmCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
				if err != nil {
					GinkgoT().Logf("failed to get virtual machine: %v", err)
					return false
				}
				return vmCr.Status.Ready
			}, 300, 2).Should(BeTrue())

			By("when call stop action")
			var (
				respCode int
				respBody []byte
			)
			err = gout.
				New(&http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					},
				}).
				POST(fmt.Sprintf("%s/%s/%s?action=stop", vmsAPI, vmNamespace, vmCrName)).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			Expect(err).ShouldNot(HaveOccurred())
			if respCode != http.StatusNoContent {
				GinkgoT().Errorf("failed to post stop action, response with %d, %v", respCode, string(respBody))
			}

			By("then the virtual machine is stopped")
			Eventually(func() bool {
				var vmCr, err = vmCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
				if err != nil {
					GinkgoT().Logf("failed to get virtual machine: %v", err)
					return false
				}
				if !vmCr.Status.Ready {
					var vmiCr, err = vmiCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
					if err != nil && apierrors.IsNotFound(err) {
						return true
					}
					if vmiCr.DeletionTimestamp != nil {
						return true
					}
					return false
				}
				return false
			}, 300, 2).Should(BeTrue())
		})

		Specify("verify the restart action", func() {

			By("given a started virtual machine")
			var vmCr = createTestingVirtualMachineCR(vmNamespace, true, false)
			var vmCli = harvester.Scaled().VirtFactory.Kubevirt().V1alpha3().VirtualMachine()
			vmCr, err := vmCli.Create(vmCr)
			Expect(err).ShouldNot(HaveOccurred())
			var vmCrName = vmCr.Name
			var vmiCli = harvester.Scaled().VirtFactory.Kubevirt().V1alpha3().VirtualMachineInstance()
			var vmiCrUID string
			Eventually(func() bool {
				var vmCr, err = vmCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
				if err != nil {
					GinkgoT().Logf("failed to get virtual machine: %v", err)
					return false
				}
				if vmCr.Status.Ready {
					var vmiCr, err = vmiCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
					if err != nil {
						GinkgoT().Logf("failed to get virtual machine instance: %v", err)
						return false
					}
					if vmiCr.IsRunning() {
						vmiCrUID = string(vmiCr.UID)
						return true
					}
				}
				return false
			}, 300, 2).Should(BeTrue())

			By("when call restart action")
			var (
				respCode int
				respBody []byte
			)
			err = gout.
				New(&http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					},
				}).
				POST(fmt.Sprintf("%s/%s/%s?action=restart", vmsAPI, vmNamespace, vmCrName)).
				BindBody(&respBody).
				Code(&respCode).
				Do()
			Expect(err).ShouldNot(HaveOccurred())
			if respCode != http.StatusNoContent {
				GinkgoT().Errorf("failed to post restart action, response with %d, %v", respCode, string(respBody))
			}

			By("then the virtual machine is restarted")
			Eventually(func() bool {
				var vmCr, err = vmCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
				if err != nil {
					GinkgoT().Logf("failed to get virtual machine: %v", err)
					return false
				}
				if vmCr.Status.Ready {
					var vmiCr, err = vmiCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
					if err != nil {
						GinkgoT().Logf("failed to get virtual machine instance: %v", err)
						return false
					}
					return vmiCr.IsRunning() &&
						vmiCrUID != string(vmiCr.UID)
				}
				return false
			}, 300, 2).Should(BeTrue())
		})

		Context("verify the ejectCdRom action", func() {

			It("should fail if there are not CdRoms in the virtual machine", func() {

				By("given a started virtual machine without any CdRoms")
				var vmCr = createTestingVirtualMachineCR(vmNamespace, true, false)
				var vmCli = harvester.Scaled().VirtFactory.Kubevirt().V1alpha3().VirtualMachine()
				vmCr, err := vmCli.Create(vmCr)
				Expect(err).ShouldNot(HaveOccurred())
				var vmCrName = vmCr.Name
				Eventually(func() bool {
					var vmCr, err = vmCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
					if err != nil {
						GinkgoT().Logf("failed to get virtual machine: %v", err)
						return false
					}
					return vmCr.Status.Ready
				}, 300, 2).Should(BeTrue())

				By("when call ejectCdRom action")
				var (
					respCode int
					respBody []byte
					reqBody  vm.EjectCdRomActionInput
				)
				err = gout.
					New(&http.Client{
						Transport: &http.Transport{
							TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
						},
					}).
					POST(fmt.Sprintf("%s/%s/%s?action=ejectCdRom", vmsAPI, vmNamespace, vmCrName)).
					SetJSON(reqBody).
					BindBody(&respBody).
					Code(&respCode).
					Do()
				Expect(err).ShouldNot(HaveOccurred())

				By("then the action is failed as not found")
				// in fact, we cannot see ejectCdRom action in steve api
				Expect(respCode).Should(Equal(http.StatusBadRequest))

			})

			It("should fail if request without any CdRoms", func() {

				By("given a started virtual machine with one CdRom")
				var vmCr = createTestingVirtualMachineCR(vmNamespace, true, true)
				var vmCli = harvester.Scaled().VirtFactory.Kubevirt().V1alpha3().VirtualMachine()
				vmCr, err := vmCli.Create(vmCr)
				Expect(err).ShouldNot(HaveOccurred())
				var vmCrName = vmCr.Name
				Eventually(func() bool {
					var vmCr, err = vmCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
					if err != nil {
						GinkgoT().Logf("failed to get virtual machine: %v", err)
						return false
					}
					return vmCr.Status.Ready
				}, 300, 2).Should(BeTrue())

				By("when call ejectCdRom action without any CdRoms")
				var (
					respCode int
					respBody []byte
					reqBody  vm.EjectCdRomActionInput
				)
				err = gout.
					New(&http.Client{
						Transport: &http.Transport{
							TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
						},
					}).
					POST(fmt.Sprintf("%s/%s/%s?action=ejectCdRom", vmsAPI, vmNamespace, vmCrName)).
					SetJSON(reqBody).
					BindBody(&respBody).
					Code(&respCode).
					Do()
				Expect(err).ShouldNot(HaveOccurred())

				By("then the action is failed as empty diskNames input")
				Expect(respCode).Should(Equal(http.StatusBadRequest))
				Expect(respBody).Should(Equal([]byte(`Parameter diskNames is empty`)))

			})

			It("should fail if the ejected target is not CdRom", func() {

				By("given a started virtual machine with one CdRom")
				var vmCr = createTestingVirtualMachineCR(vmNamespace, true, true)
				var vmCli = harvester.Scaled().VirtFactory.Kubevirt().V1alpha3().VirtualMachine()
				vmCr, err := vmCli.Create(vmCr)
				Expect(err).ShouldNot(HaveOccurred())
				var vmCrName = vmCr.Name
				Eventually(func() bool {
					var vmCr, err = vmCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
					if err != nil {
						GinkgoT().Logf("failed to get virtual machine: %v", err)
						return false
					}
					return vmCr.Status.Ready
				}, 300, 2).Should(BeTrue())

				By("when call ejectCdRom action with a not existed CdRom")
				var (
					respCode int
					respBody []byte
					reqBody  = vm.EjectCdRomActionInput{
						DiskNames: []string{"datavolume1"},
					}
				)
				err = gout.
					New(&http.Client{
						Transport: &http.Transport{
							TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
						},
					}).
					POST(fmt.Sprintf("%s/%s/%s?action=ejectCdRom", vmsAPI, vmNamespace, vmCrName)).
					SetJSON(reqBody).
					BindBody(&respBody).
					Code(&respCode).
					Do()
				Expect(err).ShouldNot(HaveOccurred())

				By("then the action is failed as there is not target diskname")
				Expect(respCode).Should(Equal(http.StatusInternalServerError))
				Expect(respBody).Should(Equal([]byte(`disk datavolume1 isn't a CD-ROM disk`)))

			})

			It("should eject the CdRom", func() {

				By("given a started virtual machine with one CdRom")
				var vmCr = createTestingVirtualMachineCR(vmNamespace, true, true)
				var vmCli = harvester.Scaled().VirtFactory.Kubevirt().V1alpha3().VirtualMachine()
				vmCr, err := vmCli.Create(vmCr)
				Expect(err).ShouldNot(HaveOccurred())
				var vmCrName = vmCr.Name
				var vmiCli = harvester.Scaled().VirtFactory.Kubevirt().V1alpha3().VirtualMachineInstance()
				Eventually(func() bool {
					var vmCr, err = vmCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
					if err != nil {
						GinkgoT().Logf("failed to get virtual machine: %v", err)
						return false
					}
					return vmCr.Status.Ready
				}, 300, 1).Should(BeTrue())

				By("when call ejectCdRom action")
				var (
					respCode int
					respBody []byte
					reqBody  = vm.EjectCdRomActionInput{
						DiskNames: []string{"datavolume2"},
					}
				)
				err = gout.
					New(&http.Client{
						Transport: &http.Transport{
							TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
						},
					}).
					POST(fmt.Sprintf("%s/%s/%s?action=ejectCdRom", vmsAPI, vmNamespace, vmCrName)).
					SetJSON(reqBody).
					BindBody(&respBody).
					Code(&respCode).
					Do()
				Expect(err).ShouldNot(HaveOccurred())
				if respCode != http.StatusNoContent {
					GinkgoT().Errorf("failed to post ejectCdRom action, response with %d, %v", respCode, string(respBody))
				}

				By("then the CdRom is ejected")
				Eventually(func() bool {
					var vmCr, err = vmCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
					if err != nil {
						GinkgoT().Logf("failed to get virtual machine: %v", err)
						return false
					}
					if vmCr.Status.Ready {
						var vmiCr, err = vmiCli.Get(vmNamespace, vmCrName, metav1.GetOptions{})
						if err != nil {
							GinkgoT().Logf("failed to get virtual machine instance: %v", err)
							return false
						}
						if vmiCr.IsRunning() {
							for _, disk := range vmiCr.Spec.Domain.Devices.Disks {
								if disk.CDRom != nil && disk.Name == "datavolume2" {
									return false
								}
							}
							return true
						}
					}
					return false
				}, 300, 1).Should(BeTrue())

			})

		})

	})

})

func createTestingVirtualMachineCR(namespace string, started, withCDRom bool) *kubevirtapis.VirtualMachine {
	var createDisks = func() []kubevirtapis.Disk {
		var disks = []kubevirtapis.Disk{
			{
				Name: "datavolume1",
				DiskDevice: kubevirtapis.DiskDevice{
					Disk: &kubevirtapis.DiskTarget{
						Bus: "virtio",
					},
				},
			},
		}

		if withCDRom {
			disks = append(disks, kubevirtapis.Disk{
				Name: "datavolume2",
				DiskDevice: kubevirtapis.DiskDevice{
					CDRom: &kubevirtapis.CDRomTarget{
						Bus: "sata",
					},
				},
			})
		}
		return disks
	}

	var createVolumes = func() []kubevirtapis.Volume {
		var volumes = []kubevirtapis.Volume{
			{
				Name: "datavolume1",
				VolumeSource: kubevirtapis.VolumeSource{
					ContainerDisk: &kubevirtapis.ContainerDiskSource{
						Image:           "kubevirt/cirros-registry-disk-demo:latest",
						ImagePullPolicy: corev1.PullIfNotPresent,
					},
				},
			},
		}

		if withCDRom {
			volumes = append(volumes, kubevirtapis.Volume{
				Name: "datavolume2",
				VolumeSource: kubevirtapis.VolumeSource{
					ContainerDisk: &kubevirtapis.ContainerDiskSource{
						Image:           "kubevirt/cirros-registry-disk-demo:latest",
						ImagePullPolicy: corev1.PullIfNotPresent,
					},
				},
			})
		}

		return volumes
	}

	return &kubevirtapis.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: "test-",
			Labels:       map[string]string{"test.harvester.cattle.io": "for-test"},
		},
		Spec: kubevirtapis.VirtualMachineSpec{
			Running: pointer.BoolPtr(started),
			Template: &kubevirtapis.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtapis.VirtualMachineInstanceSpec{
					Domain: kubevirtapis.DomainSpec{
						CPU: &kubevirtapis.CPU{
							Cores: 1,
						},
						Devices: kubevirtapis.Devices{
							Disks: createDisks(),
							Interfaces: []kubevirtapis.Interface{
								{
									Name:  "default",
									Model: "virtio",
									InterfaceBindingMethod: kubevirtapis.InterfaceBindingMethod{
										Bridge: &kubevirtapis.InterfaceBridge{},
									},
								},
							},
						},
						Resources: kubevirtapis.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
					},
					Networks: []kubevirtapis.Network{
						{
							Name: "default",
							NetworkSource: kubevirtapis.NetworkSource{
								Pod: &kubevirtapis.PodNetwork{},
							},
						},
					},
					Volumes: createVolumes(),
				},
			},
		},
	}
}
