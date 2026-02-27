package vm

import (
	"fmt"
	"testing"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestGetSCNameFromImgID(t *testing.T) {
	type input struct {
		imageId string
		images  []*harvesterv1.VirtualMachineImage
	}
	type output struct {
		scname string
		err    error
	}
	testcases := []struct {
		desc string
		in   input
		ex   output
	}{
		{
			desc: "all good",
			in: input{
				imageId: "default/test",
				images: []*harvesterv1.VirtualMachineImage{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "notthisone",
							Namespace: "default",
							Annotations: map[string]string{
								util.AnnotationStorageClassName: "barfoo",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test",
							Namespace: "default",
							Annotations: map[string]string{
								util.AnnotationStorageClassName: "foobar",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test",
							Namespace: "alsonotthisone",
							Annotations: map[string]string{
								util.AnnotationStorageClassName: "barfoo",
							},
						},
					},
				},
			},
			ex: output{
				scname: "foobar",
				err:    nil,
			},
		},
		{
			desc: "image not found",
			in: input{
				imageId: "default/test",
				images: []*harvesterv1.VirtualMachineImage{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "notthisone",
							Namespace: "default",
							Annotations: map[string]string{
								util.AnnotationStorageClassName: "barfoo",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test",
							Namespace: "alsonotthisone",
							Annotations: map[string]string{
								util.AnnotationStorageClassName: "barfoo",
							},
						},
					},
				},
			},
			ex: output{
				scname: "",
				err:    fmt.Errorf("virtualmachineimages.harvesterhci.io \"test\" not found"),
			},
		},
		{
			desc: "image doesn't have annotation",
			in: input{
				imageId: "default/test",
				images: []*harvesterv1.VirtualMachineImage{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:        "test",
							Namespace:   "default",
							Annotations: map[string]string{},
						},
					},
				},
			},
			ex: output{
				scname: "",
				err:    fmt.Errorf("VMImage default/test does not have an annotation %s", util.AnnotationStorageClassName),
			},
		},
	}

	for _, tc := range testcases {
		var res output
		var clientset = fake.NewSimpleClientset()

		for _, img := range tc.in.images {
			err := clientset.Tracker().Add(img)
			assert.Nil(t, err, "failed creating mock resources")
		}

		var handler = &vmActionHandler{
			vmImageCache: fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
		}

		res.scname, res.err = handler.getSCNameFromImgID(tc.in.imageId)

		assert.Equal(t, res.scname, tc.ex.scname, "case %q", tc.desc)
		if tc.ex.err != nil && res.err != nil {
			assert.Equal(t, res.err.Error(), tc.ex.err.Error(), "case %q", tc.desc)
		} else {
			assert.Equal(t, res.err, tc.ex.err, "case %q", tc.desc)
		}
	}
}

func TestGetVMImageBackend(t *testing.T) {
	type input struct {
		storageclasses []*storagev1.StorageClass
		targetSCName   string
		pvc            *corev1.PersistentVolumeClaim
	}
	type output struct {
		vmiBackend harvesterv1.VMIBackend
		err        error
	}
	testcases := []struct {
		desc string
		in   input
		ex   output
	}{
		{
			desc: "PVC \"longhorn-templateversion-.*\" use backend \"BackingImage\"",
			in: input{
				storageclasses: []*storagev1.StorageClass{},
				targetSCName:   "longhorn-templateversion-foobar-abcdef",
				pvc:            &corev1.PersistentVolumeClaim{},
			},
			ex: output{
				vmiBackend: harvesterv1.VMIBackendBackingImage,
				err:        nil,
			},
		},
		{
			desc: "PVC with non-Longhorn storage class use CDI backend",
			in: input{
				storageclasses: []*storagev1.StorageClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foobar",
						},
						Provisioner: "foobar-provisioner",
					},
				},
				targetSCName: "foobar",
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foobar-pvc",
						Namespace: "foobar",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: ptr.To("foobar"),
					},
				},
			},
			ex: output{
				vmiBackend: harvesterv1.VMIBackendCDI,
				err:        nil,
			},
		},
		{
			desc: "PVC using Longhorn storage class with v2 data engine use CDI backend",
			in: input{
				storageclasses: []*storagev1.StorageClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "longhorn-v2",
						},
						Provisioner: "driver.longhorn.io",
						Parameters: map[string]string{
							util.ParamDataEngine: string(longhorn.DataEngineTypeV2),
						},
					},
				},
				targetSCName: "longhorn-v2",
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{},
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: ptr.To("longhorn-v2"),
					},
				},
			},
			ex: output{
				vmiBackend: harvesterv1.VMIBackendCDI,
				err:        nil,
			},
		},
		{
			desc: "PVC using Longhorn storage class with v1 data engine use BackingImage backend",
			in: input{
				storageclasses: []*storagev1.StorageClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "longhorn-v1",
						},
						Provisioner: "driver.longhorn.io",
						Parameters: map[string]string{
							util.ParamDataEngine: string(longhorn.DataEngineTypeV1),
						},
					},
				},
				targetSCName: "longhorn-v1",
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{},
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: ptr.To("longhorn-v1"),
					},
				},
			},
			ex: output{
				vmiBackend: harvesterv1.VMIBackendBackingImage,
				err:        nil,
			},
		},
		{
			desc: "PVC using Longhorn storage class with no data engine specified use BackingImage backend",
			in: input{
				storageclasses: []*storagev1.StorageClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "longhorn-v1-no-de",
						},
						Provisioner: "driver.longhorn.io",
						Parameters:  map[string]string{},
					},
				},
				targetSCName: "longhorn-v1-no-de",
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{},
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: ptr.To("longhorn-v1-no-de"),
					},
				},
			},
			ex: output{
				vmiBackend: harvesterv1.VMIBackendBackingImage,
				err:        nil,
			},
		},
		{
			desc: "error case: storage class not found",
			in: input{
				storageclasses: []*storagev1.StorageClass{},
				targetSCName:   "no-exist",
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{},
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: ptr.To("no-exist"),
					},
				},
			},
			ex: output{
				vmiBackend: "",
				err:        fmt.Errorf("storageclasses.storage.k8s.io \"no-exist\" not found"),
			},
		},
	}

	for _, tc := range testcases {
		var res output
		var clientset = fake.NewSimpleClientset()

		for _, sc := range tc.in.storageclasses {
			err := clientset.Tracker().Add(sc)
			assert.Nil(t, err, "failed creating mock resources")
		}

		var handler = &vmActionHandler{
			storageClassCache: fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
		}

		res.vmiBackend, res.err = handler.getVMImageBackend(tc.in.targetSCName, tc.in.pvc)

		assert.Equal(t, res.vmiBackend, tc.ex.vmiBackend)
		if tc.ex.err != nil && res.err != nil {
			assert.Equal(t, res.err.Error(), tc.ex.err.Error(), "case %q", tc.desc)
		} else {
			assert.Equal(t, res.err, tc.ex.err, "case %q", tc.desc)
		}
	}
}
