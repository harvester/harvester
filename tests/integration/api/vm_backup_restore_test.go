package api_test

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	apivm "github.com/harvester/harvester/pkg/api/vm"
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/builder"
	"github.com/harvester/harvester/pkg/config"
	ctldatavolumev1 "github.com/harvester/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	. "github.com/harvester/harvester/tests/framework/dsl"
	"github.com/harvester/harvester/tests/framework/env"
	"github.com/harvester/harvester/tests/framework/fuzz"
	"github.com/harvester/harvester/tests/framework/helper"
)

var _ = Describe("verify vm backup & restore APIs", func() {
	if env.IsE2ETestsEnabled() {
		var (
			scaled            *config.Scaled
			backupController  ctlharvesterv1.VirtualMachineBackupController
			restoreController ctlharvesterv1.VirtualMachineRestoreController
			vmController      ctlkubevirtv1.VirtualMachineController
			vmiController     ctlkubevirtv1.VirtualMachineInstanceController
			dvController      ctldatavolumev1.DataVolumeController
			settingController ctllonghornv1.SettingController
			podController     ctlcorev1.PodController
			svcController     ctlcorev1.ServiceController
			backupNamespace   string
			sourceImage       string
			vmBackupTarget    string
		)

		BeforeEach(func() {
			scaled = harvester.Scaled()
			backupController = scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineBackup()
			restoreController = scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineRestore()
			vmController = scaled.VirtFactory.Kubevirt().V1().VirtualMachine()
			vmiController = scaled.VirtFactory.Kubevirt().V1().VirtualMachineInstance()
			dvController = scaled.CDIFactory.Cdi().V1beta1().DataVolume()
			settingController = scaled.LonghornFactory.Longhorn().V1beta1().Setting()
			podController = scaled.CoreFactory.Core().V1().Pod()
			svcController = scaled.CoreFactory.Core().V1().Service()
			backupNamespace = testVMNamespace
			sourceImage = "https://dl-cdn.alpinelinux.org/alpine/latest-stable/releases/x86_64/alpine-virt-3.14.0-x86_64.iso"
			vmBackupTarget = fmt.Sprintf("nfs://harvester-nfs-svc.%s:/opt/backupstore", backupNamespace)
		})

		Cleanup(func() {
			// cleanup backup vms
			vmList, err := vmController.List(backupNamespace, metav1.ListOptions{
				LabelSelector: labels.FormatLabels(testVMBackupLabels)})
			if err != nil {
				GinkgoT().Logf("failed to list backup & restore tested vms, %v", err)
				return
			}
			for _, item := range vmList.Items {
				if err = vmController.Delete(item.Namespace, item.Name, &metav1.DeleteOptions{}); err != nil {
					GinkgoT().Logf("failed to delete backup & restore tested vm %s/%s, %v", item.Namespace, item.Name, err)
				}
			}

			// cleanup vm backup & restore
			backups, err := backupController.List(backupNamespace, metav1.ListOptions{})
			if err != nil {
				GinkgoT().Logf("failed to list vm backups, %v", err)
				return
			}
			for _, backup := range backups.Items {
				if err = backupController.Delete(backup.Namespace, backup.Name, &metav1.DeleteOptions{}); err != nil {
					GinkgoT().Logf("failed to delete backup %s/%s, %v", backup.Namespace, backup.Name, err)
				}
			}

			restores, err := restoreController.List(backupNamespace, metav1.ListOptions{})
			if err != nil {
				GinkgoT().Logf("failed to list vm backups, %v", err)
				return
			}
			for _, restore := range restores.Items {
				if err = restoreController.Delete(restore.Namespace, restore.Name, &metav1.DeleteOptions{}); err != nil {
					GinkgoT().Logf("failed to delete backup %s/%s, %v", restore.Namespace, restore.Name, err)
				}
			}
			dvList, err := dvController.List(backupNamespace, metav1.ListOptions{
				LabelSelector: labels.FormatLabels(testVMBackupLabels)})
			if err != nil {
				GinkgoT().Logf("failed to list tested dvs, %v", err)
				return
			}
			for _, item := range dvList.Items {
				if err = dvController.Delete(item.Namespace, item.Name, &metav1.DeleteOptions{}); err != nil {
					GinkgoT().Logf("failed to delete tested dv %s/%s, %v", item.Namespace, item.Name, err)
				}
			}
		})

		Context("operate via steve API", func() {

			var vmsAPI, volumeAPI, restoresAPI string

			BeforeEach(func() {
				vmsAPI = helper.BuildAPIURL("v1", "kubevirt.io.virtualmachines", options.HTTPSListenPort)
				volumeAPI = helper.BuildAPIURL("v1", "cdi.kubevirt.io.datavolumes", options.HTTPSListenPort)
				restoresAPI = helper.BuildAPIURL("v1", "harvesterhci.io.virtualmachinerestores", options.HTTPSListenPort)
			})

			Specify("config the vm backup server", func() {
				var nfsName string
				var err error
				By("then create testing nfs server if not exist", func() {
					nfsName, err = createLonghornTestingNFS(svcController, podController, backupNamespace)
					MustNotError(err)
				})

				By("then validate nfs server", func() {
					MustFinallyBeTrue(func() bool {
						pod, err := podController.Get(backupNamespace, nfsName, metav1.GetOptions{})
						MustNotError(err)
						return pod.Status.Phase == corev1.PodRunning
					}, 120, 5)
				})

				By("then config the longhorn backup target", func() {
					backupSetting, err := settingController.Get("longhorn-system", "backup-target", metav1.GetOptions{})
					MustNotError(err)
					backupSetting.Value = vmBackupTarget
					_, err = settingController.Update(backupSetting)
					MustNotError(err)
				})
			})

			Specify("verify vm backup api", func() {
				By("when create a VM using a embedded and a manually attached data volumes")
				vmName := testVMGenerateName + fuzz.String(5)

				By("first manually create a data volume")
				dvName := vmName + fuzz.String(5)
				diskName2 := dvName
				respCode, respBody, err := helper.PostObject(volumeAPI, NewDataVolume(testVMBackupLabels, backupNamespace, dvName))
				MustRespCodeIs(http.StatusCreated, "create dv", err, respCode, respBody)
				MustDataVolumeSucceeded(dvController, backupNamespace, dvName)

				By("then create a VM with an embedded data volume and attach the data volume created during previous step")
				dataVolumeOption := &builder.DataVolumeOption{
					VolumeMode:  builder.PersistentVolumeModeBlock,
					AccessMode:  builder.PersistentVolumeAccessModeReadWriteMany,
					DownloadURL: sourceImage,
				}
				vm, err := NewDefaultTestVMBuilder(testVMBackupLabels).Name(vmName).
					NetworkInterface(testVMInterfaceName, testVMInterfaceModel, "", builder.NetworkInterfaceTypeMasquerade, "").
					DataVolumeDisk("root-disk", testVMDefaultDiskBus, false, 1, "2Gi", "", dataVolumeOption).
					ExistingVolumeDisk(diskName2, testVMDefaultDiskBus, false, 0, dvName).
					Run(true).VM()
				MustNotError(err)
				respCode, respBody, err = helper.PostObject(vmsAPI, vm)
				MustRespCodeIs(http.StatusCreated, "create vm", err, respCode, respBody)

				By("then the vm is up and running")
				MustVMIRunning(vmController, backupNamespace, vmName, vmiController)
				vm, err = vmController.Get(backupNamespace, vmName, metav1.GetOptions{})
				MustNotError(err)

				// backup
				vmURL := helper.BuildResourceURL(vmsAPI, backupNamespace, vmName)
				backupName := "backup" + fuzz.String(5)
				By("call vm backup action")
				respCode, respBody, err = helper.PostObjectAction(vmURL, apivm.BackupInput{Name: backupName}, "backup")
				MustRespCodeIs(http.StatusNoContent, "post backup action done", err, respCode, respBody)

				By("then validate vm backup status", func() {
					MustFinallyBeTrue(func() bool {
						backup, err := backupController.Get(backupNamespace, backupName, metav1.GetOptions{})
						MustNotError(err)
						return backup.Status != nil && *backup.Status.ReadyToUse
					}, 120, 5)
				})

				By("set backup vm to be stopped", func() {
					if vm.Status.Ready {
						respCode, respBody, err := helper.PostAction(fmt.Sprintf("%s/%s/%s", vmsAPI, backupNamespace, vmName), "stop")
						MustRespCodeIs(http.StatusNoContent, "stop vm action done", err, respCode, respBody)
						HasNoneVMI(vmController, backupNamespace, vmName, vmiController)
					}
				})

				restoreName := "restore-" + fuzz.String(3)
				By("then create a vm restore", func() {
					respCode, respBody, err := helper.PostObjectAction(vmURL, apivm.RestoreInput{
						Name:       restoreName,
						BackupName: backupName,
					}, "restore")

					MustRespCodeIs(http.StatusNoContent, "restore vm action done", err, respCode, respBody)
				})

				By("then validate restore an existing vm", func() {
					MustFinallyBeTrue(func() bool {
						restore, err := restoreController.Get(backupNamespace, restoreName, metav1.GetOptions{})
						MustNotError(err)
						return restore.Status != nil && *restore.Status.Complete
					}, 120, 5)
					MustVMIRunning(vmController, backupNamespace, vm.Name, vmiController)

					MustFinallyBeTrue(func() bool {
						vm, err := vmController.Get(backupNamespace, vm.Name, metav1.GetOptions{})
						MustNotError(err)
						for _, vol := range vm.Spec.Template.Spec.Volumes {
							// ensure all volumes are stored as data volume
							if vol.DataVolume == nil {
								return false
							}
						}
						return true
					}, 30, 5)
				})

				By("then validate restore an new vm", func() {
					restoreName := "restore-" + fuzz.String(3)
					vmName := "new-vm" + fuzz.String(3)
					newVM := harvesterv1.VirtualMachineRestore{
						ObjectMeta: metav1.ObjectMeta{
							Name:      restoreName,
							Namespace: backupNamespace,
							Labels:    testVMBackupLabels,
						},
						Spec: harvesterv1.VirtualMachineRestoreSpec{
							Target: corev1.TypedLocalObjectReference{
								APIGroup: &harvesterv1.SchemeGroupVersion.Group,
								Kind:     "VirtualMachine",
								Name:     vmName,
							},
							VirtualMachineBackupName: backupName,
							NewVM:                    true,
						},
					}
					respCode, respBody, err := helper.PostObject(restoresAPI, newVM)
					MustRespCodeIs(http.StatusCreated, "post new restore vm done", err, respCode, respBody)

					MustFinallyBeTrue(func() bool {
						restore, err := restoreController.Get(backupNamespace, restoreName, metav1.GetOptions{})
						MustNotError(err)
						return restore.Status != nil && *restore.Status.Complete
					}, 120, 5)

					MustVMIRunning(vmController, backupNamespace, vmName, vmiController)

					MustFinallyBeTrue(func() bool {
						vm, err := vmController.Get(backupNamespace, vmName, metav1.GetOptions{})
						MustNotError(err)
						for _, vol := range vm.Spec.Template.Spec.Volumes {
							// ensure all volumes are stored as data volume
							if vol.DataVolume == nil {
								return false
							}
						}
						return true
					}, 30, 5)
				})
			})
		})
	} else {
		logrus.Infof("skip vm backup & restore test by e2e tests")
	}
})

func createLonghornTestingNFS(svcController ctlcorev1.ServiceController, podController ctlcorev1.PodController, namespace string) (string, error) {
	testNFSServerLabels := map[string]string{
		"harvester.test.io/name": "harvester-nfs",
	}
	name := "harvester-nfs"
	privileged := true

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    testNFSServerLabels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:            "nfs",
					Image:           "janeczku/nfs-ganesha:latest",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Env: []corev1.EnvVar{
						{
							Name:  "EXPORT_ID",
							Value: "14",
						}, {
							Name:  "EXPORT_PATH",
							Value: "/opt/backupstore",
						}, {
							Name:  "PSEUDO_PATH",
							Value: "/opt/backupstore",
						},
					},
					Command: []string{
						"bash",
						"-c",
						"chmod 700 /opt/backupstore && /opt/start_nfs.sh | tee /var/log/ganesha.log",
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
						Capabilities: &corev1.Capabilities{
							Add: []corev1.Capability{
								"SYS_ADMIN",
								"DAC_READ_SEARCH",
							},
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "nfs-volume",
							MountPath: "/opt/backupstore",
						},
					},
					LivenessProbe: &corev1.Probe{
						InitialDelaySeconds: 5,
						PeriodSeconds:       5,
						Handler: corev1.Handler{
							Exec: &corev1.ExecAction{
								Command: []string{
									"bash",
									"-c",
									"grep \"No export entries found\" /var/log/ganesha.log > /dev/null 2>&1 ; [ $? -ne 0 ]",
								},
							},
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "nfs-volume",
				},
			},
		},
	}
	_, err := podController.Create(pod)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return "", err
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "harvester-nfs-svc",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector:  testNFSServerLabels,
			ClusterIP: "None",
		},
	}
	_, err = svcController.Create(svc)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return "", err
	}
	return name, nil
}
