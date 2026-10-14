package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/fakeclients"
)

func TestCheckSecurityParametersStorageClassCompatibility(t *testing.T) {
	const (
		encrypt                = harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt
		decrypt                = harvesterv1.VirtualMachineImageCryptoOperationTypeDecrypt
		download               = harvesterv1.VirtualMachineImageSourceTypeDownload
		upload                 = harvesterv1.VirtualMachineImageSourceTypeUpload
		clone                  = harvesterv1.VirtualMachineImageSourceTypeClone
		cdi                    = harvesterv1.VMIBackendCDI
		longhornError          = "image encryption and decryption are currently supported only with Longhorn"
		sameEngineError        = "source and target image StorageClasses must use the same Longhorn data engine"
		encryptedSourceError   = "can not re-encrypt, source image default/source () is already encrypted"
		unencryptedSourceError = "can not re-decrypt, source image default/source () is not encrypted"
		unencryptedTargetError = "CDI image decryption requires an unencrypted target StorageClass"
	)
	longhornV1 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "longhorn-v1"}, Provisioner: util.CSIProvisionerLonghorn,
		Parameters: map[string]string{util.ParamDataEngine: "v1"},
	}
	encryptedLonghornV1 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "encrypted-longhorn-v1"}, Provisioner: util.CSIProvisionerLonghorn,
		Parameters: map[string]string{util.ParamDataEngine: "v1", util.LonghornOptionEncrypted: "true"},
	}
	longhornV2 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "longhorn-v2"}, Provisioner: util.CSIProvisionerLonghorn,
		Parameters: map[string]string{util.ParamDataEngine: "v2"},
	}
	encryptedLonghornV2 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "encrypted-longhorn-v2"}, Provisioner: util.CSIProvisionerLonghorn,
		Parameters: map[string]string{util.ParamDataEngine: "v2", util.LonghornOptionEncrypted: "true"},
	}
	unencryptedLonghornV2 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "unencrypted-longhorn-v2"}, Provisioner: util.CSIProvisionerLonghorn,
		Parameters: map[string]string{util.ParamDataEngine: "v2", util.LonghornOptionEncrypted: "false"},
	}
	defaultLonghornV1 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "default-longhorn-v1"}, Provisioner: util.CSIProvisionerLonghorn,
	}
	encryptedDefaultLonghornV1 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "encrypted-default-longhorn-v1"}, Provisioner: util.CSIProvisionerLonghorn,
		Parameters: map[string]string{util.LonghornOptionEncrypted: "true"},
	}
	lvm := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "lvm"}, Provisioner: "lvm.driver.harvesterhci.io",
	}

	tests := []struct {
		name            string
		operation       harvesterv1.VirtualMachineImageCryptoOperationType
		sourceBackend   harvesterv1.VMIBackend
		targetBackend   harvesterv1.VMIBackend
		sourceType      harvesterv1.VirtualMachineImageSourceType
		sourceOperation harvesterv1.VirtualMachineImageCryptoOperationType
		sourceSC        *storagev1.StorageClass
		targetSC        *storagev1.StorageClass
		expectedError   string
	}{
		{name: "allow encryption from Longhorn V1 to encrypted Longhorn V1", operation: encrypt, sourceSC: longhornV1, targetSC: encryptedLonghornV1},
		{name: "allow decryption from encrypted Longhorn V1 to Longhorn V1", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceSC: encryptedLonghornV1, targetSC: longhornV1},
		{name: "allow encryption from Longhorn V2 to encrypted Longhorn V2", operation: encrypt, sourceBackend: cdi, targetBackend: cdi, sourceSC: longhornV2, targetSC: encryptedLonghornV2},
		{name: "allow CDI decryption from encrypted source to unencrypted target", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceBackend: cdi, targetBackend: cdi, sourceSC: encryptedLonghornV2, targetSC: longhornV2},
		{name: "reject CDI decryption from encrypted source to encrypted target", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceBackend: cdi, targetBackend: cdi, sourceSC: encryptedLonghornV2, targetSC: encryptedLonghornV2, expectedError: unencryptedTargetError},
		{name: "reject CDI decryption from unencrypted source to unencrypted target", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceBackend: cdi, targetBackend: cdi, sourceSC: longhornV2, targetSC: longhornV2, expectedError: unencryptedSourceError},
		{name: "reject CDI decryption from unencrypted source to encrypted target", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceBackend: cdi, targetBackend: cdi, sourceSC: longhornV2, targetSC: encryptedLonghornV2, expectedError: unencryptedSourceError},
		{name: "allow CDI decryption of encrypted download without clone history", operation: decrypt, sourceBackend: cdi, targetBackend: cdi, sourceType: download, sourceSC: encryptedLonghornV2, targetSC: longhornV2},
		{name: "allow CDI decryption of encrypted upload without clone history", operation: decrypt, sourceBackend: cdi, targetBackend: cdi, sourceType: upload, sourceSC: encryptedLonghornV2, targetSC: longhornV2},
		{name: "reject CDI re-encryption of encrypted download", operation: encrypt, sourceBackend: cdi, targetBackend: cdi, sourceType: download, sourceSC: encryptedLonghornV2, targetSC: encryptedLonghornV2, expectedError: encryptedSourceError},
		{name: "reject CDI decryption of unencrypted download", operation: decrypt, sourceBackend: cdi, targetBackend: cdi, sourceType: download, sourceSC: unencryptedLonghornV2, targetSC: longhornV2, expectedError: unencryptedSourceError},
		{name: "reject CDI decryption of upload when source encryption parameter is omitted", operation: decrypt, sourceBackend: cdi, targetBackend: cdi, sourceType: upload, sourceSC: longhornV2, targetSC: longhornV2, expectedError: unencryptedSourceError},
		{name: "allow CDI encryption of unencrypted upload", operation: encrypt, sourceBackend: cdi, targetBackend: cdi, sourceType: upload, sourceSC: unencryptedLonghornV2, targetSC: encryptedLonghornV2},
		{name: "allow CDI decryption of encrypted storage despite decrypt history", operation: decrypt, sourceBackend: cdi, targetBackend: cdi, sourceType: clone, sourceOperation: decrypt, sourceSC: encryptedLonghornV2, targetSC: longhornV2},
		{name: "allow CDI encryption of unencrypted storage despite encrypt history", operation: encrypt, sourceBackend: cdi, targetBackend: cdi, sourceType: clone, sourceOperation: encrypt, sourceSC: unencryptedLonghornV2, targetSC: encryptedLonghornV2},
		{name: "allow backing-image decryption based on clone history despite unencrypted StorageClass", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceSC: longhornV1, targetSC: longhornV1},
		{name: "reject backing-image decryption of encrypted download without clone history", operation: decrypt, sourceType: download, sourceSC: encryptedLonghornV1, targetSC: longhornV1, expectedError: unencryptedSourceError},
		{name: "reject backing-image re-encryption based on clone history despite unencrypted StorageClass", operation: encrypt, sourceType: clone, sourceOperation: encrypt, sourceSC: longhornV1, targetSC: encryptedLonghornV1, expectedError: encryptedSourceError},
		{name: "allow encryption when both StorageClasses omit dataEngine (Longhorn V1)", operation: encrypt, sourceSC: defaultLonghornV1, targetSC: encryptedDefaultLonghornV1},
		{name: "allow decryption when both StorageClasses omit dataEngine (Longhorn V1)", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceSC: encryptedDefaultLonghornV1, targetSC: defaultLonghornV1},
		{name: "allow encryption from default Longhorn V1 to explicit Longhorn V1", operation: encrypt, sourceSC: defaultLonghornV1, targetSC: encryptedLonghornV1},
		{name: "allow decryption from default Longhorn V1 to explicit Longhorn V1", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceSC: encryptedDefaultLonghornV1, targetSC: longhornV1},
		{name: "allow encryption from explicit Longhorn V1 to default Longhorn V1", operation: encrypt, sourceSC: longhornV1, targetSC: encryptedDefaultLonghornV1},
		{name: "allow decryption from explicit Longhorn V1 to default Longhorn V1", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceSC: encryptedLonghornV1, targetSC: defaultLonghornV1},
		{name: "reject encryption from Longhorn V1 to Longhorn V2", operation: encrypt, sourceSC: longhornV1, targetSC: encryptedLonghornV2, expectedError: sameEngineError},
		{name: "reject decryption from Longhorn V1 to Longhorn V2", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceSC: encryptedLonghornV1, targetSC: longhornV2, expectedError: sameEngineError},
		{name: "reject encryption from Longhorn V2 to Longhorn V1", operation: encrypt, sourceSC: longhornV2, targetSC: encryptedLonghornV1, expectedError: sameEngineError},
		{name: "reject decryption from Longhorn V2 to Longhorn V1", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceSC: encryptedLonghornV2, targetSC: longhornV1, expectedError: sameEngineError},
		{name: "reject encryption from default Longhorn V1 to Longhorn V2", operation: encrypt, sourceSC: defaultLonghornV1, targetSC: encryptedLonghornV2, expectedError: sameEngineError},
		{name: "reject decryption from default Longhorn V1 to Longhorn V2", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceSC: encryptedDefaultLonghornV1, targetSC: longhornV2, expectedError: sameEngineError},
		{name: "reject encryption from Longhorn V2 to default Longhorn V1", operation: encrypt, sourceSC: longhornV2, targetSC: encryptedDefaultLonghornV1, expectedError: sameEngineError},
		{name: "reject decryption from Longhorn V2 to default Longhorn V1", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceSC: encryptedLonghornV2, targetSC: defaultLonghornV1, expectedError: sameEngineError},
		{name: "reject encryption from an LVM source StorageClass", operation: encrypt, sourceSC: lvm, targetSC: encryptedLonghornV1, expectedError: longhornError},
		{name: "reject decryption from an LVM source StorageClass", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceSC: lvm, targetSC: longhornV1, expectedError: longhornError},
		{name: "reject encryption into an LVM target StorageClass", operation: encrypt, sourceSC: longhornV1, targetSC: lvm, expectedError: longhornError},
		{name: "reject decryption into an LVM target StorageClass", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceSC: encryptedLonghornV1, targetSC: lvm, expectedError: longhornError},
		{name: "reject encryption when both StorageClasses use LVM", operation: encrypt, sourceSC: lvm, targetSC: lvm, expectedError: longhornError},
		{name: "reject decryption when both StorageClasses use LVM", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceSC: lvm, targetSC: lvm, expectedError: longhornError},
		{name: "reject encryption when the source StorageClass is missing", operation: encrypt, targetSC: encryptedLonghornV1, expectedError: `failed to get source image StorageClass: storageclasses.storage.k8s.io "source-sc" not found`},
		{name: "reject decryption when the source StorageClass is missing", operation: decrypt, sourceType: clone, sourceOperation: encrypt, targetSC: longhornV1, expectedError: `failed to get source image StorageClass: storageclasses.storage.k8s.io "source-sc" not found`},
		{name: "reject encryption when the target StorageClass is missing", operation: encrypt, sourceSC: longhornV1, expectedError: "storage class target-sc not found"},
		{name: "reject decryption when the target StorageClass is missing", operation: decrypt, sourceType: clone, sourceOperation: encrypt, sourceSC: encryptedLonghornV1, expectedError: "storage class target-sc not found"},
		{name: "reject CDI target with backing-image source", operation: encrypt, sourceBackend: harvesterv1.VMIBackendBackingImage, targetBackend: harvesterv1.VMIBackendCDI, sourceSC: longhornV1, targetSC: encryptedLonghornV1, expectedError: "source and target image backends must match"},
		{name: "reject backing-image target with CDI source", operation: encrypt, sourceBackend: harvesterv1.VMIBackendCDI, targetBackend: harvesterv1.VMIBackendBackingImage, sourceSC: longhornV1, targetSC: encryptedLonghornV1, expectedError: "source and target image backends must match"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sourceBackend, targetBackend := tc.sourceBackend, tc.targetBackend
			if sourceBackend == "" {
				sourceBackend = harvesterv1.VMIBackendBackingImage
			}
			if targetBackend == "" {
				targetBackend = harvesterv1.VMIBackendBackingImage
			}
			sourceSCName, targetSCName := "source-sc", "target-sc"
			if tc.sourceSC != nil {
				sourceSCName = tc.sourceSC.Name
			}
			if tc.targetSC != nil {
				targetSCName = tc.targetSC.Name
			}
			source := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "default"},
				// The selected template differs from the image's actual StorageClass.
				Spec:   harvesterv1.VirtualMachineImageSpec{Backend: sourceBackend, SourceType: tc.sourceType, TargetStorageClassName: "source-template"},
				Status: harvesterv1.VirtualMachineImageStatus{StorageClassName: sourceSCName},
			}
			harvesterv1.ImageImported.True(source)
			if tc.sourceOperation != "" {
				source.Spec.SecurityParameters = &harvesterv1.VirtualMachineImageSecurityParameters{
					CryptoOperation: tc.sourceOperation,
				}
			}
			target := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "target", Namespace: "default",
					Annotations: map[string]string{util.AnnotationStorageClassName: targetSCName},
				},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend:                targetBackend,
					SourceType:             harvesterv1.VirtualMachineImageSourceTypeClone,
					TargetStorageClassName: targetSCName,
					SecurityParameters: &harvesterv1.VirtualMachineImageSecurityParameters{
						CryptoOperation: tc.operation, SourceImageName: source.Name, SourceImageNamespace: source.Namespace,
					},
				},
			}
			clientset := fake.NewSimpleClientset(source)
			if tc.sourceSC != nil {
				assert.NoError(t, clientset.Tracker().Add(tc.sourceSC))
			}
			if tc.targetSC != nil && (tc.sourceSC == nil || tc.targetSC.Name != tc.sourceSC.Name) {
				assert.NoError(t, clientset.Tracker().Add(tc.targetSC))
			}
			validator := &vmiValidator{
				vmiCache: fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
				scCache:  fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
				sar:      fakeclients.AllowedSARClient(),
			}
			err := validator.CheckSecurityParameters(fakeclients.NewFakeRequest("test-user"), target)
			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.expectedError)
			}
		})
	}
}

func TestCheckDisplayName(t *testing.T) {
	maxLenDisplayName := strings.Repeat("a", 63)
	tooLongDisplayName := strings.Repeat("a", 64)

	testCases := []struct {
		name           string
		displayName    string
		existingImages []*harvesterv1.VirtualMachineImage
		expectErr      bool
		errContains    string
	}{
		{
			name:        "rejects empty displayName",
			displayName: "",
			expectErr:   true,
			errContains: "displayName is required",
		},
		{
			name:        "accepts displayName with 63 chars",
			displayName: maxLenDisplayName,
			expectErr:   false,
		},
		{
			name:        "rejects displayName with more than 63 chars",
			displayName: tooLongDisplayName,
			expectErr:   true,
			errContains: "must be no more than 63 characters",
		},
		{
			name:        "rejects displayName with invalid Kubernetes label value",
			displayName: "Invalid/Name",
			expectErr:   true,
			errContains: "displayName is not a valid Kubernetes label value",
		},
		{
			name:        "rejects duplicate displayName",
			displayName: "duplicate-name",
			existingImages: []*harvesterv1.VirtualMachineImage{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-image",
						Namespace: "default",
						UID:       "existing-uid",
						Labels: map[string]string{
							util.LabelImageDisplayName: "duplicate-name",
						},
					},
				},
			},
			expectErr:   true,
			errContains: "A resource with the same name exists",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientSet := fake.NewSimpleClientset()
			for _, image := range tc.existingImages {
				err := clientSet.Tracker().Add(image)
				assert.NoError(t, err)
			}

			validator := &vmiValidator{
				vmiCache: fakeclients.VirtualMachineImageCache(clientSet.HarvesterhciV1beta1().VirtualMachineImages),
			}

			vmi := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-image",
					Namespace: "default",
				},
				Spec: harvesterv1.VirtualMachineImageSpec{
					DisplayName: tc.displayName,
				},
			}

			err := validator.CheckDisplayName(vmi)
			if tc.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckUpdateDisplayName(t *testing.T) {
	testCases := []struct {
		name           string
		oldDisplayName string
		newDisplayName string
		expectErr      bool
		errContains    string
	}{
		{
			name:           "accepts unchanged valid displayName",
			oldDisplayName: "valid-name",
			newDisplayName: "valid-name",
			expectErr:      false,
		},
		{
			name:           "rejects changed displayName",
			oldDisplayName: "old-name",
			newDisplayName: "new-name",
			expectErr:      true,
			errContains:    "displayName cannot be modified",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientSet := fake.NewSimpleClientset()
			validator := &vmiValidator{
				vmiCache: fakeclients.VirtualMachineImageCache(clientSet.HarvesterhciV1beta1().VirtualMachineImages),
			}

			oldVMI := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-image",
					Namespace: "default",
				},
				Spec: harvesterv1.VirtualMachineImageSpec{
					DisplayName: tc.oldDisplayName,
				},
			}

			newVMI := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-image",
					Namespace: "default",
				},
				Spec: harvesterv1.VirtualMachineImageSpec{
					DisplayName: tc.newDisplayName,
				},
			}

			err := validator.CheckUpdateDisplayName(oldVMI, newVMI)
			if tc.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
