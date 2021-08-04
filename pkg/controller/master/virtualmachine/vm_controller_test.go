package virtualmachine

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	fake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"
	kubevirtapis "kubevirt.io/client-go/api/v1"

	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestVMController_SetOwnerOfPVCs(t *testing.T) {
	type input struct {
		key  string
		vm   *kubevirtapis.VirtualMachine
		pvcs []*corev1.PersistentVolumeClaim
	}
	type output struct {
		vm   *kubevirtapis.VirtualMachine
		err  error
		pvcs []*corev1.PersistentVolumeClaim
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "ignore nil resource",
			given: input{
				key:  "",
				vm:   nil,
				pvcs: nil,
			},
			expected: output{
				vm:   nil,
				err:  nil,
				pvcs: nil,
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
				pvcs: nil,
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
				err:  nil,
				pvcs: nil,
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
				pvcs: nil,
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
				err:  nil,
				pvcs: nil,
			},
		},
		{
			name: "ignore if not any PVCs",
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
				pvcs: nil,
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
				err:  nil,
				pvcs: nil,
			},
		},
		{
			name: "ref in-tree PVCs",
			given: input{
				key: "default/test",
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: MustPVCTemplatesToString([]corev1.PersistentVolumeClaim{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "pvc-disk",
									},
									Spec: corev1.PersistentVolumeClaimSpec{
										StorageClassName: pointer.StringPtr("default"),
										AccessModes: []corev1.PersistentVolumeAccessMode{
											corev1.ReadWriteOnce,
										},
										Resources: corev1.ResourceRequirements{
											Requests: corev1.ResourceList{
												corev1.ResourceStorage: resource.MustParse("2Gi"),
											},
										},
									}},
							}),
						},
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "pvc-disk",
							UID:       "fake-pvc-uid",
						},
						Spec: corev1.PersistentVolumeClaimSpec{
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
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: MustPVCTemplatesToString([]corev1.PersistentVolumeClaim{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "pvc-disk",
									},
									Spec: corev1.PersistentVolumeClaimSpec{
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
							}),
						},
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				err: nil,
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "pvc-disk",
							UID:       "fake-pvc-uid",
							Annotations: map[string]string{
								ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/test"]}]`,
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
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
			name: "ignore if out-tree PVC does not exist",
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				pvcs: nil,
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				err:  nil,
				pvcs: nil,
			},
		},
		{
			name: "ref out-tree PVCs",
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "pvc-disk",
							UID:       "fake-pvc-uid",
						},
						Spec: corev1.PersistentVolumeClaimSpec{
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				err: nil,
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "pvc-disk",
							UID:       "fake-pvc-uid",
							Annotations: map[string]string{
								ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/test"]}]`,
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
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
			name: "PVCs can be referred several times",
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "pvc-disk",
							UID:       "fake-pvc-uid",
							Annotations: map[string]string{
								ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachineinstance","refs":["default/test"]}]`,
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				err: nil,
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "pvc-disk",
							UID:       "fake-pvc-uid",
							Annotations: map[string]string{
								ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/test"]},{"schema":"kubevirt.io.virtualmachineinstance","refs":["default/test"]}]`,
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
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
		{
			name: "clean ownerReference if PVC unattached",
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "attached-pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "attached-pvc-disk",
							UID:       "fake-attached-pvc-uid",
						},
						Spec: corev1.PersistentVolumeClaimSpec{
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
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "unattached-pvc-disk",
							UID:       "fake-unattached-pvc-uid",
							Annotations: map[string]string{
								ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/test"]}]`,
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "attached-pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				err: nil,
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "attached-pvc-disk",
							UID:       "fake-attached-pvc-uid",
							Annotations: map[string]string{
								ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/test"]}]`,
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
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
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "unattached-pvc-disk",
							UID:       "fake-unattached-pvc-uid",
						},
						Spec: corev1.PersistentVolumeClaimSpec{
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
		if tc.given.pvcs != nil {
			for _, dv := range tc.given.pvcs {
				var err = clientset.Tracker().Add(dv)
				assert.Nil(t, err, "mock resource should add into fake controller tracker")
			}
		}

		var ctrl = &VMController{
			pvcClient: fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims),
			pvcCache:  fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
		}
		var actual output
		actual.vm, actual.err = ctrl.SetOwnerOfPVCs(tc.given.key, tc.given.vm)
		if tc.expected.pvcs != nil {
			for _, pvc := range tc.expected.pvcs {
				var pvcStored, err = clientset.Tracker().Get(corev1.SchemeGroupVersion.WithResource("persistentvolumeclaims"), pvc.Namespace, pvc.Name)
				assert.Nil(t, err, "mock resource should get from fake controller tracker")
				actual.pvcs = append(actual.pvcs, pvcStored.(*corev1.PersistentVolumeClaim))
			}
		}

		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

func TestVMController_UnsetOwnerOfPVCs(t *testing.T) {
	type input struct {
		key string
		vm  *kubevirtapis.VirtualMachine
		pvc *corev1.PersistentVolumeClaim
	}
	type output struct {
		vm  *kubevirtapis.VirtualMachine
		err error
		pvc *corev1.PersistentVolumeClaim
	}
	var testFinalizers = []string{"wrangler.cattle.io/VMController.UnsetOwnerOfPVCs"}

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
				pvc: nil,
			},
			expected: output{
				vm:  nil,
				err: nil,
				pvc: nil,
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
				pvc: nil,
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
				pvc: nil,
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
				pvc: nil,
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
				pvc: nil,
			},
		},
		{
			name: "ignore if not any PVCs",
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
				pvc: nil,
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
				pvc: nil,
			},
		},
		{
			name: "unref in-tree PVCs",
			given: input{
				key: "default/test",
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: MustPVCTemplatesToString([]corev1.PersistentVolumeClaim{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "pvc-disk",
									},
									Spec: corev1.PersistentVolumeClaimSpec{
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
							}),
						},
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "pvc-disk",
						UID:       "fake-pvc-uid",
						Annotations: map[string]string{
							ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/test"]}]`,
						},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
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
			expected: output{
				vm: &kubevirtapis.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: MustPVCTemplatesToString([]corev1.PersistentVolumeClaim{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "pvc-disk",
									},
									Spec: corev1.PersistentVolumeClaimSpec{
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
							}),
						},
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				err: nil,
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "pvc-disk",
						UID:       "fake-pvc-uid",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
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
		{
			name: "ignore if out-tree PVC does not exist",
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				pvc: nil,
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				err: nil,
				pvc: nil,
			},
		},
		{
			name: "unref out-tree PVCs",
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "pvc-disk",
						UID:       "fake-pvc-uid",
						Annotations: map[string]string{
							ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/test"]}]`,
						},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				err: nil,
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "pvc-disk",
						UID:       "fake-pvc-uid",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
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
		{
			name: "PVCs can be unreferred several times",
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "pvc-disk",
						UID:       "fake-pvc-uid",
						Annotations: map[string]string{
							ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachine","refs":["default/test"]},{"schema":"kubevirt.io.virtualmachineinstance","refs":["default/test"]}]`,
						},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
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
											PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
												ClaimName: "pvc-disk",
											},
										},
									},
								},
							},
						},
					},
				},
				err: nil,
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "pvc-disk",
						UID:       "fake-pvc-uid",
						Annotations: map[string]string{
							ref.AnnotationSchemaOwnerKeyName: `[{"schema":"kubevirt.io.virtualmachineinstance","refs":["default/test"]}]`,
						},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
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
	}

	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		if tc.given.pvc != nil {
			var err = clientset.Tracker().Add(tc.given.pvc)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		var ctrl = &VMController{
			pvcClient: fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims),
			pvcCache:  fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
		}
		if tc.given.vm != nil {
			var hasFinalizer = sets.NewString(tc.given.vm.Finalizers...).Has("wrangler.cattle.io/VMController.UnsetOwnerOfPVCs")
			assert.True(t, hasFinalizer, "case %q's input is not a process target", tc.name)
		}
		var actual output
		actual.vm, actual.err = ctrl.UnsetOwnerOfPVCs(tc.given.key, tc.given.vm)
		if tc.expected.pvc != nil {
			var pvcStored, err = clientset.Tracker().Get(corev1.SchemeGroupVersion.WithResource("persistentvolumeclaims"), tc.expected.pvc.Namespace, tc.expected.pvc.Name)
			assert.Nil(t, err, "mock resource should get from fake controller tracker")
			actual.pvc = pvcStored.(*corev1.PersistentVolumeClaim)
		}

		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

func pointerToUint(i uint) *uint {
	return &i
}

func PVCTemplatesToString(pvcs []corev1.PersistentVolumeClaim) (string, error) {
	b, err := json.Marshal(pvcs)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func MustPVCTemplatesToString(pvcs []corev1.PersistentVolumeClaim) string {
	result, err := PVCTemplatesToString(pvcs)
	if err != nil {
		panic(err)
	}
	return result
}
