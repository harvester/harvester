package virtualmachineimage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corefake "k8s.io/client-go/kubernetes/fake"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvesterFake "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_virtualMachineImageValidator_CheckImageSecurityParameters(t *testing.T) {
	tests := []struct {
		name        string
		newImage    *v1beta1.VirtualMachineImage
		expectError bool
	}{
		{
			name: "encrypt image",
			newImage: &v1beta1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						util.AnnotationStorageClassName: "test-storage-class",
					},
				},
				Spec: v1beta1.VirtualMachineImageSpec{
					SourceType: v1beta1.VirtualMachineImageSourceTypeClone,
					SecurityParameters: &v1beta1.VirtualMachineImageSecurityParameters{
						CryptoOperation:      "encrypt",
						SourceImageName:      "source-image",
						SourceImageNamespace: "default",
					},
				},
			},
			expectError: false,
		},
		{
			name: "encrypt encrypted image",
			newImage: &v1beta1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						util.AnnotationStorageClassName: "test-storage-class",
					},
				},
				Spec: v1beta1.VirtualMachineImageSpec{
					SourceType: v1beta1.VirtualMachineImageSourceTypeClone,
					SecurityParameters: &v1beta1.VirtualMachineImageSecurityParameters{
						CryptoOperation:      "encrypt",
						SourceImageName:      "encrypted-image",
						SourceImageNamespace: "default",
					},
				},
			},
			expectError: true,
		},
		{
			name: "decrypt image",
			newImage: &v1beta1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						util.AnnotationStorageClassName: "test-storage-class",
					},
				},
				Spec: v1beta1.VirtualMachineImageSpec{
					SourceType: v1beta1.VirtualMachineImageSourceTypeClone,
					SecurityParameters: &v1beta1.VirtualMachineImageSecurityParameters{
						CryptoOperation:      "decrypt",
						SourceImageName:      "encrypted-image",
						SourceImageNamespace: "default",
					},
				},
			},
			expectError: false,
		},
		{
			name: "decrypt non-encrypted image",
			newImage: &v1beta1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						util.AnnotationStorageClassName: "test-storage-class",
					},
				},
				Spec: v1beta1.VirtualMachineImageSpec{
					SourceType: v1beta1.VirtualMachineImageSourceTypeClone,
					SecurityParameters: &v1beta1.VirtualMachineImageSecurityParameters{
						CryptoOperation:      "decrypt",
						SourceImageName:      "source-image",
						SourceImageNamespace: "default",
					},
				},
			},
			expectError: true,
		},
		{
			name: "encryption with missed annotation",
			newImage: &v1beta1.VirtualMachineImage{
				Spec: v1beta1.VirtualMachineImageSpec{
					SourceType: v1beta1.VirtualMachineImageSourceTypeClone,
					SecurityParameters: &v1beta1.VirtualMachineImageSecurityParameters{
						CryptoOperation:      "encrypt",
						SourceImageName:      "source-image",
						SourceImageNamespace: "default",
					},
				},
			},
			expectError: true,
		},
		{
			name: "encryption with storage class not found",
			newImage: &v1beta1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						util.AnnotationStorageClassName: "non-existent-storage-class",
					},
				},
				Spec: v1beta1.VirtualMachineImageSpec{
					SourceType: v1beta1.VirtualMachineImageSourceTypeClone,
					SecurityParameters: &v1beta1.VirtualMachineImageSecurityParameters{
						CryptoOperation:      "encrypt",
						SourceImageName:      "source-image",
						SourceImageNamespace: "default",
					},
				},
			},
			expectError: true,
		},
	}

	coreclientset := corefake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-image",
				Namespace: "default",
			},
		},
		&storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-storage-class",
			},
			Parameters: map[string]string{
				util.LonghornOptionEncrypted: "true",
				// ignore other parameters
				// we just focus on if this storage class is found or not
				// storage class validator help us to check the other parameters
			},
		},
	)

	harvesterClientSet := harvesterFake.NewSimpleClientset(&v1beta1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "source-image",
			Namespace: "default",
		},
		Status: v1beta1.VirtualMachineImageStatus{
			Conditions: []v1beta1.Condition{
				{
					Type:   v1beta1.ImageImported,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}, &v1beta1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "encrypted-image",
			Namespace: "default",
			Annotations: map[string]string{
				util.AnnotationStorageClassName: "test-storage-class",
			},
		},
		Spec: v1beta1.VirtualMachineImageSpec{
			SourceType: v1beta1.VirtualMachineImageSourceTypeClone,
			SecurityParameters: &v1beta1.VirtualMachineImageSecurityParameters{
				CryptoOperation:      "encrypt",
				SourceImageName:      "source-image",
				SourceImageNamespace: "default",
			},
		},
		Status: v1beta1.VirtualMachineImageStatus{
			Conditions: []v1beta1.Condition{
				{
					Type:   v1beta1.ImageImported,
					Status: corev1.ConditionTrue,
				},
			},
		},
	})

	fakeVMIMageCache := fakeclients.VirtualMachineImageCache(harvesterClientSet.HarvesterhciV1beta1().VirtualMachineImages)
	fakeSecretCache := fakeclients.SecretCache(coreclientset.CoreV1().Secrets)
	fakeStorageClassCache := fakeclients.StorageClassCache(coreclientset.StorageV1().StorageClasses)
	fakeVMBackupCache := fakeclients.VMBackupCache(harvesterClientSet.HarvesterhciV1beta1().VirtualMachineBackups)
	validator := NewValidator(fakeVMIMageCache, nil, nil, nil, fakeSecretCache, fakeStorageClassCache, fakeVMBackupCache).(*virtualMachineImageValidator)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.checkImageSecurityParameters(tc.newImage)
			if tc.expectError {
				assert.NotNil(t, err, tc.name)
			} else {
				assert.Nil(t, err, tc.name)
			}
		})
	}
}
