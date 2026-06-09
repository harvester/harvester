package virtualmachine

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	kubevirtv1 "kubevirt.io/api/core/v1"
	backendstorage "kubevirt.io/kubevirt/pkg/storage/backend-storage"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

func TestSetTargetVolumeStrategy(t *testing.T) {
	type input struct {
		key string
		vm  *kubevirtv1.VirtualMachine
	}
	type output struct {
		vm  *kubevirtv1.VirtualMachine
		err error
	}

	migrationStrategy := kubevirtv1.UpdateVolumesStrategyMigration

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "ignore nil VM",
			given: input{
				key: "",
				vm:  nil,
			},
			expected: output{
				vm:  nil,
				err: nil,
			},
		},
		{
			name: "ignore deleted VM",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						DeletionTimestamp: &metav1.Time{},
					},
				},
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         "default",
						Name:              "test",
						DeletionTimestamp: &metav1.Time{},
					},
				},
				err: nil,
			},
		},
		{
			name: "ignore VM without annotation",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{},
					},
				},
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{},
					},
				},
				err: nil,
			},
		},
		{
			name: "ignore VM with no targetVolume entries",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"}}},
							}),
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-1",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"}}},
							}),
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-1",
												},
											},
										},
									},
								},
							},
						},
					},
				},
				err: nil,
			},
		},
		{
			name: "replace PVC volume with new PVC",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume:          "new-pvc",
								},
							}),
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "old-pvc",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume:          "new-pvc",
								},
							}),
							util.AnnotationWaitingStorageMigration: "true",
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "new-pvc",
												},
											},
										},
									},
								},
							},
						},
						UpdateVolumesStrategy: &migrationStrategy,
					},
				},
				err: nil,
			},
		},
		{
			name: "replace hotpluggable PVC volume with target PVC",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume:          "new-pvc",
								},
							}),
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "old-pvc",
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
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume:          "new-pvc",
								},
							}),
							util.AnnotationWaitingStorageMigration: "true",
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "new-pvc",
												},
												Hotpluggable: true,
											},
										},
									},
								},
							},
						},
						UpdateVolumesStrategy: &migrationStrategy,
					},
				},
				err: nil,
			},
		},
		{
			name: "replace only matching volume, leave others unchanged",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-keep"}}},
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-replace"}},
									TargetVolume:          "pvc-new",
								},
							}),
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-keep",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-keep",
												},
											},
										},
									},
									{
										Name: "vol-replace",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-replace",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-keep"}}},
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-replace"}},
									TargetVolume:          "pvc-new",
								},
							}),
							util.AnnotationWaitingStorageMigration: "true",
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-keep",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-keep",
												},
											},
										},
									},
									{
										Name: "vol-replace",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "pvc-new",
												},
											},
										},
									},
								},
							},
						},
						UpdateVolumesStrategy: &migrationStrategy,
					},
				},
				err: nil,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			if tc.given.vm != nil {
				err := clientset.Tracker().Add(tc.given.vm)
				require.NoError(t, err, "mock VM should add into fake tracker")
			}

			ctrl := &VMController{
				vmClient:         newFakeVMClient(clientset),
				dataVolumeClient: newFakeDataVolumeClient(),
			}

			actual, err := ctrl.SetTargetVolumeStrategy(tc.given.key, tc.given.vm)

			if tc.expected.err != nil {
				assert.Error(t, err, "case %q", tc.name)
			} else {
				assert.NoError(t, err, "case %q", tc.name)
			}

			if tc.expected.vm == nil {
				assert.Nil(t, actual, "case %q", tc.name)
				return
			}

			require.NotNil(t, actual, "case %q", tc.name)
			assert.Equal(t, tc.expected.vm.Spec.UpdateVolumesStrategy, actual.Spec.UpdateVolumesStrategy, "UpdateVolumesStrategy")
			if tc.expected.vm.Spec.Template != nil {
				require.NotNil(t, actual.Spec.Template)
				assert.Equal(t, tc.expected.vm.Spec.Template.Spec.Volumes, actual.Spec.Template.Spec.Volumes, "Volumes")
			}
		})
	}
}

func TestCleanupTargetVolumeAnnotation(t *testing.T) {
	type input struct {
		key  string
		vm   *kubevirtv1.VirtualMachine
		pvcs []*corev1.PersistentVolumeClaim
	}
	type output struct {
		vm  *kubevirtv1.VirtualMachine
		err error
	}

	storageClassName := "longhorn"
	volumeMode := corev1.PersistentVolumeBlock

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "ignore nil VM",
			given: input{
				key: "",
				vm:  nil,
			},
			expected: output{
				vm:  nil,
				err: nil,
			},
		},
		{
			name: "ignore VM without targetVolume entries",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"}}},
							}),
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{},
					},
				},
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"}}},
							}),
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{},
					},
				},
				err: nil,
			},
		},
		{
			name: "skip cleanup when VolumesChange condition is present",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume:          "new-pvc",
								},
							}),
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "new-pvc",
												},
											},
										},
									},
								},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineStatus{
						Conditions: []kubevirtv1.VirtualMachineCondition{
							{
								Type:   kubevirtv1.VirtualMachineConditionType(kubevirtv1.VirtualMachineInstanceVolumesChange),
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume:          "new-pvc",
								},
							}),
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "new-pvc",
												},
											},
										},
									},
								},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineStatus{
						Conditions: []kubevirtv1.VirtualMachineCondition{
							{
								Type:   kubevirtv1.VirtualMachineConditionType(kubevirtv1.VirtualMachineInstanceVolumesChange),
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				err: nil,
			},
		},
		{
			name: "skip cleanup when waitingStorageMigration annotation is present",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume:          "new-pvc",
								},
							}),
							util.AnnotationWaitingStorageMigration: "true",
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "new-pvc",
												},
											},
										},
									},
								},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineStatus{
						PrintableStatus: kubevirtv1.VirtualMachineStatusRunning,
					},
				},
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume:          "new-pvc",
								},
							}),
							util.AnnotationWaitingStorageMigration: "true",
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "new-pvc",
												},
											},
										},
									},
								},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineStatus{
						PrintableStatus: kubevirtv1.VirtualMachineStatusRunning,
					},
				},
				err: nil,
			},
		},
		{
			name: "skip cleanup when volume not yet pointing to targetVolume",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume:          "new-pvc",
								},
							}),
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "old-pvc",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume:          "new-pvc",
								},
							}),
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "old-pvc",
												},
											},
										},
									},
								},
							},
						},
					},
				},
				err: nil,
			},
		},
		{
			name: "cleanup annotation after PVC migration completes",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume:          "new-pvc",
								},
							}),
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "new-pvc",
												},
											},
										},
									},
								},
							},
						},
					},
					// No VolumesChange condition = migration completed
					Status: kubevirtv1.VirtualMachineStatus{
						PrintableStatus: kubevirtv1.VirtualMachineStatusRunning,
					},
				},
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "new-pvc",
							Annotations: map[string]string{
								util.AnnotationImageID: "default/image-abc",
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
							StorageClassName: &storageClassName,
							VolumeMode:       &volumeMode,
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("10Gi"),
								},
							},
						},
					},
				},
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{
										ObjectMeta: metav1.ObjectMeta{
											Name: "new-pvc",
											Annotations: map[string]string{
												util.AnnotationImageID: "default/image-abc",
											},
										},
										Spec: corev1.PersistentVolumeClaimSpec{
											AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
											StorageClassName: &storageClassName,
											VolumeMode:       &volumeMode,
											Resources: corev1.VolumeResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceStorage: resource.MustParse("10Gi"),
												},
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
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "new-pvc",
												},
											},
										},
									},
								},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineStatus{
						PrintableStatus: kubevirtv1.VirtualMachineStatusRunning,
					},
				},
				err: nil,
			},
		},
		{
			name: "cleanup annotation after PVC migration completes",
			given: input{
				key: "default/test",
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume:          "new-pvc",
								},
							}),
						},
					},
					Spec: kubevirtv1.VirtualMachineSpec{
						Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
							Spec: kubevirtv1.VirtualMachineInstanceSpec{
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "new-pvc",
												},
											},
										},
									},
								},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineStatus{
						PrintableStatus: kubevirtv1.VirtualMachineStatusRunning,
					},
				},
				pvcs: []*corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      "new-pvc",
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
							StorageClassName: &storageClassName,
							VolumeMode:       &volumeMode,
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("20Gi"),
								},
							},
						},
					},
				},
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{
									PersistentVolumeClaim: corev1.PersistentVolumeClaim{
										ObjectMeta: metav1.ObjectMeta{
											Name: "new-pvc",
										},
										Spec: corev1.PersistentVolumeClaimSpec{
											AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
											StorageClassName: &storageClassName,
											VolumeMode:       &volumeMode,
											Resources: corev1.VolumeResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceStorage: resource.MustParse("20Gi"),
												},
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
								Volumes: []kubevirtv1.Volume{
									{
										Name: "vol-1",
										VolumeSource: kubevirtv1.VolumeSource{
											PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
												PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
													ClaimName: "new-pvc",
												},
											},
										},
									},
								},
							},
						},
					},
					Status: kubevirtv1.VirtualMachineStatus{
						PrintableStatus: kubevirtv1.VirtualMachineStatusRunning,
					},
				},
				err: nil,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			if tc.given.vm != nil {
				err := clientset.Tracker().Add(tc.given.vm)
				require.NoError(t, err, "mock VM should add into fake tracker")
			}

			ctrl := &VMController{
				vmClient: newFakeVMClient(clientset),
				pvcCache: newFakePVCCache(tc.given.pvcs...),
			}

			actual, err := ctrl.CleanupTargetVolumeAnnotation(tc.given.key, tc.given.vm)

			if tc.expected.err != nil {
				assert.Error(t, err, "case %q", tc.name)
			} else {
				assert.NoError(t, err, "case %q", tc.name)
			}

			if tc.expected.vm == nil {
				assert.Nil(t, actual, "case %q", tc.name)
				return
			}

			require.NotNil(t, actual, "case %q", tc.name)

			// Compare annotation
			expectedAnno := tc.expected.vm.Annotations[util.AnnotationVolumeClaimTemplates]
			actualAnno := actual.Annotations[util.AnnotationVolumeClaimTemplates]

			if expectedAnno != "" {
				var expectedEntries, actualEntries []util.VolumeClaimTemplateEntry
				require.NoError(t, json.Unmarshal([]byte(expectedAnno), &expectedEntries), "unmarshal expected annotation")
				require.NoError(t, json.Unmarshal([]byte(actualAnno), &actualEntries), "unmarshal actual annotation")
				assert.Equal(t, expectedEntries, actualEntries, "annotation entries")
			}

			// Compare volumes unchanged
			if tc.expected.vm.Spec.Template != nil {
				require.NotNil(t, actual.Spec.Template)
				assert.Equal(t, tc.expected.vm.Spec.Template.Spec.Volumes, actual.Spec.Template.Spec.Volumes, "Volumes")
			}
		})
	}
}

func TestReconcileBackendStorageClone(t *testing.T) {
	targetVMName := "target-vm"
	sourceVMName := "source-vm"
	namespace := "default"
	storageClassName := "longhorn"
	volumeMode := corev1.PersistentVolumeFilesystem
	origFetchImageFromHelmValues := fetchImageFromHelmValues
	fetchImageFromHelmValues = func(_ kubernetes.Interface, _, _ string, _ []string) (settings.Image, error) {
		return settings.Image{
			Repository:      "rancher/shell",
			Tag:             "v0.7.0",
			ImagePullPolicy: corev1.PullIfNotPresent,
		}, nil
	}
	t.Cleanup(func() {
		fetchImageFromHelmValues = origFetchImageFromHelmValues
	})

	type input struct {
		key  string
		vm   *kubevirtv1.VirtualMachine
		pvcs []*corev1.PersistentVolumeClaim
		jobs []*batchv1.Job
	}

	type output struct {
		err         bool
		cloneStatus string
		cloneStage  string
		runStrategy *kubevirtv1.VirtualMachineRunStrategy
		createdPVC  bool
		createdJob  bool
		deletedJob  bool
		enqueue     bool
		pvc         *corev1.PersistentVolumeClaim
		job         *batchv1.Job
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "ignore nil VM",
			given: input{
				key: "",
				vm:  nil,
			},
			expected: output{},
		},
		{
			name: "ignore VM without clone annotations",
			given: input{
				key:  namespace + "/" + targetVMName,
				vm:   newTestVM(namespace, targetVMName, map[string]string{util.AnnotationBSCloneSourceVM: sourceVMName, util.AnnotationBSCloneRunStrategy: string(kubevirtv1.RunStrategyRerunOnFailure)}),
				pvcs: []*corev1.PersistentVolumeClaim{newTestSourcePVC(namespace, sourceVMName, storageClassName, volumeMode), newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound)},
			},
			expected: output{
				runStrategy: ptrRunStrategy(kubevirtv1.RunStrategyHalted),
			},
		},
		{
			name: "source PVC exists but cloned PVC does not - create PVC and enqueue retry",
			given: input{
				key:  namespace + "/" + targetVMName,
				vm:   newTestVM(namespace, targetVMName, newTestCloneInProgressAnnotations(sourceVMName)),
				pvcs: []*corev1.PersistentVolumeClaim{newTestSourcePVC(namespace, sourceVMName, storageClassName, volumeMode)},
			},
			expected: output{
				cloneStatus: util.CloneInProgress,
				runStrategy: ptrRunStrategy(kubevirtv1.RunStrategyHalted),
				createdPVC:  true,
				enqueue:     true,
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      backendstorage.PVCPrefix + "-" + targetVMName,
						Labels: map[string]string{
							backendstorage.PVCPrefix: targetVMName,
						},
						OwnerReferences: []metav1.OwnerReference{
							*metav1.NewControllerRef(
								newTestVM(namespace, targetVMName, newTestCloneInProgressAnnotations(sourceVMName)),
								kubevirtv1.VirtualMachineGroupVersionKind,
							),
						},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						DataSource: &corev1.TypedLocalObjectReference{
							Kind: "PersistentVolumeClaim",
							Name: backendstorage.PVCPrefix + "-" + sourceVMName,
						},
						StorageClassName: &storageClassName,
						VolumeMode:       &volumeMode,
					},
				},
			},
		},
		{
			name: "bound cloned PVC exists and stale job start time mismatch - delete stale job and enqueue retry",
			given: input{
				key:  namespace + "/" + targetVMName,
				vm:   newTestVMWithPersistentEFI(namespace, targetVMName, newTestCloneInProgressAnnotations(sourceVMName)),
				pvcs: []*corev1.PersistentVolumeClaim{newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound)},
				jobs: []*batchv1.Job{
					newTestBackendStorageJob(
						mustBuildBackendStorageJob(t, &VMController{
							clientset: k8sfake.NewSimpleClientset(),
						},
							newTestVMWithPersistentEFI(namespace, targetVMName, newTestCloneInProgressAnnotations(sourceVMName)),
							sourceVMName,
							newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound),
							"backend-storage-"+targetVMName,
							util.CloneActionRenameEFI,
						),
						func(job *batchv1.Job) {
							job.Annotations[util.AnnotationBSCloneStartTime] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
						},
					),
				},
			},
			expected: output{
				cloneStatus: util.CloneInProgress,
				runStrategy: ptrRunStrategy(kubevirtv1.RunStrategyHalted),
				deletedJob:  true,
				enqueue:     true,
			},
		},
		{
			name: "bound cloned PVC exists but rename job does not - create job and enqueue retry",
			given: input{
				key:  namespace + "/" + targetVMName,
				vm:   newTestVMWithPersistentEFI(namespace, targetVMName, newTestCloneInProgressAnnotations(sourceVMName)),
				pvcs: []*corev1.PersistentVolumeClaim{newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound)},
			},
			expected: output{
				cloneStatus: util.CloneInProgress,
				runStrategy: ptrRunStrategy(kubevirtv1.RunStrategyHalted),
				createdJob:  true,
				enqueue:     true,
				job: mustBuildBackendStorageJob(t, &VMController{
					clientset: k8sfake.NewSimpleClientset(),
				},
					newTestVMWithPersistentEFI(namespace, targetVMName, newTestCloneInProgressAnnotations(sourceVMName)),
					sourceVMName,
					newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound),
					"backend-storage-"+targetVMName,
					util.CloneActionRenameEFI,
				),
			},
		},
		{
			name: "bound cloned PVC exists and VM has persistent TPM only - mark clone complete without backend storage job",
			given: input{
				key:  namespace + "/" + targetVMName,
				vm:   newTestVMWithPersistentTPM(namespace, targetVMName, newTestCloneInProgressAnnotationsWithAction(sourceVMName, "")),
				pvcs: []*corev1.PersistentVolumeClaim{newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound)},
			},
			expected: output{
				cloneStatus: util.CloneComplete,
				runStrategy: ptrRunStrategy(kubevirtv1.RunStrategyRerunOnFailure),
			},
		},
		{
			name: "clone exceeded timeout - mark clone failed and keep VM halted",
			given: input{
				key: namespace + "/" + targetVMName,
				vm: func() *kubevirtv1.VirtualMachine {
					annotations := newTestCloneInProgressAnnotations(sourceVMName)
					annotations[util.AnnotationBSCloneStartTime] = time.Now().Add(-backendStorageCloneTimeout - time.Minute).UTC().Format(time.RFC3339)
					return newTestVMWithPersistentEFI(namespace, targetVMName, annotations)
				}(),
				pvcs: []*corev1.PersistentVolumeClaim{newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound)},
			},
			expected: output{
				cloneStatus: util.CloneFailed,
				runStrategy: ptrRunStrategy(kubevirtv1.RunStrategyHalted),
			},
		},
		{
			name: "bound cloned PVC and running rename job exist - no create and enqueue retry",
			given: input{
				key:  namespace + "/" + targetVMName,
				vm:   newTestVMWithPersistentEFI(namespace, targetVMName, newTestCloneInProgressAnnotations(sourceVMName)),
				pvcs: []*corev1.PersistentVolumeClaim{newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound)},
				jobs: []*batchv1.Job{
					newTestBackendStorageJob(
						mustBuildBackendStorageJob(t, &VMController{
							clientset: k8sfake.NewSimpleClientset(),
						},
							newTestVMWithPersistentEFI(namespace, targetVMName, newTestCloneInProgressAnnotations(sourceVMName)),
							sourceVMName,
							newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound),
							"backend-storage-"+targetVMName,
							util.CloneActionRenameEFI,
						),
						func(job *batchv1.Job) {
							job.Status = batchv1.JobStatus{
								Active: 1,
							}
						},
					),
				},
			},
			expected: output{
				cloneStatus: util.CloneInProgress,
				runStrategy: ptrRunStrategy(kubevirtv1.RunStrategyHalted),
				enqueue:     true,
			},
		},
		{
			name: "bound cloned PVC and deleting rename job exist - wait for deletion and enqueue retry",
			given: input{
				key:  namespace + "/" + targetVMName,
				vm:   newTestVMWithPersistentEFI(namespace, targetVMName, newTestCloneInProgressAnnotations(sourceVMName)),
				pvcs: []*corev1.PersistentVolumeClaim{newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound)},
				jobs: []*batchv1.Job{
					newTestBackendStorageJob(
						mustBuildBackendStorageJob(t, &VMController{
							clientset: k8sfake.NewSimpleClientset(),
						},
							newTestVMWithPersistentEFI(namespace, targetVMName, newTestCloneInProgressAnnotations(sourceVMName)),
							sourceVMName,
							newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound),
							"backend-storage-"+targetVMName,
							util.CloneActionRenameEFI,
						),
						func(job *batchv1.Job) {
							now := metav1.NewTime(time.Now().UTC())
							job.DeletionTimestamp = &now
						},
					),
				},
			},
			expected: output{
				cloneStatus: util.CloneInProgress,
				runStrategy: ptrRunStrategy(kubevirtv1.RunStrategyHalted),
				enqueue:     true,
			},
		},
		{
			name: "job failed with max retries reached - mark clone failed and keep VM halted",
			given: input{
				key: namespace + "/" + targetVMName,
				vm: func() *kubevirtv1.VirtualMachine {
					annotations := newTestCloneInProgressAnnotations(sourceVMName)
					annotations[util.AnnotationBSCloneRetries] = strconv.Itoa(maxBackendStorageCloneRetries)
					return newTestVMWithPersistentEFI(namespace, targetVMName, annotations)
				}(),
				pvcs: []*corev1.PersistentVolumeClaim{newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound)},
				jobs: []*batchv1.Job{
					newTestBackendStorageJob(
						mustBuildBackendStorageJob(t, &VMController{
							clientset: k8sfake.NewSimpleClientset(),
						},
							newTestVMWithPersistentEFI(namespace, targetVMName, newTestCloneInProgressAnnotations(sourceVMName)),
							sourceVMName,
							newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound),
							"backend-storage-"+targetVMName,
							util.CloneActionRenameEFI,
						),
						func(job *batchv1.Job) {
							job.Status = batchv1.JobStatus{
								Failed: 1,
								Conditions: []batchv1.JobCondition{{
									Type:   batchv1.JobFailed,
									Status: corev1.ConditionTrue,
								}},
							}
						},
					),
				},
			},
			expected: output{
				cloneStatus: util.CloneFailed,
				runStrategy: ptrRunStrategy(kubevirtv1.RunStrategyHalted),
				deletedJob:  true,
			},
		},
		{
			name: "job completed - mark clone pre-completed and enqueue retry",
			given: input{
				key:  namespace + "/" + targetVMName,
				vm:   newTestVMWithPersistentEFI(namespace, targetVMName, newTestCloneInProgressAnnotations(sourceVMName)),
				pvcs: []*corev1.PersistentVolumeClaim{newTestSourcePVC(namespace, sourceVMName, storageClassName, volumeMode), newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound)},
				jobs: []*batchv1.Job{
					newTestBackendStorageJob(
						mustBuildBackendStorageJob(t, &VMController{
							clientset: k8sfake.NewSimpleClientset(),
						},
							newTestVMWithPersistentEFI(namespace, targetVMName, newTestCloneInProgressAnnotations(sourceVMName)),
							sourceVMName,
							newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound),
							"backend-storage-"+targetVMName,
							util.CloneActionRenameEFI,
						),
						func(job *batchv1.Job) {
							job.Status = batchv1.JobStatus{
								Succeeded: 1,
								Conditions: []batchv1.JobCondition{{
									Type:   batchv1.JobComplete,
									Status: corev1.ConditionTrue,
								}},
							}
						},
					),
				},
			},
			expected: output{
				cloneStatus: util.CloneInProgress,
				cloneStage:  util.CloneStagePreCompleted,
				runStrategy: ptrRunStrategy(kubevirtv1.RunStrategyHalted),
				deletedJob:  true,
				enqueue:     true,
			},
		},
		{
			name: "pre-completed clone without job - mark clone complete and restore RunStrategy",
			given: input{
				key: namespace + "/" + targetVMName,
				vm: func() *kubevirtv1.VirtualMachine {
					annotations := newTestCloneInProgressAnnotations(sourceVMName)
					annotations[util.AnnotationBSCloneStage] = util.CloneStagePreCompleted
					return newTestVMWithPersistentEFI(namespace, targetVMName, annotations)
				}(),
				pvcs: []*corev1.PersistentVolumeClaim{newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound)},
			},
			expected: output{
				cloneStatus: util.CloneComplete,
				runStrategy: ptrRunStrategy(kubevirtv1.RunStrategyRerunOnFailure),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			if tc.given.vm != nil {
				err := clientset.Tracker().Add(tc.given.vm)
				require.NoError(t, err, "mock VM should add into fake tracker")
			}

			k8sClientset := k8sfake.NewSimpleClientset()
			recorder := record.NewFakeRecorder(10)
			pvcClient := newFakePVCClient(tc.given.pvcs...)
			jobClient := newFakeJobClient(tc.given.jobs...)
			vmController := &fakeRequeueVMController{
				fakeVMClient: newFakeVMClient(clientset),
			}

			ctrl := &VMController{
				vmClient:     newFakeVMClient(clientset),
				vmController: vmController,
				recorder:     recorder,
				pvcClient:    pvcClient,
				jobClient:    jobClient,
				pvcCache:     newFakePVCCache(tc.given.pvcs...),
				jobCache:     newFakeJobCache(tc.given.jobs...),
				clientset:    k8sClientset,
			}

			result, err := ctrl.ReconcileBackendStorageClone(tc.given.key, tc.given.vm)
			if tc.expected.err {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if result != nil {
				assert.Equal(t, tc.expected.cloneStatus, result.Annotations[util.AnnotationBSCloneStatus])
				assert.Equal(t, tc.expected.cloneStage, result.Annotations[util.AnnotationBSCloneStage])
				if tc.expected.runStrategy == nil {
					assert.Nil(t, result.Spec.RunStrategy)
				} else {
					require.NotNil(t, result.Spec.RunStrategy)
					assert.Equal(t, *tc.expected.runStrategy, *result.Spec.RunStrategy)
				}
			}

			if tc.expected.createdPVC {
				require.Len(t, pvcClient.created, 1)
				assertCreatedTestPVC(t, tc.expected.pvc, pvcClient.created[0])
			} else {
				assert.Empty(t, pvcClient.created)
			}

			if tc.expected.createdJob {
				require.Len(t, jobClient.created, 1)
				assertCreatedTestJob(t, tc.expected.job, jobClient.created[0])
			} else {
				assert.Empty(t, jobClient.created)
			}

			if tc.expected.deletedJob {
				require.Len(t, jobClient.deleted, 1)
				assert.Equal(t, namespace, jobClient.deleted[0].Namespace)
				assert.Equal(t, "backend-storage-"+targetVMName, jobClient.deleted[0].Name)
			} else {
				assert.Empty(t, jobClient.deleted)
			}

			if tc.expected.enqueue {
				require.Len(t, vmController.enqueues, 1)
				assert.Equal(t, namespace, vmController.enqueues[0].namespace)
				assert.Equal(t, targetVMName, vmController.enqueues[0].name)
				assert.Equal(t, 5*time.Second, vmController.enqueues[0].after)
			} else {
				assert.Empty(t, vmController.enqueues)
			}
		})
	}
}

func TestBuildBackendStorageJobRequiresConfiguredImage(t *testing.T) {
	namespace := "default"
	sourceVMName := "source-vm"
	targetVMName := "target-vm"
	origFetchImageFromHelmValues := fetchImageFromHelmValues
	fetchImageFromHelmValues = func(_ kubernetes.Interface, _, _ string, _ []string) (settings.Image, error) {
		return settings.Image{}, assert.AnError
	}
	t.Cleanup(func() {
		fetchImageFromHelmValues = origFetchImageFromHelmValues
	})

	ctrl := &VMController{
		clientset: k8sfake.NewSimpleClientset(),
	}

	job, err := ctrl.buildBackendStorageJob(
		newTestVMWithPersistentEFI(namespace, targetVMName, newTestCloneInProgressAnnotations(sourceVMName)),
		sourceVMName,
		newTestClonedPVC(namespace, targetVMName, corev1.ClaimBound),
		"backend-storage-"+targetVMName,
		util.CloneActionRenameEFI,
	)

	require.Error(t, err)
	assert.Nil(t, job)
}
