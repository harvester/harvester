package virtualmachineimage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestVirtualMachineImageValidator_Create(t *testing.T) {
	tests := []struct {
		name        string
		vmi         *harvesterv1.VirtualMachineImage
		sarDenied   bool
		expectError bool
	}{
		{
			name: "download image with valid URL - pass",
			vmi: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "test-img", Namespace: "default"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend:     harvesterv1.VMIBackendBackingImage,
					DisplayName: "test-image",
					SourceType:  harvesterv1.VirtualMachineImageSourceTypeDownload,
					URL:         "http://example.com/image.iso",
				},
			},
			expectError: false,
		},
		{
			name: "clone image, SAR denied for source image - fail",
			vmi: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "test-clone", Namespace: "default"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend:     harvesterv1.VMIBackendBackingImage,
					DisplayName: "test-clone-image",
					SourceType:  harvesterv1.VirtualMachineImageSourceTypeClone,
					SecurityParameters: &harvesterv1.VirtualMachineImageSecurityParameters{
						CryptoOperation:      harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
						SourceImageName:      "source-image",
						SourceImageNamespace: "default",
					},
				},
			},
			sarDenied:   true,
			expectError: true,
		},
		{
			// SAR passes but source image doesn't exist — confirms the check proceeds past SAR
			name: "clone image, SAR allowed, source not found - fail",
			vmi: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "test-clone-2", Namespace: "default"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend:     harvesterv1.VMIBackendBackingImage,
					DisplayName: "test-clone-image-2",
					SourceType:  harvesterv1.VirtualMachineImageSourceTypeClone,
					SecurityParameters: &harvesterv1.VirtualMachineImageSecurityParameters{
						CryptoOperation:      harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
						SourceImageName:      "source-image",
						SourceImageNamespace: "default",
					},
				},
			},
			sarDenied:   false,
			expectError: true,
		},
	}

	allowedFakeSAR := fakeclients.AllowedSARClient()
	denyFakeSAR := fakeclients.DeniedSARClient()

	fakeRequest := fakeclients.NewFakeRequest("test-user")

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()

			sar := allowedFakeSAR
			if tc.sarDenied {
				sar = denyFakeSAR
			}

			validator := NewValidator(
				fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
				fakeclients.PodCache(clientset.CoreV1().Pods),
				nil, nil,
				fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
				nil,
				sar,
			)

			err := validator.Create(fakeRequest, tc.vmi)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
