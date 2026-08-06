package util_test

import (
	"testing"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/stretchr/testify/require"
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
			clientset := fakegenerated.NewSimpleClientset()
			cache := fakeclients.BackingImageCache(clientset.LonghornV1beta2().BackingImages)

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
	clientset := fakegenerated.NewSimpleClientset(
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
	cache := fakeclients.BackingImageCache(clientset.LonghornV1beta2().BackingImages)

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
	clientset := fakegenerated.NewSimpleClientset(
		&lhv1beta2.BackingImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      legacyBackingImage,
				Namespace: util.LonghornSystemNamespaceName,
			},
		},
	)
	cache := fakeclients.BackingImageCache(clientset.LonghornV1beta2().BackingImages)

	name, err := util.GetBackingImageName(cache, vmi)
	require.NoError(t, err)
	require.Equal(t, legacyBackingImage, name)

	bi, err := util.GetBackingImage(cache, vmi)
	require.NoError(t, err)
	require.Equal(t, legacyBackingImage, bi.Name)
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
	clientset := fakegenerated.NewSimpleClientset()
	cache := fakeclients.BackingImageCache(clientset.LonghornV1beta2().BackingImages)

	params, err := util.GetImageStorageClassParameters(cache, vmi)
	require.NoError(t, err)
	require.Equal(t, restoredBackingImage, params[util.LonghornOptionBackingImageName])
	require.Equal(t, "2", params["numberOfReplicas"])
}
