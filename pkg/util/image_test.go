package util_test

import (
	"testing"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	fakegenerated "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestGetBackingImageNameRestoreFromURL(t *testing.T) {
	const (
		imageUID               = "faec4fac-4330-46c7-b6fb-314288a012cd"
		restoredBackingImage   = "vmi-06791a48-8d0d-4895-999b-28296f0e1c10"
		restoreBackingImageURL = "s3://mybucket@pcloud/?backingImage=" + restoredBackingImage
	)

	tests := []struct {
		name     string
		vmi      *harvesterv1.VirtualMachineImage
		expected string
	}{
		{
			name: "restore image uses backingImage from url",
			vmi: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-zxhcn",
					Namespace: "default",
					UID:       imageUID,
				},
				Spec: harvesterv1.VirtualMachineImageSpec{
					SourceType: harvesterv1.VirtualMachineImageSourceTypeRestore,
					URL:        restoreBackingImageURL,
				},
			},
			expected: restoredBackingImage,
		},
		{
			name: "restore image without backingImage in url uses image uid",
			vmi: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-zxhcn",
					Namespace: "default",
					UID:       imageUID,
				},
				Spec: harvesterv1.VirtualMachineImageSpec{
					SourceType: harvesterv1.VirtualMachineImageSourceTypeRestore,
					URL:        "s3://mybucket@pcloud/",
				},
			},
			expected: "vmi-" + imageUID,
		},
		{
			name: "download image ignores backingImage url parameter",
			vmi: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-zxhcn",
					Namespace: "default",
					UID:       imageUID,
				},
				Spec: harvesterv1.VirtualMachineImageSpec{
					SourceType: harvesterv1.VirtualMachineImageSourceTypeDownload,
					URL:        restoreBackingImageURL,
				},
			},
			expected: "vmi-" + imageUID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := fakegenerated.NewSimpleClientset()
			cache := fakeclients.BackingImageCache(cs.LonghornV1beta2().BackingImages)

			name, err := util.GetBackingImageName(cache, tt.vmi)
			require.NoError(t, err)
			require.Equal(t, tt.expected, name)
		})
	}
}

func TestGetBackingImageRestoreUsesURLName(t *testing.T) {
	const (
		legacyBackingImage   = "default-image-zxhcn"
		restoredBackingImage = "vmi-06791a48-8d0d-4895-999b-28296f0e1c10"
	)

	vmi := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "image-zxhcn",
			Namespace: "default",
			UID:       "faec4fac-4330-46c7-b6fb-314288a012cd",
		},
		Spec: harvesterv1.VirtualMachineImageSpec{
			SourceType: harvesterv1.VirtualMachineImageSourceTypeRestore,
			URL:        "s3://mybucket@pcloud/?backingImage=" + restoredBackingImage,
		},
	}
	cs := fakegenerated.NewSimpleClientset(
		&lhv1beta2.BackingImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      legacyBackingImage,
				Namespace: util.LonghornSystemNamespaceName,
			},
		},
		&lhv1beta2.BackingImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      restoredBackingImage,
				Namespace: util.LonghornSystemNamespaceName,
			},
		},
	)
	cache := fakeclients.BackingImageCache(cs.LonghornV1beta2().BackingImages)

	bi, err := util.GetBackingImage(cache, vmi)
	require.NoError(t, err)
	require.Equal(t, restoredBackingImage, bi.Name)
}

func TestGetBackingImageNameRestoreUsesExistingLegacyName(t *testing.T) {
	const legacyBackingImage = "default-image-zxhcn"

	vmi := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "image-zxhcn",
			Namespace: "default",
			UID:       "faec4fac-4330-46c7-b6fb-314288a012cd",
		},
		Spec: harvesterv1.VirtualMachineImageSpec{
			SourceType: harvesterv1.VirtualMachineImageSourceTypeRestore,
			URL:        "s3://mybucket@pcloud/?backingImage=vmi-06791a48-8d0d-4895-999b-28296f0e1c10",
		},
	}
	cs := fakegenerated.NewSimpleClientset(
		&lhv1beta2.BackingImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      legacyBackingImage,
				Namespace: util.LonghornSystemNamespaceName,
			},
		},
	)
	cache := fakeclients.BackingImageCache(cs.LonghornV1beta2().BackingImages)

	name, err := util.GetBackingImageName(cache, vmi)
	require.NoError(t, err)
	require.Equal(t, legacyBackingImage, name)

	bi, err := util.GetBackingImage(cache, vmi)
	require.NoError(t, err)
	require.Equal(t, legacyBackingImage, bi.Name)
}

func TestGetImageDiskSizeQuantity(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)

	tests := []struct {
		name        string
		virtualSize int64
		size        int64
		expected    int64
	}{
		{
			name:        "virtual size is used when larger",
			virtualSize: 5 * gib,
			size:        1 * gib,
			expected:    5 * gib,
		},
		{
			name:        "artifact size is used when larger",
			virtualSize: 1 * gib,
			size:        3 * gib,
			expected:    3 * gib,
		},
		{
			name:        "non-GiB-aligned size is rounded up to the next GiB",
			virtualSize: gib + 1,
			size:        0,
			expected:    2 * gib,
		},
		{
			name:        "zero sizes result in a zero quantity",
			virtualSize: 0,
			size:        0,
			expected:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			image := &harvesterv1.VirtualMachineImage{
				Status: harvesterv1.VirtualMachineImageStatus{
					VirtualSize: tt.virtualSize,
					Size:        tt.size,
				},
			}

			quantity, err := util.GetImageDiskSizeQuantity(image)
			require.NoError(t, err)
			require.Equal(t, tt.expected, quantity.Value())
		})
	}
}

func TestGetPVCSourceImage(t *testing.T) {
	image := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-image",
			Namespace: "default",
		},
	}

	goldenImage := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
		},
	}

	tests := []struct {
		name     string
		pvc      *corev1.PersistentVolumeClaim
		expected *harvesterv1.VirtualMachineImage
	}{
		{
			name: "pvc with imageId annotation resolves the referenced image",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						util.AnnotationImageID: "default/test-image",
					},
				},
			},
			expected: image,
		},
		{
			name: "golden image pvc resolves the same-named image",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						util.AnnotationGoldenImage: "true",
					},
				},
			},
			expected: goldenImage,
		},
		{
			name: "pvc without any image annotation has no source image",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-pvc",
					Namespace: "default",
				},
			},
			expected: nil,
		},
		{
			name: "malformed imageId annotation has no source image",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						util.AnnotationImageID: "test-image",
					},
				},
			},
			expected: nil,
		},
		{
			name: "imageId annotation referencing a non-existent image has no source image",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						util.AnnotationImageID: "default/does-not-exist",
					},
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := fakegenerated.NewSimpleClientset(image, goldenImage)
			cache := fakeclients.VirtualMachineImageCache(cs.HarvesterhciV1beta1().VirtualMachineImages)

			result, err := util.GetPVCSourceImage(tt.pvc, cache)
			require.NoError(t, err)
			if tt.expected == nil {
				require.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			require.Equal(t, tt.expected.Name, result.Name)
			require.Equal(t, tt.expected.Namespace, result.Namespace)
		})
	}
}

func TestGetImageStorageClassParametersRestoreFromURL(t *testing.T) {
	const restoredBackingImage = "vmi-06791a48-8d0d-4895-999b-28296f0e1c10"

	vmi := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "image-zxhcn",
			Namespace: "default",
			UID:       "faec4fac-4330-46c7-b6fb-314288a012cd",
		},
		Spec: harvesterv1.VirtualMachineImageSpec{
			SourceType: harvesterv1.VirtualMachineImageSourceTypeRestore,
			URL:        "s3://mybucket@pcloud/?backingImage=" + restoredBackingImage,
			StorageClassParameters: map[string]string{
				"numberOfReplicas": "2",
			},
		},
	}
	cs := fakegenerated.NewSimpleClientset()
	cache := fakeclients.BackingImageCache(cs.LonghornV1beta2().BackingImages)

	params, err := util.GetImageStorageClassParameters(cache, vmi)
	require.NoError(t, err)
	require.Equal(t, restoredBackingImage, params[util.LonghornOptionBackingImageName])
	require.Equal(t, "2", params["numberOfReplicas"])
}
