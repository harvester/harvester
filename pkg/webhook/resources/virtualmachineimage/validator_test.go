package virtualmachineimage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestVirtualMachineImageValidator_Create(t *testing.T) {
	tests := []struct {
		name          string
		vmi           *harvesterv1.VirtualMachineImage
		sarDenied     bool
		expectedError string
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
			sarDenied:     true,
			expectedError: `user "test-user" is not authorized to access source image default/source-image`,
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
			expectedError: "source image default/source-image not found",
		},
		{
			name: "CDI: download image with valid URL - pass",
			vmi: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cdi-img", Namespace: "default"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend:     harvesterv1.VMIBackendCDI,
					DisplayName: "test-cdi-image",
					SourceType:  harvesterv1.VirtualMachineImageSourceTypeDownload,
					URL:         "http://example.com/image.iso",
				},
			},
		},
		{
			name: "CDI: clone image, SAR denied for source image - fail",
			vmi: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cdi-clone", Namespace: "default"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend:     harvesterv1.VMIBackendCDI,
					DisplayName: "test-cdi-clone-image",
					SourceType:  harvesterv1.VirtualMachineImageSourceTypeClone,
					SecurityParameters: &harvesterv1.VirtualMachineImageSecurityParameters{
						CryptoOperation:      harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
						SourceImageName:      "source-image",
						SourceImageNamespace: "default",
					},
				},
			},
			sarDenied:     true,
			expectedError: `user "test-user" is not authorized to access source image default/source-image`,
		},
		{
			// SAR passes but source image doesn't exist — confirms the check proceeds past SAR
			name: "CDI: clone image, SAR allowed, source not found - fail",
			vmi: &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cdi-clone-2", Namespace: "default"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend:     harvesterv1.VMIBackendCDI,
					DisplayName: "test-cdi-clone-image-2",
					SourceType:  harvesterv1.VirtualMachineImageSourceTypeClone,
					SecurityParameters: &harvesterv1.VirtualMachineImageSecurityParameters{
						CryptoOperation:      harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
						SourceImageName:      "source-image",
						SourceImageNamespace: "default",
					},
				},
			},
			expectedError: "source image default/source-image not found",
		},
	}

	fakeRequest := fakeclients.NewFakeRequest("test-user")

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()

			sar := fakeclients.SARClientSet(!tc.sarDenied)

			validator := NewValidator(
				fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
				fakeclients.PodCache(clientset.CoreV1().Pods),
				nil, nil,
				fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
				nil,
				sar.AuthorizationV1().SubjectAccessReviews(),
			)

			err := validator.Create(fakeRequest, tc.vmi)
			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
			}
			if tc.vmi.Spec.SourceType == harvesterv1.VirtualMachineImageSourceTypeClone {
				assert.Len(t, sar.Actions(), 1)
			} else {
				assert.Empty(t, sar.Actions())
			}
		})
	}
}

func TestVirtualMachineImageValidator_CreateClone(t *testing.T) {
	for _, backend := range []harvesterv1.VMIBackend{harvesterv1.VMIBackendBackingImage, harvesterv1.VMIBackendCDI} {
		t.Run(string(backend), func(t *testing.T) {
			for _, operation := range []harvesterv1.VirtualMachineImageCryptoOperationType{
				harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
				harvesterv1.VirtualMachineImageCryptoOperationTypeDecrypt,
			} {
				t.Run(string(operation), func(t *testing.T) {
					sourceEncrypted, targetEncrypted := "false", "true"
					if operation == harvesterv1.VirtualMachineImageCryptoOperationTypeDecrypt {
						sourceEncrypted, targetEncrypted = "true", "false"
					}
					sourceSC := &storagev1.StorageClass{
						ObjectMeta:  metav1.ObjectMeta{Name: "source-sc"},
						Provisioner: util.CSIProvisionerLonghorn,
						Parameters:  map[string]string{util.LonghornOptionEncrypted: sourceEncrypted},
					}
					targetSC := &storagev1.StorageClass{
						ObjectMeta:  metav1.ObjectMeta{Name: "target-sc"},
						Provisioner: util.CSIProvisionerLonghorn,
						Parameters:  map[string]string{util.LonghornOptionEncrypted: targetEncrypted},
					}
					source := &harvesterv1.VirtualMachineImage{
						ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "default"},
						Spec: harvesterv1.VirtualMachineImageSpec{
							Backend: backend, SourceType: harvesterv1.VirtualMachineImageSourceTypeDownload,
						},
						Status: harvesterv1.VirtualMachineImageStatus{StorageClassName: sourceSC.Name},
					}
					if backend == harvesterv1.VMIBackendBackingImage && operation == harvesterv1.VirtualMachineImageCryptoOperationTypeDecrypt {
						source.Spec.SourceType = harvesterv1.VirtualMachineImageSourceTypeClone
						source.Spec.SecurityParameters = &harvesterv1.VirtualMachineImageSecurityParameters{
							CryptoOperation: harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
							SourceImageName: "original", SourceImageNamespace: source.Namespace,
						}
					}
					harvesterv1.ImageImported.True(source)
					target := &harvesterv1.VirtualMachineImage{
						ObjectMeta: metav1.ObjectMeta{
							Name: "target", Namespace: "default",
							Annotations: map[string]string{util.AnnotationStorageClassName: targetSC.Name},
						},
						Spec: harvesterv1.VirtualMachineImageSpec{
							Backend: backend, DisplayName: "clone-image",
							SourceType: harvesterv1.VirtualMachineImageSourceTypeClone,
							SecurityParameters: &harvesterv1.VirtualMachineImageSecurityParameters{
								CryptoOperation: operation, SourceImageName: source.Name, SourceImageNamespace: source.Namespace,
							},
						},
					}
					clientset := fake.NewSimpleClientset(source, sourceSC, targetSC)
					sar := fakeclients.SARClientSet(true)
					validator := NewValidator(
						fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
						fakeclients.PodCache(clientset.CoreV1().Pods), nil, nil,
						fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses), nil,
						sar.AuthorizationV1().SubjectAccessReviews(),
					)
					assert.NoError(t, validator.Create(fakeclients.NewFakeRequest("test-user"), target))
					assert.Len(t, sar.Actions(), 1, "clone security validation should check source access once")
				})
			}
		})
	}
}

func TestImageSecurityParametersAreImmutable(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	validator := NewValidator(
		fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
		fakeclients.PodCache(clientset.CoreV1().Pods), nil, nil,
		fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses), nil,
		fakeclients.AllowedSARClient(),
	)
	for _, backend := range []harvesterv1.VMIBackend{
		harvesterv1.VMIBackendCDI,
		harvesterv1.VMIBackendBackingImage,
	} {
		t.Run(string(backend), func(t *testing.T) {
			oldImage := &harvesterv1.VirtualMachineImage{Spec: harvesterv1.VirtualMachineImageSpec{
				Backend: backend, SourceType: harvesterv1.VirtualMachineImageSourceTypeClone,
				SecurityParameters: &harvesterv1.VirtualMachineImageSecurityParameters{
					CryptoOperation: harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
					SourceImageName: "source", SourceImageNamespace: "default",
				},
			}}

			t.Run("reject changing crypto operation", func(t *testing.T) {
				updated := oldImage.DeepCopy()
				updated.Spec.SecurityParameters.CryptoOperation = harvesterv1.VirtualMachineImageCryptoOperationTypeDecrypt
				assert.EqualError(t, validator.Update(nil, oldImage, updated), "securityParameters cannot be modified")
			})

			t.Run("reject changing source image name", func(t *testing.T) {
				updated := oldImage.DeepCopy()
				updated.Spec.SecurityParameters.SourceImageName = "another-source"
				assert.EqualError(t, validator.Update(nil, oldImage, updated), "securityParameters cannot be modified")
			})

			t.Run("reject changing source image namespace", func(t *testing.T) {
				updated := oldImage.DeepCopy()
				updated.Spec.SecurityParameters.SourceImageNamespace = "another-namespace"
				assert.EqualError(t, validator.Update(nil, oldImage, updated), "securityParameters cannot be modified")
			})

			t.Run("reject removing security parameters", func(t *testing.T) {
				updated := oldImage.DeepCopy()
				updated.Spec.SecurityParameters = nil
				assert.EqualError(t, validator.Update(nil, oldImage, updated), "securityParameters cannot be modified")
			})
		})
	}
}
