package virtualmachineimage

import (
	"fmt"
	"strconv"

	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	authorizationv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type securityValidator struct {
	vmiCache ctlharvesterv1.VirtualMachineImageCache
	scCache  ctlstoragev1.StorageClassCache
	sar      authorizationv1client.SubjectAccessReviewInterface
}

// Validate applies only to clone operations, which encrypt or decrypt an image.
// These operations currently support only the Longhorn provisioner.
// Validation checks the required security parameters, source access, storage
// compatibility, and the encryption state of the source and target.
func (v *securityValidator) Validate(request *types.Request, vmi *v1beta1.VirtualMachineImage) error {
	if vmi.Spec.SourceType != v1beta1.VirtualMachineImageSourceTypeClone {
		return nil
	}

	sp := vmi.Spec.SecurityParameters
	if err := checkRequiredSecurityParameters(sp); err != nil {
		return err
	}

	sourceImage, err := v.getCloneSourceImage(request, vmi)
	if err != nil {
		return err
	}
	sourceSC, targetSC, err := v.getCloneStorageClasses(vmi, sourceImage)
	if err != nil {
		return err
	}
	if err := checkCloneStorageClassCompatibility(sourceSC, targetSC); err != nil {
		return err
	}
	if err := checkSourceEncryption(sourceImage, sourceSC, sp.CryptoOperation); err != nil {
		return err
	}
	return checkTargetEncryption(targetSC, sp.CryptoOperation)
}

// checkRequiredSecurityParameters ensures a clone request specifies a crypto
// operation and the source image's name and namespace.
func checkRequiredSecurityParameters(sp *v1beta1.VirtualMachineImageSecurityParameters) error {
	if sp == nil {
		return werror.NewInvalidError(`securityParameters is required when image source type is "clone"`, "spec.securityParameters")
	}
	if sp.CryptoOperation == "" {
		return werror.NewInvalidError(`cryptoOperation is required when image source type is "clone"`, "spec.securityParameters.cryptoOperation")
	}
	if sp.SourceImageName == "" {
		return werror.NewInvalidError(`SourceImageName is required when image source type is "clone"`, "spec.securityParameters.sourceImageName")
	}
	if sp.SourceImageNamespace == "" {
		return werror.NewInvalidError(`SourceImageNamespace is required when image source type is "clone"`, "spec.securityParameters.sourceImageNamespace")
	}
	return nil
}

// getCloneSourceImage checks read access before loading the source image and
// verifies that it is imported and uses the target image's backend.
func (v *securityValidator) getCloneSourceImage(request *types.Request, vmi *v1beta1.VirtualMachineImage) (*v1beta1.VirtualMachineImage, error) {
	sp := vmi.Spec.SecurityParameters
	allowed, err := util.CheckObjectAccess(request.Context, util.ResourceAccessCheck{
		SAR:       v.sar,
		Username:  request.UserInfo.Username,
		Groups:    request.UserInfo.Groups,
		Verb:      util.VerbGet,
		GVR:       util.VirtualMachineImageGVR,
		Namespace: sp.SourceImageNamespace,
		Name:      sp.SourceImageName,
	})
	if err != nil {
		return nil, werror.NewInternalError(fmt.Sprintf("failed to check access to source image %s/%s: %v", sp.SourceImageNamespace, sp.SourceImageName, err))
	}
	if !allowed {
		return nil, werror.NewForbidden(fmt.Sprintf("user %q is not authorized to access source image %s/%s", request.UserInfo.Username, sp.SourceImageNamespace, sp.SourceImageName), "")
	}

	sourceImage, err := v.vmiCache.Get(sp.SourceImageNamespace, sp.SourceImageName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, werror.NewInvalidError(fmt.Sprintf("source image %s/%s not found", sp.SourceImageNamespace, sp.SourceImageName), "spec.securityParameters.sourceImageName")
		}
		return nil, werror.NewInternalError(fmt.Sprintf("failed to get source image %s/%s: %v", sp.SourceImageNamespace, sp.SourceImageName, err))
	}
	if !v1beta1.ImageImported.IsTrue(sourceImage) {
		return nil, werror.NewInvalidError(fmt.Sprintf("source image %s/%s (%s) is not ready", sourceImage.Namespace, sourceImage.Name, sourceImage.Spec.DisplayName), "")
	}
	if sourceImage.Spec.Backend != vmi.Spec.Backend {
		return nil, werror.NewInvalidError("source and target image backends must match", "spec.backend")
	}
	return sourceImage, nil
}

// getCloneStorageClasses returns the source image's actual StorageClass and
// the target StorageClass selected by the storageClassName annotation.
func (v *securityValidator) getCloneStorageClasses(vmi, sourceImage *v1beta1.VirtualMachineImage) (*storagev1.StorageClass, *storagev1.StorageClass, error) {
	scName, ok := vmi.Annotations[util.AnnotationStorageClassName]
	if !ok {
		return nil, nil, werror.NewInvalidError("storageClassName is required when using security params", fmt.Sprintf("metadata.annotations[%s]", util.AnnotationStorageClassName))
	}
	targetSC, err := v.scCache.Get(scName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil, werror.NewInvalidError(fmt.Sprintf("storage class %s not found", scName), fmt.Sprintf("metadata.annotations[%s]", util.AnnotationStorageClassName))
		}
		return nil, nil, werror.NewInternalError(fmt.Sprintf("failed to get storage class %s: %v", scName, err))
	}
	sourceSC, err := v.scCache.Get(sourceImage.Status.StorageClassName)
	if err != nil {
		return nil, nil, werror.NewInvalidError(fmt.Sprintf("failed to get source image StorageClass: %v", err), "spec.securityParameters.sourceImageName")
	}
	return sourceSC, targetSC, nil
}

// checkCloneStorageClassCompatibility requires Longhorn storage classes with
// matching data engines, treating an omitted dataEngine as v1.
func checkCloneStorageClassCompatibility(sourceSC, targetSC *storagev1.StorageClass) error {
	if sourceSC.Provisioner != util.CSIProvisionerLonghorn || targetSC.Provisioner != util.CSIProvisionerLonghorn {
		return werror.NewInvalidError("image encryption and decryption are currently supported only with Longhorn", "spec.targetStorageClassName")
	}
	sourceEngine := sourceSC.Parameters[util.ParamDataEngine]
	targetEngine := targetSC.Parameters[util.ParamDataEngine]
	if sourceEngine == "" {
		sourceEngine = "v1"
	}
	if targetEngine == "" {
		targetEngine = "v1"
	}
	if sourceEngine != targetEngine {
		return werror.NewInvalidError("source and target image StorageClasses must use the same Longhorn data engine", "spec.targetStorageClassName")
	}
	return nil
}

// checkSourceEncryption uses the source image's crypto operation for backing
// images and its StorageClass's encrypted parameter for CDI images.
func checkSourceEncryption(sourceImage *v1beta1.VirtualMachineImage, sourceSC *storagev1.StorageClass, operation v1beta1.VirtualMachineImageCryptoOperationType) error {
	// For backing-image downloads/uploads, selecting an encrypted StorageClass
	// copies encrypted=true into the generated StorageClass but does not encrypt
	// the backing-image file itself. createBackingImage requests encryption only
	// for clone operations with cryptoOperation=encrypt, so use the source image's
	// security parameters to determine whether the backing image is encrypted.
	sp := sourceImage.Spec.SecurityParameters
	sourceEncrypted := sp != nil && sp.CryptoOperation == v1beta1.VirtualMachineImageCryptoOperationTypeEncrypt
	if sourceImage.Spec.Backend == v1beta1.VMIBackendCDI {
		// CDI stores image data in a PVC, whose StorageClass controls encryption
		// even for downloads/uploads without an explicit crypto operation.
		sourceEncrypted = false
		if value, ok := sourceSC.Parameters[util.LonghornOptionEncrypted]; ok {
			var err error
			sourceEncrypted, err = strconv.ParseBool(value)
			if err != nil {
				return werror.NewInvalidError(fmt.Sprintf("source image StorageClass %s has an invalid encrypted parameter", sourceSC.Name), "spec.securityParameters.sourceImageName")
			}
		}
	}
	if sourceEncrypted && operation == v1beta1.VirtualMachineImageCryptoOperationTypeEncrypt {
		return werror.NewInvalidError(fmt.Sprintf("can not re-encrypt, source image %s/%s (%s) is already encrypted", sourceImage.Namespace, sourceImage.Name, sourceImage.Spec.DisplayName), "")
	}
	if !sourceEncrypted && operation == v1beta1.VirtualMachineImageCryptoOperationTypeDecrypt {
		return werror.NewInvalidError(fmt.Sprintf("can not re-decrypt, source image %s/%s (%s) is not encrypted", sourceImage.Namespace, sourceImage.Name, sourceImage.Spec.DisplayName), "")
	}
	return nil
}

// checkTargetEncryption first rejects target StorageClasses tied to a fixed
// backing image, then requires an encrypted target for encryption and an
// unencrypted target for decryption.
func checkTargetEncryption(targetSC *storagev1.StorageClass, operation v1beta1.VirtualMachineImageCryptoOperationType) error {
	if targetSC.Parameters[util.LonghornOptionBackingImageName] != "" {
		return werror.NewInvalidError(fmt.Sprintf("target image StorageClass %s is tied to a specific backing image and cannot be used as a target for encryption or decryption", targetSC.Name), fmt.Sprintf("metadata.annotations[%s]", util.AnnotationStorageClassName))
	}

	targetEncrypted := false
	if value, ok := targetSC.Parameters[util.LonghornOptionEncrypted]; ok {
		var parseErr error
		targetEncrypted, parseErr = strconv.ParseBool(value)
		if parseErr != nil {
			return werror.NewInvalidError(fmt.Sprintf("target image StorageClass %s has an invalid encrypted parameter", targetSC.Name), "spec.targetStorageClassName")
		}
	}
	if targetEncrypted && operation == v1beta1.VirtualMachineImageCryptoOperationTypeDecrypt {
		return werror.NewInvalidError("image decryption requires an unencrypted target StorageClass", "spec.targetStorageClassName")
	}
	if !targetEncrypted && operation == v1beta1.VirtualMachineImageCryptoOperationTypeEncrypt {
		return werror.NewInvalidError(fmt.Sprintf("target image StorageClass %s is not configured for encryption", targetSC.Name), "spec.targetStorageClassName")
	}
	return nil
}
