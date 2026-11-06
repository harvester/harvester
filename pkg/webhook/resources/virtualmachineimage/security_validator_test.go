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
	werror "github.com/harvester/harvester/pkg/webhook/error"
)

// The webhook Create tests cover successful encryption and decryption for both
// backends. These cases check the clone gate and propagation of helper errors.
func TestSecurityValidator_Validate(t *testing.T) {
	for _, sourceType := range []harvesterv1.VirtualMachineImageSourceType{
		harvesterv1.VirtualMachineImageSourceTypeDownload,
		harvesterv1.VirtualMachineImageSourceTypeUpload,
	} {
		t.Run("ignore "+string(sourceType), func(t *testing.T) {
			validator := &securityValidator{}
			image := &harvesterv1.VirtualMachineImage{Spec: harvesterv1.VirtualMachineImageSpec{SourceType: sourceType}}
			assert.NoError(t, validator.Validate(nil, image))
		})
	}
	t.Run("require clone security parameters", func(t *testing.T) {
		validator := &securityValidator{}
		image := &harvesterv1.VirtualMachineImage{Spec: harvesterv1.VirtualMachineImageSpec{
			SourceType: harvesterv1.VirtualMachineImageSourceTypeClone,
		}}
		assert.EqualError(t, validator.Validate(nil, image), `securityParameters is required when image source type is "clone"`)
	})

	tests := []struct {
		name            string
		sourceEncrypted string
		targetEncrypted string
		targetEngine    string
		expectedError   string
	}{
		{name: "reject incompatible storage classes", sourceEncrypted: "false", targetEncrypted: "true", targetEngine: "v2", expectedError: "source and target image StorageClasses must use the same Longhorn data engine"},
		{name: "reject encrypted source", sourceEncrypted: "true", targetEncrypted: "true", targetEngine: "v1", expectedError: "can not re-encrypt, source image default/source () is already encrypted"},
		{name: "reject unencrypted target", sourceEncrypted: "false", targetEncrypted: "false", targetEngine: "v1", expectedError: "target image StorageClass target-sc is not configured for encryption"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sourceSC := &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{Name: "source-sc"}, Provisioner: util.CSIProvisionerLonghorn,
				Parameters: map[string]string{util.ParamDataEngine: "v1", util.LonghornOptionEncrypted: tc.sourceEncrypted},
			}
			targetSC := &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{Name: "target-sc"}, Provisioner: util.CSIProvisionerLonghorn,
				Parameters: map[string]string{util.ParamDataEngine: tc.targetEngine, util.LonghornOptionEncrypted: tc.targetEncrypted},
			}
			source := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "default"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend: harvesterv1.VMIBackendCDI, SourceType: harvesterv1.VirtualMachineImageSourceTypeDownload,
				},
				Status: harvesterv1.VirtualMachineImageStatus{StorageClassName: sourceSC.Name},
			}
			harvesterv1.ImageImported.True(source)
			target := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{util.AnnotationStorageClassName: targetSC.Name}},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend: harvesterv1.VMIBackendCDI, SourceType: harvesterv1.VirtualMachineImageSourceTypeClone,
					SecurityParameters: &harvesterv1.VirtualMachineImageSecurityParameters{
						CryptoOperation: harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
						SourceImageName: source.Name, SourceImageNamespace: source.Namespace,
					},
				},
			}
			clientset := fake.NewSimpleClientset(source, sourceSC, targetSC)
			validator := &securityValidator{
				vmiCache: fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
				scCache:  fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses),
				sar:      fakeclients.AllowedSARClient(),
			}
			assert.EqualError(t, validator.Validate(fakeclients.NewFakeRequest("test-user"), target), tc.expectedError)
		})
	}
}

func TestCheckRequiredSecurityParameters(t *testing.T) {
	tests := []struct {
		name          string
		parameters    *harvesterv1.VirtualMachineImageSecurityParameters
		expectedError string
		expectedField string
	}{
		{
			name: "complete security parameters",
			parameters: &harvesterv1.VirtualMachineImageSecurityParameters{
				CryptoOperation: harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
				SourceImageName: "source", SourceImageNamespace: "default",
			},
		},
		{
			name:          "missing security parameters",
			expectedError: `securityParameters is required when image source type is "clone"`,
			expectedField: "spec.securityParameters",
		},
		{
			name:          "missing operation",
			parameters:    &harvesterv1.VirtualMachineImageSecurityParameters{},
			expectedError: `cryptoOperation is required when image source type is "clone"`,
			expectedField: "spec.securityParameters.cryptoOperation",
		},
		{
			name: "missing source name",
			parameters: &harvesterv1.VirtualMachineImageSecurityParameters{
				CryptoOperation: harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt,
			},
			expectedError: `SourceImageName is required when image source type is "clone"`,
			expectedField: "spec.securityParameters.sourceImageName",
		},
		{
			name: "missing source namespace",
			parameters: &harvesterv1.VirtualMachineImageSecurityParameters{
				CryptoOperation: harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt, SourceImageName: "source",
			},
			expectedError: `SourceImageNamespace is required when image source type is "clone"`,
			expectedField: "spec.securityParameters.sourceImageNamespace",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkRequiredSecurityParameters(tc.parameters)
			if tc.expectedError == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tc.expectedError)
			var admitErr werror.AdmitError
			if assert.ErrorAs(t, err, &admitErr) {
				assert.Equal(t, tc.expectedField, admitErr.AsResult().Details.Causes[0].Field)
			}
		})
	}
}

func TestSecurityValidator_GetCloneSourceImage(t *testing.T) {
	backingImage := &harvesterv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "source-ns"},
		Spec:       harvesterv1.VirtualMachineImageSpec{Backend: harvesterv1.VMIBackendBackingImage},
	}
	harvesterv1.ImageImported.True(backingImage)
	cdiImage := backingImage.DeepCopy()
	cdiImage.Spec.Backend = harvesterv1.VMIBackendCDI
	pendingImage := backingImage.DeepCopy()
	harvesterv1.ImageImported.False(pendingImage)

	tests := []struct {
		name          string
		source        *harvesterv1.VirtualMachineImage
		targetBackend harvesterv1.VMIBackend
		allowed       bool
		expectedError string
	}{
		{name: "ready BackingImage source", source: backingImage, targetBackend: harvesterv1.VMIBackendBackingImage, allowed: true},
		{name: "ready CDI source", source: cdiImage, targetBackend: harvesterv1.VMIBackendCDI, allowed: true},
		{name: "access denied before source lookup", targetBackend: harvesterv1.VMIBackendBackingImage, expectedError: `user "test-user" is not authorized to access source image source-ns/source`},
		{name: "source not found", targetBackend: harvesterv1.VMIBackendBackingImage, allowed: true, expectedError: "source image source-ns/source not found"},
		{name: "source not imported", source: pendingImage, targetBackend: harvesterv1.VMIBackendBackingImage, allowed: true, expectedError: "source image source-ns/source () is not ready"},
		{name: "BackingImage source with CDI target", source: backingImage, targetBackend: harvesterv1.VMIBackendCDI, allowed: true, expectedError: "source and target image backends must match"},
		{name: "CDI source with BackingImage target", source: cdiImage, targetBackend: harvesterv1.VMIBackendBackingImage, allowed: true, expectedError: "source and target image backends must match"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			if tc.source != nil {
				assert.NoError(t, clientset.Tracker().Add(tc.source))
			}
			sar := fakeclients.SARClientSet(tc.allowed)
			validator := &securityValidator{
				vmiCache: fakeclients.VirtualMachineImageCache(clientset.HarvesterhciV1beta1().VirtualMachineImages),
				sar:      sar.AuthorizationV1().SubjectAccessReviews(),
			}
			target := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Namespace: "target-ns"},
				Spec: harvesterv1.VirtualMachineImageSpec{
					Backend: tc.targetBackend,
					SecurityParameters: &harvesterv1.VirtualMachineImageSecurityParameters{
						SourceImageName: "source", SourceImageNamespace: "source-ns",
					},
				},
			}
			source, err := validator.getCloneSourceImage(fakeclients.NewFakeRequest("test-user"), target)
			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
				assert.Nil(t, source)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.source, source)
		})
	}
}

func TestSecurityValidator_GetCloneStorageClasses(t *testing.T) {
	sourceSC := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "source-sc"}}
	targetSC := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "target-sc"}}
	annotations := map[string]string{util.AnnotationStorageClassName: targetSC.Name}
	tests := []struct {
		name           string
		annotations    map[string]string
		storageClasses []*storagev1.StorageClass
		expectedError  string
	}{
		{name: "use source status and target annotation", annotations: annotations, storageClasses: []*storagev1.StorageClass{sourceSC, targetSC}},
		{name: "missing target annotation", expectedError: "storageClassName is required when using security params"},
		{name: "missing target StorageClass", annotations: annotations, storageClasses: []*storagev1.StorageClass{sourceSC}, expectedError: "storage class target-sc not found"},
		{name: "missing source StorageClass", annotations: annotations, storageClasses: []*storagev1.StorageClass{targetSC}, expectedError: `failed to get source image StorageClass: storageclasses.storage.k8s.io "source-sc" not found`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			for _, sc := range tc.storageClasses {
				assert.NoError(t, clientset.Tracker().Add(sc))
			}
			validator := &securityValidator{scCache: fakeclients.StorageClassCache(clientset.StorageV1().StorageClasses)}
			// These template names deliberately differ from the resolved classes.
			source := &harvesterv1.VirtualMachineImage{
				Spec:   harvesterv1.VirtualMachineImageSpec{TargetStorageClassName: "source-template"},
				Status: harvesterv1.VirtualMachineImageStatus{StorageClassName: sourceSC.Name},
			}
			target := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Annotations: tc.annotations},
				Spec:       harvesterv1.VirtualMachineImageSpec{TargetStorageClassName: "target-template"},
			}
			actualSourceSC, actualTargetSC, err := validator.getCloneStorageClasses(target, source)
			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
				assert.Nil(t, actualSourceSC)
				assert.Nil(t, actualTargetSC)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, sourceSC, actualSourceSC)
			assert.Equal(t, targetSC, actualTargetSC)
		})
	}
}

func TestCheckCloneStorageClassCompatibility(t *testing.T) {
	const (
		longhornError   = "image encryption and decryption are currently supported only with Longhorn"
		sameEngineError = "source and target image StorageClasses must use the same Longhorn data engine"
	)
	longhornV1 := &storagev1.StorageClass{
		Provisioner: util.CSIProvisionerLonghorn, Parameters: map[string]string{util.ParamDataEngine: "v1"},
	}
	longhornV2 := &storagev1.StorageClass{
		Provisioner: util.CSIProvisionerLonghorn, Parameters: map[string]string{util.ParamDataEngine: "v2"},
	}
	defaultLonghorn := &storagev1.StorageClass{Provisioner: util.CSIProvisionerLonghorn}
	lvm := &storagev1.StorageClass{Provisioner: "lvm.driver.harvesterhci.io"}
	tests := []struct {
		name          string
		sourceSC      *storagev1.StorageClass
		targetSC      *storagev1.StorageClass
		expectedError string
	}{
		{name: "matching V1 engines", sourceSC: longhornV1, targetSC: longhornV1},
		{name: "matching V2 engines", sourceSC: longhornV2, targetSC: longhornV2},
		{name: "both engines default to V1", sourceSC: defaultLonghorn, targetSC: defaultLonghorn},
		{name: "default source engine matches V1", sourceSC: defaultLonghorn, targetSC: longhornV1},
		{name: "V1 matches default target engine", sourceSC: longhornV1, targetSC: defaultLonghorn},
		{name: "V1 source with V2 target", sourceSC: longhornV1, targetSC: longhornV2, expectedError: sameEngineError},
		{name: "V2 source with V1 target", sourceSC: longhornV2, targetSC: longhornV1, expectedError: sameEngineError},
		{name: "default source with V2 target", sourceSC: defaultLonghorn, targetSC: longhornV2, expectedError: sameEngineError},
		{name: "V2 source with default target", sourceSC: longhornV2, targetSC: defaultLonghorn, expectedError: sameEngineError},
		{name: "non-Longhorn source", sourceSC: lvm, targetSC: longhornV1, expectedError: longhornError},
		{name: "non-Longhorn target", sourceSC: longhornV1, targetSC: lvm, expectedError: longhornError},
		{name: "both classes use another provisioner", sourceSC: lvm, targetSC: lvm, expectedError: longhornError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkCloneStorageClassCompatibility(tc.sourceSC, tc.targetSC)
			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckSourceEncryption_BackingImage(t *testing.T) {
	const (
		encrypt                = harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt
		decrypt                = harvesterv1.VirtualMachineImageCryptoOperationTypeDecrypt
		download               = harvesterv1.VirtualMachineImageSourceTypeDownload
		upload                 = harvesterv1.VirtualMachineImageSourceTypeUpload
		clone                  = harvesterv1.VirtualMachineImageSourceTypeClone
		encryptedSourceError   = "can not re-encrypt, source image default/source () is already encrypted"
		unencryptedSourceError = "can not re-decrypt, source image default/source () is not encrypted"
	)
	tests := []struct {
		name            string
		sourceType      harvesterv1.VirtualMachineImageSourceType
		sourceOperation harvesterv1.VirtualMachineImageCryptoOperationType
		scEncrypted     string
		operation       harvesterv1.VirtualMachineImageCryptoOperationType
		expectedError   string
	}{
		{name: "encrypt download with encrypted template", sourceType: download, scEncrypted: "true", operation: encrypt},
		{name: "reject decrypting download with encrypted template", sourceType: download, scEncrypted: "true", operation: decrypt, expectedError: unencryptedSourceError},
		{name: "encrypt upload with encrypted template", sourceType: upload, scEncrypted: "true", operation: encrypt},
		{name: "reject decrypting upload with encrypted template", sourceType: upload, scEncrypted: "true", operation: decrypt, expectedError: unencryptedSourceError},
		{name: "encrypt previously decrypted image", sourceType: clone, sourceOperation: decrypt, scEncrypted: "true", operation: encrypt},
		{name: "reject decrypting previously decrypted image", sourceType: clone, sourceOperation: decrypt, scEncrypted: "true", operation: decrypt, expectedError: unencryptedSourceError},
		{name: "reject re-encryption despite unencrypted StorageClass", sourceType: clone, sourceOperation: encrypt, scEncrypted: "false", operation: encrypt, expectedError: encryptedSourceError},
		{name: "decrypt encrypted image despite unencrypted StorageClass", sourceType: clone, sourceOperation: encrypt, scEncrypted: "false", operation: decrypt},
		{name: "encrypt upload with unencrypted template", sourceType: upload, scEncrypted: "false", operation: encrypt},
		{name: "reject decrypting unencrypted download", sourceType: download, scEncrypted: "false", operation: decrypt, expectedError: unencryptedSourceError},
		{name: "reject decrypting upload with omitted encryption parameter", sourceType: upload, operation: decrypt, expectedError: unencryptedSourceError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "default"},
				Spec:       harvesterv1.VirtualMachineImageSpec{Backend: harvesterv1.VMIBackendBackingImage, SourceType: tc.sourceType},
			}
			if tc.sourceOperation != "" {
				source.Spec.SecurityParameters = &harvesterv1.VirtualMachineImageSecurityParameters{CryptoOperation: tc.sourceOperation}
			}
			sc := &storagev1.StorageClass{}
			if tc.scEncrypted != "" {
				sc.Parameters = map[string]string{util.LonghornOptionEncrypted: tc.scEncrypted}
			}
			err := checkSourceEncryption(source, sc, tc.operation)
			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckSourceEncryption_CDI(t *testing.T) {
	const (
		encrypt                = harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt
		decrypt                = harvesterv1.VirtualMachineImageCryptoOperationTypeDecrypt
		download               = harvesterv1.VirtualMachineImageSourceTypeDownload
		upload                 = harvesterv1.VirtualMachineImageSourceTypeUpload
		clone                  = harvesterv1.VirtualMachineImageSourceTypeClone
		encryptedSourceError   = "can not re-encrypt, source image default/source () is already encrypted"
		unencryptedSourceError = "can not re-decrypt, source image default/source () is not encrypted"
	)
	tests := []struct {
		name            string
		sourceType      harvesterv1.VirtualMachineImageSourceType
		sourceOperation harvesterv1.VirtualMachineImageCryptoOperationType
		scEncrypted     string
		operation       harvesterv1.VirtualMachineImageCryptoOperationType
		expectedError   string
	}{
		{name: "decrypt encrypted download without clone history", sourceType: download, scEncrypted: "true", operation: decrypt},
		{name: "decrypt encrypted upload without clone history", sourceType: upload, scEncrypted: "true", operation: decrypt},
		{name: "reject re-encryption of encrypted download", sourceType: download, scEncrypted: "true", operation: encrypt, expectedError: encryptedSourceError},
		{name: "decrypt encrypted storage despite decrypt history", sourceType: clone, sourceOperation: decrypt, scEncrypted: "true", operation: decrypt},
		{name: "encrypt unencrypted storage despite encrypt history", sourceType: clone, sourceOperation: encrypt, scEncrypted: "false", operation: encrypt},
		{name: "reject decrypting unencrypted storage despite encrypt history", sourceType: clone, sourceOperation: encrypt, scEncrypted: "false", operation: decrypt, expectedError: unencryptedSourceError},
		{name: "reject decrypting unencrypted download", sourceType: download, scEncrypted: "false", operation: decrypt, expectedError: unencryptedSourceError},
		{name: "reject decrypting upload with omitted encryption parameter", sourceType: upload, operation: decrypt, expectedError: unencryptedSourceError},
		{name: "encrypt unencrypted upload", sourceType: upload, scEncrypted: "false", operation: encrypt},
		{name: "encrypt storage with omitted parameter despite encrypt history", sourceType: clone, sourceOperation: encrypt, operation: encrypt},
		{name: "reject decrypting storage with omitted parameter despite encrypt history", sourceType: clone, sourceOperation: encrypt, operation: decrypt, expectedError: unencryptedSourceError},
		{name: "reject invalid encryption parameter", sourceType: download, scEncrypted: "invalid", operation: encrypt, expectedError: "source image StorageClass source-sc has an invalid encrypted parameter"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := &harvesterv1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "default"},
				Spec:       harvesterv1.VirtualMachineImageSpec{Backend: harvesterv1.VMIBackendCDI, SourceType: tc.sourceType},
			}
			if tc.sourceOperation != "" {
				source.Spec.SecurityParameters = &harvesterv1.VirtualMachineImageSecurityParameters{CryptoOperation: tc.sourceOperation}
			}
			sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "source-sc"}}
			if tc.scEncrypted != "" {
				sc.Parameters = map[string]string{util.LonghornOptionEncrypted: tc.scEncrypted}
			}
			err := checkSourceEncryption(source, sc, tc.operation)
			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckTargetEncryption(t *testing.T) {
	const (
		encrypt           = harvesterv1.VirtualMachineImageCryptoOperationTypeEncrypt
		decrypt           = harvesterv1.VirtualMachineImageCryptoOperationTypeDecrypt
		backingImageError = "target image StorageClass target-sc is tied to a specific backing image and cannot be used as a target for encryption or decryption"
		invalidValueError = "target image StorageClass target-sc has an invalid encrypted parameter"
	)
	tests := []struct {
		name          string
		parameters    map[string]string
		operation     harvesterv1.VirtualMachineImageCryptoOperationType
		expectedError string
	}{
		{name: "allow encryption", parameters: map[string]string{util.LonghornOptionEncrypted: "true"}, operation: encrypt},
		{name: "allow decryption", parameters: map[string]string{util.LonghornOptionEncrypted: "false"}, operation: decrypt},
		{name: "allow decryption when encryption parameter is omitted", operation: decrypt},
		{name: "reject encryption into unencrypted target", operation: encrypt, expectedError: "target image StorageClass target-sc is not configured for encryption"},
		{name: "reject decryption into encrypted target", parameters: map[string]string{util.LonghornOptionEncrypted: "true"}, operation: decrypt, expectedError: "image decryption requires an unencrypted target StorageClass"},
		{name: "reject invalid encryption parameter for encryption", parameters: map[string]string{util.LonghornOptionEncrypted: "invalid"}, operation: encrypt, expectedError: invalidValueError},
		{name: "reject invalid encryption parameter for decryption", parameters: map[string]string{util.LonghornOptionEncrypted: "invalid"}, operation: decrypt, expectedError: invalidValueError},

		// Fixed backing images are rejected before checking the encryption state.
		{name: "reject encrypted backing-image target for encryption", parameters: map[string]string{util.LonghornOptionBackingImageName: "existing-image", util.LonghornOptionEncrypted: "true"}, operation: encrypt, expectedError: backingImageError},
		{name: "reject unencrypted backing-image target for encryption", parameters: map[string]string{util.LonghornOptionBackingImageName: "existing-image", util.LonghornOptionEncrypted: "false"}, operation: encrypt, expectedError: backingImageError},
		{name: "reject encrypted backing-image target for decryption", parameters: map[string]string{util.LonghornOptionBackingImageName: "existing-image", util.LonghornOptionEncrypted: "true"}, operation: decrypt, expectedError: backingImageError},
		{name: "reject unencrypted backing-image target for decryption", parameters: map[string]string{util.LonghornOptionBackingImageName: "existing-image", util.LonghornOptionEncrypted: "false"}, operation: decrypt, expectedError: backingImageError},
		{name: "reject backing-image target with omitted encryption parameter", parameters: map[string]string{util.LonghornOptionBackingImageName: "existing-image"}, operation: decrypt, expectedError: backingImageError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "target-sc"}, Parameters: tc.parameters}
			err := checkTargetEncryption(sc, tc.operation)
			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
