package virtualmachine

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	fake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"
	kubevirtv1 "kubevirt.io/api/core/v1"

	fakegenerated "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestVMController_SetOwnerOfPVCs(t *testing.T) {
	type input struct {
		key  string
		vm   *kubevirtv1.VirtualMachine
		pvcs []*corev1.PersistentVolumeClaim
	}
	type output struct {
		vm   *kubevirtv1.VirtualMachine
		err  error
		pvcs []*corev1.PersistentVolumeClaim
	}

	var testFinalizers = []string{harvesterUnsetOwnerOfPVCsFinalizer}
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						DeletionTimestamp: &metav1.Time{},
						Finalizers:        testFinalizers,
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{},
					},
				},
				pvcs: nil,
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{},
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: nil,
					},
				},
				pvcs: nil,
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
					},
					Spec: kubevirtv1.VirtualMachineSpec{
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Finalizers: testFinalizers,
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											EmptyDisk: &kubevirtv1.EmptyDiskSource{
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Finalizers: testFinalizers,
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											EmptyDisk: &kubevirtv1.EmptyDiskSource{
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Finalizers: testFinalizers,
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
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{

											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Finalizers: testFinalizers,
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
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Finalizers: testFinalizers,
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Finalizers: testFinalizers,
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Finalizers: testFinalizers,
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Finalizers: testFinalizers,
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Finalizers: testFinalizers,
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Finalizers: testFinalizers,
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{

											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "attached-pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vm-uid",
						Finalizers: testFinalizers,
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "attached-pvc-disk",
												},
												Hotpluggable: true,
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
		var pvcobjs, harvobjs []runtime.Object
		for _, v := range tc.given.pvcs {
			if tc.given.pvcs != nil {
				pvcobjs = append(pvcobjs, v)
			}
		}

		clientset := fake.NewSimpleClientset(pvcobjs...)

		if tc.given.vm != nil {
			harvobjs = append(harvobjs, tc.given.vm)
		}

		harvFakeClient := fakegenerated.NewSimpleClientset(harvobjs...)

		var ctrl = &VMController{
			pvcClient:      fakeclients.PersistentVolumeClaimClient(clientset.CoreV1().PersistentVolumeClaims),
			pvcCache:       fakeclients.PersistentVolumeClaimCache(clientset.CoreV1().PersistentVolumeClaims),
			vmClient:       fakeclients.VirtualMachineClient(harvFakeClient.KubevirtV1().VirtualMachines),
			vmBackupCache:  fakeclients.VMBackupCache(harvFakeClient.HarvesterhciV1beta1().VirtualMachineBackups),
			vmBackupClient: fakeclients.VMBackupClient(harvFakeClient.HarvesterhciV1beta1().VirtualMachineBackups),
		}

		var actual output
		actual.vm, actual.err = ctrl.ManageOwnerOfPVCs(tc.given.key, tc.given.vm)
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
		vm  *kubevirtv1.VirtualMachine
		pvc *corev1.PersistentVolumeClaim
	}
	type output struct {
		vm  *kubevirtv1.VirtualMachine
		err error
		pvc *corev1.PersistentVolumeClaim
	}
	var testFinalizers = []string{harvesterUnsetOwnerOfPVCsFinalizer}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "ignore if not any PVCs",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											EmptyDisk: &kubevirtv1.EmptyDiskSource{
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											EmptyDisk: &kubevirtv1.EmptyDiskSource{
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
				vm: &kubevirtv1.VirtualMachine{
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
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
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
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vm-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Domain: kubevirtv1.DomainSpec{
									Devices: kubevirtv1.Devices{
										Disks: []kubevirtv1.Disk{
											{
												Name:      "disk1",
												BootOrder: pointerToUint(1),
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
											{
												Name: "disk2",
												DiskDevice: kubevirtv1.DiskDevice{
													Disk: &kubevirtv1.DiskTarget{
														Bus: "virtio",
													},
												},
											},
										},
									},
								},
								Volumes: []kubevirtv1.Volume{
									{
										Name: "disk1",
										VolumeSource: kubevirtv1.VolumeSource{
											ContainerDisk: &kubevirtv1.ContainerDiskSource{
												Image: "vmidisks/fedora25:latest",
											},
										},
									},
									{
										Name: "disk2",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-disk",
												},
												Hotpluggable: true,
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
			var hasFinalizer = sets.NewString(tc.given.vm.Finalizers...).Has(harvesterUnsetOwnerOfPVCsFinalizer)
			assert.True(t, hasFinalizer, "case %q's input is not a process target", tc.name)
		}
		var actual output
		actual.vm = tc.given.vm
		actual.err = ctrl.unsetOwnerOfPVCs(tc.given.vm)
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
