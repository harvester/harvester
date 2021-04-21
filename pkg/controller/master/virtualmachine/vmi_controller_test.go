package virtualmachine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	kubevirtapis "kubevirt.io/client-go/api/v1"
	cdiapisalpha "kubevirt.io/containerized-data-importer/pkg/apis/core/v1alpha1"
	cdiapis "kubevirt.io/containerized-data-importer/pkg/apis/core/v1beta1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	kubevirttype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
	kubevirtctrl "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/ref"
)

func TestVMIController_UnsetOwnerOfDataVolumes(t *testing.T) {
	type input struct {
		key string
		vmi *kubevirtapis.VirtualMachineInstance
		vm  *kubevirtapis.VirtualMachine
		dv  *cdiapis.DataVolume
	}
	type output struct {
		vmi *kubevirtapis.VirtualMachineInstance
		err error
		dv  *cdiapis.DataVolume
	}
	var testFinalizers = []string{"wrangler.cattle.io/VMIController.UnsetOwnerOfDataVolumes"}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "ignore nil resource",
			given: input{
				key: "",
				vmi: nil,
				vm:  nil,
				dv:  nil,
			},
			expected: output{
				vmi: nil,
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "ignore none deleted resource",
			given: input{
				key: "default/test",
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vmi-uid",
						Finalizers: testFinalizers,
					},
				},
				vm: nil,
				dv: nil,
			},
			expected: output{
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vmi-uid",
						Finalizers: testFinalizers,
					},
				},
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "ignore if didn't own by any resources",
			given: input{
				key: "default/test",
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
					},
				},
				vm: nil,
				dv: nil,
			},
			expected: output{
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
					},
				},
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "ignore if owned by none virtualmachine resource",
			given: input{
				key: "default/test",
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "apps/v1",
								Kind:               "Deployment",
								Name:               "test",
								UID:                "fake-deployment-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
				},
				vm: nil,
				dv: nil,
			},
			expected: output{
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "apps/v1",
								Kind:               "Deployment",
								Name:               "test",
								UID:                "fake-deployment-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
				},
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "ignore if parent virtualmachine has not been found",
			given: input{
				key: "default/test",
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
				},
				vm: nil,
				dv: nil,
			},
			expected: output{
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
				},
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "ignore if parent virtualmachine has been deleted",
			given: input{
				key: "default/test",
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
				},
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: kubevirtapis.VirtualMachineSpec{
						Template: &kubevirtapis.VirtualMachineInstanceTemplateSpec{},
					},
				},
				dv: nil,
			},
			expected: output{
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
				},
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "ignore if parent virtualmachine's template is nil",
			given: input{
				key: "default/test",
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
				},
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
					},
					Spec: kubevirtapis.VirtualMachineSpec{
						Template: nil,
					},
				},
				dv: nil,
			},
			expected: output{
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
				},
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "ignore if both virtualmachine instance and parent virtualmachine have not any datavolumes",
			given: input{
				key: "default/test",
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
					Spec: kubevirtapis.VirtualMachineInstanceSpec{
						Domain: kubevirtapis.DomainSpec{
							Devices: kubevirtapis.Devices{
								Disks: []kubevirtapis.Disk{
									{
										Name:      "disk1",
										BootOrder: pointerToUint(1),
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
									{
										Name: "disk2",
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
								},
							},
						},
						Volumes: []kubevirtapis.Volume{
							{
								Name: "disk1",
								VolumeSource: kubevirtapis.VolumeSource{
									ContainerDisk: &kubevirtapis.ContainerDiskSource{
										Image: "vmidisks/fedora25:latest",
									},
								},
							},
							{
								Name: "disk2",
								VolumeSource: kubevirtapis.VolumeSource{
									EmptyDisk: &kubevirtapis.EmptyDiskSource{
										Capacity: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
				},
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
					},
					Spec: kubevirtapis.VirtualMachineSpec{
						Template: &kubevirtapis.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtapis.VirtualMachineInstanceSpec{
								Domain: kubevirtapis.DomainSpec{
									Devices: kubevirtapis.Devices{
										Disks: []kubevirtapis.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtapis.DiskDevice{
													Disk: &kubevirtapis.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtapis.DiskDevice{
													Disk: &kubevirtapis.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtapis.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtapis.VolumeSource{
											ContainerDisk: &kubevirtapis.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtapis.VolumeSource{
											EmptyDisk: &kubevirtapis.EmptyDiskSource{
												Capacity: resource.MustParse("2Gi"),
											},
										},
									},
								},
							},
						},
					},
				},
				dv: nil,
			},
			expected: output{
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
					Spec: kubevirtapis.VirtualMachineInstanceSpec{
						Domain: kubevirtapis.DomainSpec{
							Devices: kubevirtapis.Devices{
								Disks: []kubevirtapis.Disk{
									{
										Name:      "disk1",
										BootOrder: pointerToUint(1),
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
									{
										Name: "disk2",
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
								},
							},
						},
						Volumes: []kubevirtapis.Volume{
							{
								Name: "disk1",
								VolumeSource: kubevirtapis.VolumeSource{
									ContainerDisk: &kubevirtapis.ContainerDiskSource{
										Image: "vmidisks/fedora25:latest",
									},
								},
							},
							{
								Name: "disk2",
								VolumeSource: kubevirtapis.VolumeSource{
									EmptyDisk: &kubevirtapis.EmptyDiskSource{
										Capacity: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
				},
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "ignore if only increased datavolumes in parent virtualmachine",
			given: input{
				key: "default/test",
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
					Spec: kubevirtapis.VirtualMachineInstanceSpec{
						Domain: kubevirtapis.DomainSpec{
							Devices: kubevirtapis.Devices{
								Disks: []kubevirtapis.Disk{
									{
										Name:      "disk1",
										BootOrder: pointerToUint(1),
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
									{
										Name: "disk2",
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
								},
							},
						},
						Volumes: []kubevirtapis.Volume{
							{
								Name: "disk1",
								VolumeSource: kubevirtapis.VolumeSource{
									ContainerDisk: &kubevirtapis.ContainerDiskSource{
										Image: "vmidisks/fedora25:latest",
									},
								},
							},
							{
								Name: "disk2",
								VolumeSource: kubevirtapis.VolumeSource{
									EmptyDisk: &kubevirtapis.EmptyDiskSource{
										Capacity: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
				},
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
					},
					Spec: kubevirtapis.VirtualMachineSpec{
						DataVolumeTemplates: []kubevirtapis.DataVolumeTemplateSpec{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "dv-disk",
								},
								Spec: cdiapisalpha.DataVolumeSpec{
									Source: cdiapisalpha.DataVolumeSource{
										Blank: &cdiapisalpha.DataVolumeBlankImage{},
									},
									PVC: &corev1.PersistentVolumeClaimSpec{
										StorageClassName: pointer.StringPtr("default"),
										AccessModes: []corev1.PersistentVolumeAccessMode{
											corev1.ReadWriteOnce,
										},
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceStorage: resource.MustParse("2Gi"),
											},
										},
									},
								},
							},
						},
						Template: &kubevirtapis.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtapis.VirtualMachineInstanceSpec{
								Domain: kubevirtapis.DomainSpec{
									Devices: kubevirtapis.Devices{
										Disks: []kubevirtapis.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtapis.DiskDevice{
													Disk: &kubevirtapis.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtapis.DiskDevice{
													Disk: &kubevirtapis.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtapis.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtapis.VolumeSource{
											ContainerDisk: &kubevirtapis.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtapis.VolumeSource{
											EmptyDisk: &kubevirtapis.EmptyDiskSource{
												Capacity: resource.MustParse("2Gi"),
											},
										},
									},
									{
										Name: "disk3",
										VolumeSource: kubevirtapis.VolumeSource{
											DataVolume: &kubevirtapis.DataVolumeSource{
												Name: "dv-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				dv: nil,
			},
			expected: output{
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
					Spec: kubevirtapis.VirtualMachineInstanceSpec{
						Domain: kubevirtapis.DomainSpec{
							Devices: kubevirtapis.Devices{
								Disks: []kubevirtapis.Disk{
									{
										Name:      "disk1",
										BootOrder: pointerToUint(1),
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
									{
										Name: "disk2",
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
								},
							},
						},
						Volumes: []kubevirtapis.Volume{
							{
								Name: "disk1",
								VolumeSource: kubevirtapis.VolumeSource{
									ContainerDisk: &kubevirtapis.ContainerDiskSource{
										Image: "vmidisks/fedora25:latest",
									},
								},
							},
							{
								Name: "disk2",
								VolumeSource: kubevirtapis.VolumeSource{
									EmptyDisk: &kubevirtapis.EmptyDiskSource{
										Capacity: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
				},
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "ignore if the decreased datavolumes in parent virtualmachine have not been found",
			given: input{
				key: "default/test",
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
					Spec: kubevirtapis.VirtualMachineInstanceSpec{
						Domain: kubevirtapis.DomainSpec{
							Devices: kubevirtapis.Devices{
								Disks: []kubevirtapis.Disk{
									{
										Name:      "disk1",
										BootOrder: pointerToUint(1),
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
									{
										Name: "disk2",
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
								},
							},
						},
						Volumes: []kubevirtapis.Volume{
							{
								Name: "disk1",
								VolumeSource: kubevirtapis.VolumeSource{
									ContainerDisk: &kubevirtapis.ContainerDiskSource{
										Image: "vmidisks/fedora25:latest",
									},
								},
							},
							{
								Name: "disk2",
								VolumeSource: kubevirtapis.VolumeSource{
									DataVolume: &kubevirtapis.DataVolumeSource{
										Name: "dv-disk",
									},
								},
							},
						},
					},
				},
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
					},
					Spec: kubevirtapis.VirtualMachineSpec{
						Template: &kubevirtapis.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtapis.VirtualMachineInstanceSpec{
								Domain: kubevirtapis.DomainSpec{
									Devices: kubevirtapis.Devices{
										Disks: []kubevirtapis.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtapis.DiskDevice{
													Disk: &kubevirtapis.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtapis.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtapis.VolumeSource{
											ContainerDisk: &kubevirtapis.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
								},
							},
						},
					},
				},
				dv: nil,
			},
			expected: output{
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
					Spec: kubevirtapis.VirtualMachineInstanceSpec{
						Domain: kubevirtapis.DomainSpec{
							Devices: kubevirtapis.Devices{
								Disks: []kubevirtapis.Disk{
									{
										Name:      "disk1",
										BootOrder: pointerToUint(1),
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
									{
										Name: "disk2",
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
								},
							},
						},
						Volumes: []kubevirtapis.Volume{
							{
								Name: "disk1",
								VolumeSource: kubevirtapis.VolumeSource{
									ContainerDisk: &kubevirtapis.ContainerDiskSource{
										Image: "vmidisks/fedora25:latest",
									},
								},
							},
							{
								Name: "disk2",
								VolumeSource: kubevirtapis.VolumeSource{
									DataVolume: &kubevirtapis.DataVolumeSource{
										Name: "dv-disk",
									},
								},
							},
						},
					},
				},
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "unref ownless out-tree datavolumes",
			given: input{
				key: "default/test",
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
					Spec: kubevirtapis.VirtualMachineInstanceSpec{
						Domain: kubevirtapis.DomainSpec{
							Devices: kubevirtapis.Devices{
								Disks: []kubevirtapis.Disk{
									{
										Name:      "disk1",
										BootOrder: pointerToUint(1),
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
									{
										Name: "disk2",
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
								},
							},
						},
						Volumes: []kubevirtapis.Volume{
							{
								Name: "disk1",
								VolumeSource: kubevirtapis.VolumeSource{
									ContainerDisk: &kubevirtapis.ContainerDiskSource{
										Image: "vmidisks/fedora25:latest",
									},
								},
							},
							{
								Name: "disk2",
								VolumeSource: kubevirtapis.VolumeSource{
									DataVolume: &kubevirtapis.DataVolumeSource{
										Name: "dv-disk",
									},
								},
							},
						},
					},
				},
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
					},
					Spec: kubevirtapis.VirtualMachineSpec{
						Template: &kubevirtapis.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtapis.VirtualMachineInstanceSpec{
								Domain: kubevirtapis.DomainSpec{
									Devices: kubevirtapis.Devices{
										Disks: []kubevirtapis.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtapis.DiskDevice{
													Disk: &kubevirtapis.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtapis.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtapis.VolumeSource{
											ContainerDisk: &kubevirtapis.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
								},
							},
						},
					},
				},
				dv: &cdiapis.DataVolume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "dv-disk",
						UID:       "fake-dv-uid",
						Annotations: map[string]string{
							ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/test"]}]`,
						},
					},
					Spec: cdiapis.DataVolumeSpec{
						Source: cdiapis.DataVolumeSource{
							Blank: &cdiapis.DataVolumeBlankImage{},
						},
						PVC: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: pointer.StringPtr("default"),
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteOnce,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("2Gi"),
								},
							},
						},
					},
				},
			},
			expected: output{
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
					Spec: kubevirtapis.VirtualMachineInstanceSpec{
						Domain: kubevirtapis.DomainSpec{
							Devices: kubevirtapis.Devices{
								Disks: []kubevirtapis.Disk{
									{
										Name:      "disk1",
										BootOrder: pointerToUint(1),
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
									{
										Name: "disk2",
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
								},
							},
						},
						Volumes: []kubevirtapis.Volume{
							{
								Name: "disk1",
								VolumeSource: kubevirtapis.VolumeSource{
									ContainerDisk: &kubevirtapis.ContainerDiskSource{
										Image: "vmidisks/fedora25:latest",
									},
								},
							},
							{
								Name: "disk2",
								VolumeSource: kubevirtapis.VolumeSource{
									DataVolume: &kubevirtapis.DataVolumeSource{
										Name: "dv-disk",
									},
								},
							},
						},
					},
				},
				err: nil,
				dv: &cdiapis.DataVolume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "dv-disk",
						UID:       "fake-dv-uid",
					},
					Spec: cdiapis.DataVolumeSpec{
						Source: cdiapis.DataVolumeSource{
							Blank: &cdiapis.DataVolumeBlankImage{},
						},
						PVC: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: pointer.StringPtr("default"),
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteOnce,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("2Gi"),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "unref ownless generated datavolumes",
			given: input{
				key: "default/test",
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
					Spec: kubevirtapis.VirtualMachineInstanceSpec{
						Domain: kubevirtapis.DomainSpec{
							Devices: kubevirtapis.Devices{
								Disks: []kubevirtapis.Disk{
									{
										Name:      "disk1",
										BootOrder: pointerToUint(1),
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
									{
										Name: "disk2",
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
								},
							},
						},
						Volumes: []kubevirtapis.Volume{
							{
								Name: "disk1",
								VolumeSource: kubevirtapis.VolumeSource{
									ContainerDisk: &kubevirtapis.ContainerDiskSource{
										Image: "vmidisks/fedora25:latest",
									},
								},
							},
							{
								Name: "disk2",
								VolumeSource: kubevirtapis.VolumeSource{
									DataVolume: &kubevirtapis.DataVolumeSource{
										Name: "dv-disk",
									},
								},
							},
						},
					},
				},
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
					},
					Spec: kubevirtapis.VirtualMachineSpec{
						Template: &kubevirtapis.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtapis.VirtualMachineInstanceSpec{
								Domain: kubevirtapis.DomainSpec{
									Devices: kubevirtapis.Devices{
										Disks: []kubevirtapis.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtapis.DiskDevice{
													Disk: &kubevirtapis.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtapis.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtapis.VolumeSource{
											ContainerDisk: &kubevirtapis.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
								},
							},
						},
					},
				},
				dv: &cdiapis.DataVolume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "dv-disk",
						UID:       "fake-dv-uid",
						Annotations: map[string]string{
							ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/test"]}]`,
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
					Spec: cdiapis.DataVolumeSpec{
						Source: cdiapis.DataVolumeSource{
							Blank: &cdiapis.DataVolumeBlankImage{},
						},
						PVC: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: pointer.StringPtr("default"),
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteOnce,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("2Gi"),
								},
							},
						},
					},
				},
			},
			expected: output{
				vmi: &kubevirtapis.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
					Spec: kubevirtapis.VirtualMachineInstanceSpec{
						Domain: kubevirtapis.DomainSpec{
							Devices: kubevirtapis.Devices{
								Disks: []kubevirtapis.Disk{
									{
										Name:      "disk1",
										BootOrder: pointerToUint(1),
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
									{
										Name: "disk2",
										DiskDevice: kubevirtapis.DiskDevice{
											Disk: &kubevirtapis.DiskTarget{
												Bus: "virtio",
											},
										},
									},
								},
							},
						},
						Volumes: []kubevirtapis.Volume{
							{
								Name: "disk1",
								VolumeSource: kubevirtapis.VolumeSource{
									ContainerDisk: &kubevirtapis.ContainerDiskSource{
										Image: "vmidisks/fedora25:latest",
									},
								},
							},
							{
								Name: "disk2",
								VolumeSource: kubevirtapis.VolumeSource{
									DataVolume: &kubevirtapis.DataVolumeSource{
										Name: "dv-disk",
									},
								},
							},
						},
					},
				},
				err: nil,
				dv: &cdiapis.DataVolume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "dv-disk",
						UID:       "fake-dv-uid",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1",
								Kind:               "VirtualMachine",
								Name:               "test",
								UID:                "fake-vm-uid",
								BlockOwnerDeletion: pointer.BoolPtr(true),
								Controller:         pointer.BoolPtr(true),
							},
						},
					},
					Spec: cdiapis.DataVolumeSpec{
						Source: cdiapis.DataVolumeSource{
							Blank: &cdiapis.DataVolumeBlankImage{},
						},
						PVC: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: pointer.StringPtr("default"),
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteOnce,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("2Gi"),
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		if tc.given.vmi != nil {
			var err = clientset.Tracker().Add(tc.given.vmi)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		if tc.given.dv != nil {
			var err = clientset.Tracker().Add(tc.given.dv)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		if tc.given.vm != nil {
			var err = clientset.Tracker().Add(tc.given.vm)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		var ctrl = &VMIController{
			virtualMachineCache: fakeVirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
			dataVolumeClient:    fakeDataVolumeClient(clientset.CdiV1beta1().DataVolumes),
			dataVolumeCache:     fakeDataVolumeCache(clientset.CdiV1beta1().DataVolumes),
		}
		if tc.given.vmi != nil {
			var hasFinalizer = sets.NewString(tc.given.vmi.Finalizers...).Has("wrangler.cattle.io/VMIController.UnsetOwnerOfDataVolumes")
			assert.True(t, hasFinalizer, "case %q's input is not a process target", tc.name)
		}
		var actual output
		actual.vmi, actual.err = ctrl.UnsetOwnerOfDataVolumes(tc.given.key, tc.given.vmi)
		if tc.expected.dv != nil {
			var dvStored, err = clientset.Tracker().Get(cdiapis.SchemeGroupVersion.WithResource("datavolumes"), tc.expected.dv.Namespace, tc.expected.dv.Name)
			assert.Nil(t, err, "mock resource should get from fake controller tracker")
			actual.dv = dvStored.(*cdiapis.DataVolume)
		}

		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

type fakeVirtualMachineCache func(string) kubevirttype.VirtualMachineInterface

func (c fakeVirtualMachineCache) Get(namespace, name string) (*kubevirtapis.VirtualMachine, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeVirtualMachineCache) List(namespace string, selector labels.Selector) ([]*kubevirtapis.VirtualMachine, error) {
	panic("implement me")
}

func (c fakeVirtualMachineCache) AddIndexer(indexName string, indexer kubevirtctrl.VirtualMachineIndexer) {
	panic("implement me")
}

func (c fakeVirtualMachineCache) GetByIndex(indexName, key string) ([]*kubevirtapis.VirtualMachine, error) {
	panic("implement me")
}
