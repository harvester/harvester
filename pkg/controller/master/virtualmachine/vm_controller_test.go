package virtualmachine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/utils/pointer"
	kubevirtapis "kubevirt.io/client-go/api/v1alpha3"
	cdiapisalpha "kubevirt.io/containerized-data-importer/pkg/apis/core/v1alpha1"
	cdiapis "kubevirt.io/containerized-data-importer/pkg/apis/core/v1beta1"

	"github.com/rancher/harvester/pkg/generated/clientset/versioned/fake"
	cditype "github.com/rancher/harvester/pkg/generated/clientset/versioned/typed/cdi.kubevirt.io/v1beta1"
	cdictrl "github.com/rancher/harvester/pkg/generated/controllers/cdi.kubevirt.io/v1beta1"
	"github.com/rancher/harvester/pkg/indexeres"
	"github.com/rancher/harvester/pkg/ref"
)

func TestVMController_SetOwnerOfDataVolumes(t *testing.T) {
	type input struct {
		key string
		vm  *kubevirtapis.VirtualMachine
		dvs []*cdiapis.DataVolume
	}
	type output struct {
		vm  *kubevirtapis.VirtualMachine
		err error
		dvs []*cdiapis.DataVolume
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "ignore nil resource",
			given: input{
				key: "",
				vm:  nil,
				dvs: nil,
			},
			expected: output{
				vm:  nil,
				err: nil,
				dvs: nil,
			},
		},
		{
			name: "ignore deleted resource",
			given: input{
				key: "default/test",
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
				dvs: nil,
			},
			expected: output{
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
				err: nil,
				dvs: nil,
			},
		},
		{
			name: "ignore nil virtual machine instance template",
			given: input{
				key: "default/test",
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
				dvs: nil,
			},
			expected: output{
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
				err: nil,
				dvs: nil,
			},
		},
		{
			name: "ignore if not any datavolumes",
			given: input{
				key: "default/test",
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
				dvs: nil,
			},
			expected: output{
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
				err: nil,
				dvs: nil,
			},
		},
		{
			name: "ref in-tree datavolumes",
			given: input{
				key: "default/test",
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
				dvs: []*cdiapis.DataVolume{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "dv-disk",
							UID:       "fake-dv-uid",
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion:         "kubevirt.io/v1alpha3",
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
			expected: output{
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
				err: nil,
				dvs: []*cdiapis.DataVolume{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "dv-disk",
							UID:       "fake-dv-uid",
							Annotations: map[string]string{
								ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/test"]}]`,
							},
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion:         "kubevirt.io/v1alpha3",
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
		},
		{
			name: "ignore if out-tree datavolume is not existed",
			given: input{
				key: "default/test",
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
				dvs: nil,
			},
			expected: output{
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
				err: nil,
				dvs: nil,
			},
		},
		{
			name: "ref out-tree datavolumes",
			given: input{
				key: "default/test",
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
				dvs: []*cdiapis.DataVolume{
					{
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
			expected: output{
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
				err: nil,
				dvs: []*cdiapis.DataVolume{
					{
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
			},
		},
		{
			name: "datavolumes can be referred several times",
			given: input{
				key: "default/test",
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
				dvs: []*cdiapis.DataVolume{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "dv-disk",
							UID:       "fake-dv-uid",
							Annotations: map[string]string{
								ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachineinstance","refs":["default/test"]}]`,
							},
						},
						Spec: cdiapis.DataVolumeSpec{
							Source: cdiapis.DataVolumeSource{
								Blank: &cdiapis.DataVolumeBlankImage{},
							},
							PVC: &corev1.PersistentVolumeClaimSpec{
								StorageClassName: pointer.StringPtr("default"),
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteMany,
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
			expected: output{
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
				err: nil,
				dvs: []*cdiapis.DataVolume{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "dv-disk",
							UID:       "fake-dv-uid",
							Annotations: map[string]string{
								ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/test"]},{"schema":"kubevirt.io.virtualmachineinstance","refs":["default/test"]}]`,
							},
						},
						Spec: cdiapis.DataVolumeSpec{
							Source: cdiapis.DataVolumeSource{
								Blank: &cdiapis.DataVolumeBlankImage{},
							},
							PVC: &corev1.PersistentVolumeClaimSpec{
								StorageClassName: pointer.StringPtr("default"),
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteMany,
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
		},
		{
			name: "clean ownerReference if datavolume unattached",
			given: input{
				key: "default/test",
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
											DataVolume: &kubevirtapis.DataVolumeSource{
												Name: "attached-dv-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				dvs: []*cdiapis.DataVolume{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "attached-dv-disk",
							UID:       "fake-attached-dv-uid",
						},
						Spec: cdiapis.DataVolumeSpec{
							Source: cdiapis.DataVolumeSource{
								Blank: &cdiapis.DataVolumeBlankImage{},
							},
							PVC: &corev1.PersistentVolumeClaimSpec{
								StorageClassName: pointer.StringPtr("default"),
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteMany,
								},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "unattached-dv-disk",
							UID:       "fake-unattached-dv-uid",
							Annotations: map[string]string{
								ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/test"]}]`,
							},
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion:         "kubevirt.io/v1alpha3",
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
									corev1.ReadWriteMany,
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
			expected: output{
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
											DataVolume: &kubevirtapis.DataVolumeSource{
												Name: "attached-dv-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				err: nil,
				dvs: []*cdiapis.DataVolume{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "attached-dv-disk",
							UID:       "fake-attached-dv-uid",
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
									corev1.ReadWriteMany,
								},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse("2Gi"),
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "unattached-dv-disk",
							UID:       "fake-unattached-dv-uid",
						},
						Spec: cdiapis.DataVolumeSpec{
							Source: cdiapis.DataVolumeSource{
								Blank: &cdiapis.DataVolumeBlankImage{},
							},
							PVC: &corev1.PersistentVolumeClaimSpec{
								StorageClassName: pointer.StringPtr("default"),
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteMany,
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
		},
	}

	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		if tc.given.vm != nil {
			var err = clientset.Tracker().Add(tc.given.vm)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		if tc.given.dvs != nil {
			for _, dv := range tc.given.dvs {
				var err = clientset.Tracker().Add(dv)
				assert.Nil(t, err, "mock resource should add into fake controller tracker")
			}
		}

		var ctrl = &VMController{
			dataVolumeClient: fakeDataVolumeClient(clientset.CdiV1beta1().DataVolumes),
			dataVolumeCache:  fakeDataVolumeCache(clientset.CdiV1beta1().DataVolumes),
		}
		var actual output
		actual.vm, actual.err = ctrl.SetOwnerOfDataVolumes(tc.given.key, tc.given.vm)
		if tc.expected.dvs != nil {
			for _, dv := range tc.expected.dvs {
				var dvStored, err = clientset.Tracker().Get(cdiapis.SchemeGroupVersion.WithResource("datavolumes"), dv.Namespace, dv.Name)
				assert.Nil(t, err, "mock resource should get from fake controller tracker")
				actual.dvs = append(actual.dvs, dvStored.(*cdiapis.DataVolume))
			}
		}

		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

func TestVMController_UnsetOwnerOfDataVolumes(t *testing.T) {
	type input struct {
		key string
		vm  *kubevirtapis.VirtualMachine
		dv  *cdiapis.DataVolume
	}
	type output struct {
		vm  *kubevirtapis.VirtualMachine
		err error
		dv  *cdiapis.DataVolume
	}
	var testFinalizers = []string{"wrangler.cattle.io/VMController.UnsetOwnerOfDataVolumes"}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "ignore nil resource",
			given: input{
				key: "",
				vm:  nil,
				dv:  nil,
			},
			expected: output{
				vm:  nil,
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "ignore none deleted resource",
			given: input{
				key: "default/test",
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Finalizers: testFinalizers,
					},
					Spec: kubevirtapis.VirtualMachineSpec{
						Template: &kubevirtapis.VirtualMachineInstanceTemplateSpec{},
					},
				},
				dv: nil,
			},
			expected: output{
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Finalizers: testFinalizers,
					},
					Spec: kubevirtapis.VirtualMachineSpec{
						Template: &kubevirtapis.VirtualMachineInstanceTemplateSpec{},
					},
				},
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "ignore nil virtual machine instance template",
			given: input{
				key: "default/test",
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: kubevirtapis.VirtualMachineSpec{
						Template: nil,
					},
				},
				dv: nil,
			},
			expected: output{
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: kubevirtapis.VirtualMachineSpec{
						Template: nil,
					},
				},
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "ignore if not any datavolumes",
			given: input{
				key: "default/test",
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
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
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
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
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "unref in-tree datavolumes",
			given: input{
				key: "default/test",
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
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
								APIVersion:         "kubevirt.io/v1alpha3",
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
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
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
				err: nil,
				dv: &cdiapis.DataVolume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "dv-disk",
						UID:       "fake-dv-uid",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "kubevirt.io/v1alpha3",
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
		{
			name: "ignore if out-tree datavolume is not existed",
			given: input{
				key: "default/test",
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
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
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
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
				err: nil,
				dv:  nil,
			},
		},
		{
			name: "unref out-tree datavolumes",
			given: input{
				key: "default/test",
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
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
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
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
			name: "datavolumes can be unreferred several times",
			given: input{
				key: "default/test",
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
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
				dv: &cdiapis.DataVolume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "dv-disk",
						UID:       "fake-dv-uid",
						Annotations: map[string]string{
							ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/test"]},{"schema":"kubevirt.io.virtualmachineinstance","refs":["default/test"]}]`,
						},
					},
					Spec: cdiapis.DataVolumeSpec{
						Source: cdiapis.DataVolumeSource{
							Blank: &cdiapis.DataVolumeBlankImage{},
						},
						PVC: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: pointer.StringPtr("default"),
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteMany,
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
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
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
				err: nil,
				dv: &cdiapis.DataVolume{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "dv-disk",
						UID:       "fake-dv-uid",
						Annotations: map[string]string{
							ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachineinstance","refs":["default/test"]}]`,
						},
					},
					Spec: cdiapis.DataVolumeSpec{
						Source: cdiapis.DataVolumeSource{
							Blank: &cdiapis.DataVolumeBlankImage{},
						},
						PVC: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: pointer.StringPtr("default"),
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteMany,
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
		if tc.given.vm != nil {
			var err = clientset.Tracker().Add(tc.given.vm)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		if tc.given.dv != nil {
			var err = clientset.Tracker().Add(tc.given.dv)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		var ctrl = &VMController{
			dataVolumeClient: fakeDataVolumeClient(clientset.CdiV1beta1().DataVolumes),
			dataVolumeCache:  fakeDataVolumeCache(clientset.CdiV1beta1().DataVolumes),
		}
		if tc.given.vm != nil {
			var hasFinalizer = sets.NewString(tc.given.vm.Finalizers...).Has("wrangler.cattle.io/VMController.UnsetOwnerOfDataVolumes")
			assert.True(t, hasFinalizer, "case %q's input is not a process target", tc.name)
		}
		var actual output
		actual.vm, actual.err = ctrl.UnsetOwnerOfDataVolumes(tc.given.key, tc.given.vm)
		if tc.expected.dv != nil {
			var dvStored, err = clientset.Tracker().Get(cdiapis.SchemeGroupVersion.WithResource("datavolumes"), tc.expected.dv.Namespace, tc.expected.dv.Name)
			assert.Nil(t, err, "mock resource should get from fake controller tracker")
			actual.dv = dvStored.(*cdiapis.DataVolume)
		}

		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

func pointerToUint(i uint) *uint {
	return &i
}

type fakeDataVolumeClient func(string) cditype.DataVolumeInterface

func (c fakeDataVolumeClient) Create(volume *cdiapis.DataVolume) (*cdiapis.DataVolume, error) {
	return c(volume.Namespace).Create(context.TODO(), volume, metav1.CreateOptions{})
}

func (c fakeDataVolumeClient) Update(volume *cdiapis.DataVolume) (*cdiapis.DataVolume, error) {
	return c(volume.Namespace).Update(context.TODO(), volume, metav1.UpdateOptions{})
}

func (c fakeDataVolumeClient) UpdateStatus(volume *cdiapis.DataVolume) (*cdiapis.DataVolume, error) {
	panic("implement me")
}

func (c fakeDataVolumeClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	return c(namespace).Delete(context.TODO(), name, *options)
}

func (c fakeDataVolumeClient) Get(namespace, name string, options metav1.GetOptions) (*cdiapis.DataVolume, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c fakeDataVolumeClient) List(namespace string, opts metav1.ListOptions) (*cdiapis.DataVolumeList, error) {
	return c(namespace).List(context.TODO(), opts)
}

func (c fakeDataVolumeClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c(namespace).Watch(context.TODO(), opts)
}

func (c fakeDataVolumeClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *cdiapis.DataVolume, err error) {
	return c(namespace).Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type fakeDataVolumeCache func(string) cditype.DataVolumeInterface

func (c fakeDataVolumeCache) Get(namespace, name string) (*cdiapis.DataVolume, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeDataVolumeCache) List(namespace string, selector labels.Selector) ([]*cdiapis.DataVolume, error) {
	panic("implement me")
}

func (c fakeDataVolumeCache) AddIndexer(indexName string, indexer cdictrl.DataVolumeIndexer) {
	panic("implement me")
}

func (c fakeDataVolumeCache) GetByIndex(indexName, key string) ([]*cdiapis.DataVolume, error) {
	switch indexName {
	case indexeres.DataVolumeByVMIndex:
		vmNamespace, _ := ref.Parse(key)
		dataVolumeList, err := c(vmNamespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		var dataVolumes []*cdiapis.DataVolume
		for _, dataVolume := range dataVolumeList.Items {
			dataVolumes = append(dataVolumes, &dataVolume)
		}
		return dataVolumes, nil
	default:
		return nil, nil
	}
}
