package storageclass

import (
	"fmt"
	"testing"

	lhcrypto "github.com/longhorn/longhorn-manager/csi/crypto"
	lhtypes "github.com/longhorn/longhorn-manager/types"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	corefake "k8s.io/client-go/kubernetes/fake"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvesterFake "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func Test_storageClassValidator_validateEncryption(t *testing.T) {

	normalSC := storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sc1",
		},
		Parameters: map[string]string{
			util.LonghornOptionEncrypted:          "true",
			util.CSIProvisionerSecretNameKey:      "test-secret",
			util.CSIProvisionerSecretNamespaceKey: "default",
			util.CSINodeStageSecretNameKey:        "test-secret",
			util.CSINodeStageSecretNamespaceKey:   "default",
			util.CSINodePublishSecretNameKey:      "test-secret",
			util.CSINodePublishSecretNamespaceKey: "default",
		},
	}
	emptySecretSc := storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sc1",
		},
		Parameters: map[string]string{
			util.LonghornOptionEncrypted:          "true",
			util.CSIProvisionerSecretNameKey:      "",
			util.CSIProvisionerSecretNamespaceKey: "",
			util.CSINodeStageSecretNameKey:        "",
			util.CSINodeStageSecretNamespaceKey:   "",
			util.CSINodePublishSecretNameKey:      "",
			util.CSINodePublishSecretNamespaceKey: "",
		},
	}

	tests := []struct {
		name         string
		secret       *corev1.Secret
		storageClass *storagev1.StorageClass
		expectError  bool
	}{
		{
			name: "valid encryption parameters",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					lhtypes.CryptoKeyHash:     []byte(lhcrypto.CryptoKeyDefaultHash),
					lhtypes.CryptoKeyCipher:   []byte(lhcrypto.CryptoKeyDefaultCipher),
					lhtypes.CryptoKeySize:     []byte(lhcrypto.CryptoKeyDefaultSize),
					lhtypes.CryptoPBKDF:       []byte(lhcrypto.CryptoDefaultPBKDF),
					lhtypes.CryptoKeyProvider: []byte("secret"),
					lhtypes.CryptoKeyValue:    []byte("test-value"),
				},
			},
			storageClass: &normalSC,
			expectError:  false,
		},
		{
			name: "empty secret name and namespace",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					lhtypes.CryptoKeyHash:     []byte(lhcrypto.CryptoKeyDefaultHash),
					lhtypes.CryptoKeyCipher:   []byte(lhcrypto.CryptoKeyDefaultCipher),
					lhtypes.CryptoKeySize:     []byte(lhcrypto.CryptoKeyDefaultSize),
					lhtypes.CryptoPBKDF:       []byte(lhcrypto.CryptoDefaultPBKDF),
					lhtypes.CryptoKeyProvider: []byte("secret"),
					lhtypes.CryptoKeyValue:    []byte("test-value"),
				},
			},
			storageClass: &emptySecretSc,
			expectError:  true,
		},
		{
			name: fmt.Sprintf("invalid secret: missing %s", lhtypes.CryptoKeyHash),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					lhtypes.CryptoKeyCipher:   []byte(lhcrypto.CryptoKeyDefaultCipher),
					lhtypes.CryptoKeySize:     []byte(lhcrypto.CryptoKeyDefaultSize),
					lhtypes.CryptoPBKDF:       []byte(lhcrypto.CryptoDefaultPBKDF),
					lhtypes.CryptoKeyProvider: []byte("secret"),
					lhtypes.CryptoKeyValue:    []byte("test-value"),
				},
			},
			storageClass: &normalSC,
			expectError:  true,
		},
		{
			name: fmt.Sprintf("invalid secret: missing %s", lhtypes.CryptoKeyCipher),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					lhtypes.CryptoKeyHash:     []byte(lhcrypto.CryptoKeyDefaultHash),
					lhtypes.CryptoKeySize:     []byte(lhcrypto.CryptoKeyDefaultSize),
					lhtypes.CryptoPBKDF:       []byte(lhcrypto.CryptoDefaultPBKDF),
					lhtypes.CryptoKeyProvider: []byte("secret"),
					lhtypes.CryptoKeyValue:    []byte("test-value"),
				},
			},
			storageClass: &normalSC,
			expectError:  true,
		},
		{
			name: fmt.Sprintf("invalid secret: missing %s", lhtypes.CryptoKeySize),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					lhtypes.CryptoKeyHash:     []byte(lhcrypto.CryptoKeyDefaultHash),
					lhtypes.CryptoKeyCipher:   []byte(lhcrypto.CryptoKeyDefaultCipher),
					lhtypes.CryptoPBKDF:       []byte(lhcrypto.CryptoDefaultPBKDF),
					lhtypes.CryptoKeyProvider: []byte("secret"),
					lhtypes.CryptoKeyValue:    []byte("test-value"),
				},
			},
			storageClass: &normalSC,
			expectError:  true,
		},
		{
			name: fmt.Sprintf("invalid secret: missing %s", lhtypes.CryptoPBKDF),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					lhtypes.CryptoKeyHash:     []byte(lhcrypto.CryptoKeyDefaultHash),
					lhtypes.CryptoKeyCipher:   []byte(lhcrypto.CryptoKeyDefaultCipher),
					lhtypes.CryptoKeySize:     []byte(lhcrypto.CryptoKeyDefaultSize),
					lhtypes.CryptoKeyProvider: []byte("secret"),
					lhtypes.CryptoKeyValue:    []byte("test-value"),
				},
			},
			storageClass: &normalSC,
			expectError:  true,
		},
		{
			name: fmt.Sprintf("invalid secret: missing %s", lhtypes.CryptoKeyProvider),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					lhtypes.CryptoKeyHash:   []byte(lhcrypto.CryptoKeyDefaultHash),
					lhtypes.CryptoKeyCipher: []byte(lhcrypto.CryptoKeyDefaultCipher),
					lhtypes.CryptoKeySize:   []byte(lhcrypto.CryptoKeyDefaultSize),
					lhtypes.CryptoPBKDF:     []byte(lhcrypto.CryptoDefaultPBKDF),
					lhtypes.CryptoKeyValue:  []byte("test-value"),
				},
			},
			storageClass: &normalSC,
			expectError:  true,
		},
		{
			name: fmt.Sprintf("invalid secret: missing %s", lhtypes.CryptoKeyValue),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					lhtypes.CryptoKeyHash:     []byte(lhcrypto.CryptoKeyDefaultHash),
					lhtypes.CryptoKeyCipher:   []byte(lhcrypto.CryptoKeyDefaultCipher),
					lhtypes.CryptoKeySize:     []byte(lhcrypto.CryptoKeyDefaultSize),
					lhtypes.CryptoPBKDF:       []byte(lhcrypto.CryptoDefaultPBKDF),
					lhtypes.CryptoKeyProvider: []byte("secret"),
				},
			},
			storageClass: &normalSC,
			expectError:  true,
		},
		{
			name: fmt.Sprintf("invalid secret: %s is empty", lhtypes.CryptoKeyValue),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					lhtypes.CryptoKeyHash:     []byte(lhcrypto.CryptoKeyDefaultHash),
					lhtypes.CryptoKeyCipher:   []byte(lhcrypto.CryptoKeyDefaultCipher),
					lhtypes.CryptoKeySize:     []byte(lhcrypto.CryptoKeyDefaultSize),
					lhtypes.CryptoPBKDF:       []byte(lhcrypto.CryptoDefaultPBKDF),
					lhtypes.CryptoKeyProvider: []byte("secret"),
					lhtypes.CryptoKeyValue:    []byte(""),
				},
			},
			storageClass: &normalSC,
			expectError:  true,
		},
		{
			name: fmt.Sprintf("invalid secret: %s is wrong", lhtypes.CryptoKeyHash),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					lhtypes.CryptoKeyHash:     []byte("test"),
					lhtypes.CryptoKeyCipher:   []byte(lhcrypto.CryptoKeyDefaultCipher),
					lhtypes.CryptoKeySize:     []byte(lhcrypto.CryptoKeyDefaultSize),
					lhtypes.CryptoPBKDF:       []byte(lhcrypto.CryptoDefaultPBKDF),
					lhtypes.CryptoKeyProvider: []byte("secret"),
					lhtypes.CryptoKeyValue:    []byte("test"),
				},
			},
			storageClass: &normalSC,
			expectError:  true,
		},
		{
			name: fmt.Sprintf("invalid secret: %s is wrong", lhtypes.CryptoKeyCipher),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					lhtypes.CryptoKeyHash:     []byte(lhcrypto.CryptoKeyDefaultHash),
					lhtypes.CryptoKeyCipher:   []byte("test"),
					lhtypes.CryptoKeySize:     []byte(lhcrypto.CryptoKeyDefaultSize),
					lhtypes.CryptoPBKDF:       []byte(lhcrypto.CryptoDefaultPBKDF),
					lhtypes.CryptoKeyProvider: []byte("secret"),
					lhtypes.CryptoKeyValue:    []byte("test-value"),
				},
			},
			storageClass: &normalSC,
			expectError:  true,
		},
		{
			name: fmt.Sprintf("invalid secret: %s is wrong", lhtypes.CryptoKeySize),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					lhtypes.CryptoKeyHash:     []byte(lhcrypto.CryptoKeyDefaultHash),
					lhtypes.CryptoKeyCipher:   []byte(lhcrypto.CryptoKeyDefaultCipher),
					lhtypes.CryptoKeySize:     []byte("test"),
					lhtypes.CryptoPBKDF:       []byte(lhcrypto.CryptoDefaultPBKDF),
					lhtypes.CryptoKeyProvider: []byte("secret"),
					lhtypes.CryptoKeyValue:    []byte("test-value"),
				},
			},
			storageClass: &normalSC,
			expectError:  true,
		},
		{
			name: fmt.Sprintf("invalid secret: %s is wrong", lhtypes.CryptoPBKDF),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					lhtypes.CryptoKeyHash:     []byte(lhcrypto.CryptoKeyDefaultHash),
					lhtypes.CryptoKeyCipher:   []byte(lhcrypto.CryptoKeyDefaultCipher),
					lhtypes.CryptoKeySize:     []byte(lhcrypto.CryptoKeyDefaultSize),
					lhtypes.CryptoPBKDF:       []byte("test"),
					lhtypes.CryptoKeyProvider: []byte("secret"),
					lhtypes.CryptoKeyValue:    []byte("test-value"),
				},
			},
			storageClass: &normalSC,
			expectError:  true,
		},
		{
			name: fmt.Sprintf("invalid secret: %s is wrong", lhtypes.CryptoKeyProvider),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					lhtypes.CryptoKeyHash:     []byte(lhcrypto.CryptoKeyDefaultHash),
					lhtypes.CryptoKeyCipher:   []byte(lhcrypto.CryptoKeyDefaultCipher),
					lhtypes.CryptoKeySize:     []byte(lhcrypto.CryptoKeyDefaultSize),
					lhtypes.CryptoPBKDF:       []byte(lhcrypto.CryptoDefaultPBKDF),
					lhtypes.CryptoKeyProvider: []byte("test"),
					lhtypes.CryptoKeyValue:    []byte("test-value"),
				},
			},
			storageClass: &normalSC,
			expectError:  true,
		},
		{
			name: "secret not found",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc3",
				},
				Parameters: map[string]string{
					util.LonghornOptionEncrypted:          "true",
					util.CSIProvisionerSecretNameKey:      "non-existent-secret",
					util.CSIProvisionerSecretNamespaceKey: "default",
					util.CSINodeStageSecretNameKey:        "non-existent-secret",
					util.CSINodeStageSecretNamespaceKey:   "default",
					util.CSINodePublishSecretNameKey:      "non-existent-secret",
					util.CSINodePublishSecretNamespaceKey: "default",
				},
			},
			expectError: true,
		},
		{
			name: "encryption disabled",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc4",
				},
				Parameters: map[string]string{
					util.LonghornOptionEncrypted: "false",
				},
			},
			expectError: false,
		},
		{
			name: "invalid encryption value",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc2",
				},
				Parameters: map[string]string{
					util.LonghornOptionEncrypted: "invalid-value-here",
				},
			},
			expectError: true,
		},
		{
			name: "missing parameters for encryption",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc5",
				},
				Parameters: map[string]string{
					util.LonghornOptionEncrypted:     "true",
					util.CSIProvisionerSecretNameKey: "non-existent-secret",
				},
			},
			expectError: true,
		},
	}

	coreclientset := corefake.NewSimpleClientset()
	harvesterClientSet := harvesterFake.NewSimpleClientset()
	fakeVMIMageCache := fakeclients.VirtualMachineImageCache(harvesterClientSet.HarvesterhciV1beta1().VirtualMachineImages)
	fakeSecretCache := fakeclients.SecretCache(coreclientset.CoreV1().Secrets)
	fakeStorageClassCache := fakeclients.StorageClassCache(coreclientset.StorageV1().StorageClasses)
	validator := NewValidator(fakeStorageClassCache, fakeSecretCache, fakeVMIMageCache).(*storageClassValidator)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.secret != nil {
				coreclientset.Tracker().Add(tc.secret)
			}

			err := validator.validateEncryption(tc.storageClass)
			if tc.expectError {
				assert.NotNil(t, err, tc.name)
			} else {
				assert.Nil(t, err, tc.name)
			}

			if tc.secret != nil {
				coreclientset.Tracker().Delete(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, tc.secret.Namespace, tc.secret.Name)
			}
		})
	}
}

func Test_storageClassValidator_Delete(t *testing.T) {
	tests := []struct {
		name         string
		storageClass *storagev1.StorageClass
		expectError  bool
	}{
		{
			name: "storage class in use by VM image",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-storage-class",
				},
			},
			expectError: true,
		},
		{
			name: "storage class any with AnnotationIsReservedStorageClass true can't be deleted",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "any",
					Annotations: map[string]string{
						util.AnnotationIsReservedStorageClass: "true",
					},
				},
			},
			expectError: true,
		},
		{
			name: "storage class any with AnnotationIsReservedStorageClass false can be deleted",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "any",
					Annotations: map[string]string{
						util.AnnotationIsReservedStorageClass: "false",
					},
				},
			},
			expectError: false,
		},
		{
			name: "storage class harvester-longhorn with AnnotationIsReservedStorageClass false can be deleted too",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: util.StorageClassHarvesterLonghorn,
					Annotations: map[string]string{
						util.AnnotationIsReservedStorageClass: "false",
					},
				},
			},
			expectError: false,
		},
		{
			name: "storage class harvester-longhorn without AnnotationIsReservedStorageClass can't be deleted",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: util.StorageClassHarvesterLonghorn,
					Annotations: map[string]string{
						util.HelmReleaseNameAnnotation:      util.HarvesterChartReleaseName,
						util.HelmReleaseNamespaceAnnotation: util.HarvesterSystemNamespaceName,
					},
				},
			},
			expectError: true,
		},
		{
			name: "storage class others without AnnotationIsReservedStorageClass can be deleted",
			storageClass: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "others",
				},
			},
			expectError: false,
		},
	}

	harvesterClientSet := harvesterFake.NewSimpleClientset(&v1beta1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vmimage",
			Namespace: "default",
			Annotations: map[string]string{
				util.AnnotationStorageClassName: "test-storage-class",
			},
		},
	})

	fakeVMIMageCache := fakeclients.VirtualMachineImageCache(harvesterClientSet.HarvesterhciV1beta1().VirtualMachineImages)
	fakeSecretCache := fakeclients.SecretCache(corefake.NewSimpleClientset().CoreV1().Secrets)
	fakeStorageClassCache := fakeclients.StorageClassCache(corefake.NewSimpleClientset().StorageV1().StorageClasses)
	validator := NewValidator(fakeStorageClassCache, fakeSecretCache, fakeVMIMageCache).(*storageClassValidator)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Delete(nil, tc.storageClass)
			if tc.expectError {
				assert.NotNil(t, err, tc.name)
			} else {
				assert.Nil(t, err, tc.name)
			}
		})
	}
}
