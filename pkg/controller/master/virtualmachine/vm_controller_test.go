package virtualmachine

import (
	"encoding/json"
	"testing"

	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	kubevirtv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
)

// fakeDataVolumeClient implements ctlcdiv1.DataVolumeClient for testing.
type fakeDataVolumeClient struct {
	dvs map[string]*cdiv1.DataVolume // "namespace/name" -> DataVolume
}

func newFakeDataVolumeClient(dvs ...*cdiv1.DataVolume) *fakeDataVolumeClient {
	m := make(map[string]*cdiv1.DataVolume)
	for _, dv := range dvs {
		m[dv.Namespace+"/"+dv.Name] = dv
	}
	return &fakeDataVolumeClient{dvs: m}
}

func (c *fakeDataVolumeClient) Get(namespace, name string, _ metav1.GetOptions) (*cdiv1.DataVolume, error) {
	if dv, ok := c.dvs[namespace+"/"+name]; ok {
		return dv, nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "cdi.kubevirt.io", Resource: "datavolumes"}, name)
}

func (c *fakeDataVolumeClient) Create(_ *cdiv1.DataVolume) (*cdiv1.DataVolume, error) {
	panic("not implemented")
}

func (c *fakeDataVolumeClient) Update(_ *cdiv1.DataVolume) (*cdiv1.DataVolume, error) {
	panic("not implemented")
}

func (c *fakeDataVolumeClient) UpdateStatus(_ *cdiv1.DataVolume) (*cdiv1.DataVolume, error) {
	panic("not implemented")
}

func (c *fakeDataVolumeClient) Delete(_, _ string, _ *metav1.DeleteOptions) error {
	panic("not implemented")
}

func (c *fakeDataVolumeClient) List(_ string, _ metav1.ListOptions) (*cdiv1.DataVolumeList, error) {
	panic("not implemented")
}

func (c *fakeDataVolumeClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("not implemented")
}

func (c *fakeDataVolumeClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (*cdiv1.DataVolume, error) {
	panic("not implemented")
}

func (c *fakeDataVolumeClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*cdiv1.DataVolume, *cdiv1.DataVolumeList], error) {
	panic("not implemented")
}

// fakePVCCache implements v1.PersistentVolumeClaimCache for testing.
type fakePVCCache struct {
	pvcs map[string]*corev1.PersistentVolumeClaim // "namespace/name" -> PVC
}

func newFakePVCCache(pvcs ...*corev1.PersistentVolumeClaim) *fakePVCCache {
	m := make(map[string]*corev1.PersistentVolumeClaim)
	for _, pvc := range pvcs {
		m[pvc.Namespace+"/"+pvc.Name] = pvc
	}
	return &fakePVCCache{pvcs: m}
}

func (c *fakePVCCache) Get(namespace, name string) (*corev1.PersistentVolumeClaim, error) {
	if pvc, ok := c.pvcs[namespace+"/"+name]; ok {
		return pvc, nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "persistentvolumeclaims"}, name)
}

func (c *fakePVCCache) List(_ string, _ labels.Selector) ([]*corev1.PersistentVolumeClaim, error) {
	panic("not implemented")
}

func (c *fakePVCCache) AddIndexer(_ string, _ generic.Indexer[*corev1.PersistentVolumeClaim]) {
	panic("not implemented")
}

func (c *fakePVCCache) GetByIndex(_, _ string) ([]*corev1.PersistentVolumeClaim, error) {
	panic("not implemented")
}

// mustMarshalEntries is a test helper that marshals VolumeClaimTemplateEntry slice to JSON string.
func mustMarshalEntries(entries []util.VolumeClaimTemplateEntry) string {
	data, err := json.Marshal(entries)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// newFakeVMClient creates a fakeVMClient from a fake clientset.
// Reuses the fakeVMClient type defined in vmi_network_controller_test.go.
func newFakeVMClient(clientset *fake.Clientset) fakeVMClient {
	return fakeVMClient(clientset.KubevirtV1().VirtualMachines)
}

func TestSetTargetVolumeStrategy(t *testing.T) {
	type input struct {
		key string
		vm  *kubevirtv1.VirtualMachine
		dvs []*cdiv1.DataVolume
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
								{PVC: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"}}},
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
								{PVC: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"}}},
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
									PVC:          corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume: "new-pvc",
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
				dvs: nil,
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
									PVC:          corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume: "new-pvc",
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
						UpdateVolumesStrategy: &migrationStrategy,
					},
				},
				err: nil,
			},
		},
		{
			name: "replace PVC volume with DataVolume",
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
									PVC:          corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume: "new-dv",
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
				dvs: []*cdiv1.DataVolume{
					{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "new-dv"}},
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
									PVC:          corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume: "new-dv",
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
											DataVolume: &kubevirtv1.DataVolumeSource{
												Name:         "new-dv",
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
								{PVC: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-keep"}}},
								{
									PVC:          corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-replace"}},
									TargetVolume: "pvc-new",
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
				dvs: nil,
			},
			expected: output{
				vm: &kubevirtv1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test",
						UID:       "fake-vm-uid",
						Annotations: map[string]string{
							util.AnnotationVolumeClaimTemplates: mustMarshalEntries([]util.VolumeClaimTemplateEntry{
								{PVC: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-keep"}}},
								{
									PVC:          corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-replace"}},
									TargetVolume: "pvc-new",
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
				dataVolumeClient: newFakeDataVolumeClient(tc.given.dvs...),
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
								{PVC: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"}}},
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
								{PVC: corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1"}}},
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
									PVC:          corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume: "new-pvc",
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
									PVC:          corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume: "new-pvc",
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
									PVC:          corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume: "new-pvc",
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
									PVC:          corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume: "new-pvc",
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
									PVC:          corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume: "new-pvc",
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
									PVC: corev1.PersistentVolumeClaim{
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
				},
				err: nil,
			},
		},
		{
			name: "cleanup annotation after DataVolume migration completes",
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
									PVC:          corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "old-pvc"}},
									TargetVolume: "new-dv",
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
											DataVolume: &kubevirtv1.DataVolumeSource{
												Name: "new-dv",
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
							Name:      "new-dv",
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
									PVC: corev1.PersistentVolumeClaim{
										ObjectMeta: metav1.ObjectMeta{
											Name: "new-dv",
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
											DataVolume: &kubevirtv1.DataVolumeSource{
												Name: "new-dv",
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
