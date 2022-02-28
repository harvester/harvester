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
	corefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	kubevirttype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
	kubevirtctrl "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestVMIController_UnsetOwnerOfPVCs(t *testing.T) {
	type input struct {
		key string
		vmi *kubevirtv1.VirtualMachineInstance
		vm  *kubevirtv1.VirtualMachine
		pvc *corev1.PersistentVolumeClaim
	}
	type output struct {
		vmi *kubevirtv1.VirtualMachineInstance
		err error
		pvc *corev1.PersistentVolumeClaim
	}
	var testFinalizers = []string{"wrangler.cattle.io/VMIController.UnsetOwnerOfPVCs"}

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
				pvc: nil,
			},
			expected: output{
				vmi: nil,
				err: nil,
				pvc: nil,
			},
		},
		{
			name: "ignore none deleted resource",
			given: input{
				key: "default/test",
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vmi-uid",
						Finalizers: testFinalizers,
					},
				},
				vm:  nil,
				pvc: nil,
			},
			expected: output{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "default",
						Name:       "test",
						UID:        "fake-vmi-uid",
						Finalizers: testFinalizers,
					},
				},
				err: nil,
				pvc: nil,
			},
		},
		{
			name: "ignore if isn't owned by any resources",
			given: input{
				key: "default/test",
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
					},
				},
				vm:  nil,
				pvc: nil,
			},
			expected: output{
				vmi: &kubevirtv1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						UID:               "fake-vmi-uid",
						Finalizers:        testFinalizers,
						DeletionTimestamp: &metav1.Time{},
					},
				},
				err: nil,
				pvc: nil,
			},
		},
		{
			name: "ignore if owned by none virtualmachine resource",
			given: input{
				key: "default/test",
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				vm:  nil,
				pvc: nil,
			},
			expected: output{
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				pvc: nil,
			},
		},
		{
			name: "ignore if parent virtualmachine is not found",
			given: input{
				key: "default/test",
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				vm:  nil,
				pvc: nil,
			},
			expected: output{
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				pvc: nil,
			},
		},
		{
			name: "ignore if parent virtualmachine has been deleted",
			given: input{
				key: "default/test",
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				pvc: nil,
			},
			expected: output{
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				pvc: nil,
			},
		},
		{
			name: "ignore if parent virtualmachine's template is nil",
			given: input{
				key: "default/test",
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				pvc: nil,
			},
			expected: output{
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				pvc: nil,
			},
		},
		{
			name: "ignore if both virtualmachine instance and parent virtualmachine do not have any PVCs",
			given: input{
				key: "default/test",
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				err: nil,
				pvc: nil,
			},
		},
		{
			name: "ignore if only add PVCs in parent virtualmachine",
			given: input{
				key: "default/test",
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				vm: &kubevirtv1.VirtualMachine{
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
									{
										Name: "disk3",
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
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				err: nil,
				pvc: nil,
			},
		},
		{
			name: "ignore if the removed PVCs in parent virtualmachine is not found",
			given: input{
				key: "default/test",
				vmi: &kubevirtv1.VirtualMachineInstance{
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
								},
							},
						},
					},
				},
				pvc: nil,
			},
			expected: output{
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				err: nil,
				pvc: nil,
			},
		},
		{
			name: "unref ownless out-tree PVCs",
			given: input{
				key: "default/test",
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				vmi: &kubevirtv1.VirtualMachineInstance{
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
			name: "unref ownless generated PVCs",
			given: input{
				key: "default/test",
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				vmi: &kubevirtv1.VirtualMachineInstance{
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
				err: nil,
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "pvc-disk",
						UID:       "fake-pvc-uid",
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
	}

	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		var coreclientset = corefake.NewSimpleClientset()
		if tc.given.vmi != nil {
			var err = clientset.Tracker().Add(tc.given.vmi)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		if tc.given.vm != nil {
			var err = clientset.Tracker().Add(tc.given.vm)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		if tc.given.pvc != nil {
			var err = coreclientset.Tracker().Add(tc.given.pvc)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		var ctrl = &VMIController{
			virtualMachineCache: fakeVirtualMachineCache(clientset.KubevirtV1().VirtualMachines),
			pvcClient:           fakeclients.PersistentVolumeClaimClient(coreclientset.CoreV1().PersistentVolumeClaims),
			pvcCache:            fakeclients.PersistentVolumeClaimCache(coreclientset.CoreV1().PersistentVolumeClaims),
		}
		if tc.given.vmi != nil {
			var hasFinalizer = sets.NewString(tc.given.vmi.Finalizers...).Has("wrangler.cattle.io/VMIController.UnsetOwnerOfPVCs")
			assert.True(t, hasFinalizer, "case %q's input is not a process target", tc.name)
		}
		var actual output
		actual.vmi, actual.err = ctrl.UnsetOwnerOfPVCs(tc.given.key, tc.given.vmi)
		if tc.expected.pvc != nil {
			var pvcStored, err = coreclientset.Tracker().Get(corev1.SchemeGroupVersion.WithResource("persistentvolumeclaims"), tc.expected.pvc.Namespace, tc.expected.pvc.Name)
			assert.Nil(t, err, "mock resource should get from fake controller tracker")
			actual.pvc = pvcStored.(*corev1.PersistentVolumeClaim)
		}

		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

type fakeVirtualMachineCache func(string) kubevirttype.VirtualMachineInterface

func (c fakeVirtualMachineCache) Get(namespace, name string) (*kubevirtv1.VirtualMachine, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c fakeVirtualMachineCache) List(namespace string, selector labels.Selector) ([]*kubevirtv1.VirtualMachine, error) {
	panic("implement me")
}

func (c fakeVirtualMachineCache) AddIndexer(indexName string, indexer kubevirtctrl.VirtualMachineIndexer) {
	panic("implement me")
}

func (c fakeVirtualMachineCache) GetByIndex(indexName, key string) ([]*kubevirtv1.VirtualMachine, error) {
	panic("implement me")
}
