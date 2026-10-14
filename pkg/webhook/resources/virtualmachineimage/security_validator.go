package virtualmachineimage

import (
	"fmt"
	"strconv"

	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	authorizationv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

type securityValidator struct {
	vmio    common.VMIOperator
	scCache ctlstoragev1.StorageClassCache
	sar     authorizationv1client.SubjectAccessReviewInterface
}

// Validate checks encryption and decryption requests for Longhorn image clones.
func (v *securityValidator) Validate(
	request *types.Request,
	vmi *v1beta1.VirtualMachineImage,
) error {
	if v.vmio.GetSourceType(vmi) != v1beta1.VirtualMachineImageSourceTypeClone {
		return nil
	}
	if err := v.checkSecurity(vmi); err != nil {
		return err
	}

	source, err := v.getSource(request, vmi)
	if err != nil {
		return err
	}
	sourceSC, targetSC, err := v.getStorageClasses(vmi, source)
	if err != nil {
		return err
	}
	if err := checkStorageClasses(sourceSC, targetSC); err != nil {
		return err
	}
	operation := v1beta1.VirtualMachineImageCryptoOperationType(
		v.vmio.GetSecurityCryptoOption(vmi),
	)
	if err := v.checkSourceEncryption(source, sourceSC, operation); err != nil {
		return err
	}
	return checkTargetEncryption(targetSC, operation)
}

func (v *securityValidator) checkSecurity(vmi *v1beta1.VirtualMachineImage) error {
	// The security getters require non-nil parameters.
	if vmi.Spec.SecurityParameters == nil {
		return werror.NewInvalidError(
			`securityParameters is required when image source type is "clone"`,
			"spec.securityParameters",
		)
	}
	if v.vmio.GetSecurityCryptoOption(vmi) == "" {
		return werror.NewInvalidError(
			`cryptoOperation is required when image source type is "clone"`,
			"spec.securityParameters.cryptoOperation",
		)
	}
	if v.vmio.GetSecuritySrcImgName(vmi) == "" {
		return werror.NewInvalidError(
			`SourceImageName is required when image source type is "clone"`,
			"spec.securityParameters.sourceImageName",
		)
	}
	if v.vmio.GetSecuritySrcImgNamespace(vmi) == "" {
		return werror.NewInvalidError(
			`SourceImageNamespace is required when image source type is "clone"`,
			"spec.securityParameters.sourceImageNamespace",
		)
	}
	return nil
}

func (v *securityValidator) getSource(
	request *types.Request,
	vmi *v1beta1.VirtualMachineImage,
) (*v1beta1.VirtualMachineImage, error) {
	namespace := v.vmio.GetSecuritySrcImgNamespace(vmi)
	name := v.vmio.GetSecuritySrcImgName(vmi)
	if err := v.checkSourceAccess(request, namespace, name); err != nil {
		return nil, err
	}

	source, err := v.vmio.GetVMImageObj(namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, werror.NewInvalidError(
				fmt.Sprintf("source image %s/%s not found", namespace, name),
				"spec.securityParameters.sourceImageName",
			)
		}
		return nil, werror.NewInternalError(
			fmt.Sprintf("failed to get source image %s/%s: %v", namespace, name, err),
		)
	}
	if !v.vmio.IsImported(source) {
		return nil, werror.NewInvalidError(
			fmt.Sprintf("source image %s/%s (%s) is not ready",
				v.vmio.GetNamespace(source), v.vmio.GetName(source),
				v.vmio.GetDisplayName(source)),
			"",
		)
	}
	if util.GetVMIBackend(source) != util.GetVMIBackend(vmi) {
		return nil, werror.NewInvalidError(
			"source and target image backends must match", "spec.backend",
		)
	}
	return source, nil
}

func (v *securityValidator) checkSourceAccess(
	request *types.Request,
	namespace, name string,
) error {
	allowed, err := util.CheckObjectAccess(request.Context, util.ResourceAccessCheck{
		SAR:       v.sar,
		Username:  request.UserInfo.Username,
		Groups:    request.UserInfo.Groups,
		Verb:      util.VerbGet,
		GVR:       util.VirtualMachineImageGVR,
		Namespace: namespace,
		Name:      name,
	})
	if err != nil {
		return werror.NewInternalError(
			fmt.Sprintf("failed to check access to source image %s/%s: %v",
				namespace, name, err),
		)
	}
	if !allowed {
		return werror.NewForbidden(
			fmt.Sprintf("user %q is not authorized to access source image %s/%s",
				request.UserInfo.Username, namespace, name),
			"",
		)
	}
	return nil
}

func (v *securityValidator) getStorageClasses(
	vmi, source *v1beta1.VirtualMachineImage,
) (*storagev1.StorageClass, *storagev1.StorageClass, error) {
	targetSC, err := v.getTargetSC(vmi)
	if err != nil {
		return nil, nil, err
	}
	// The actual source class can differ from its template or resolved class.
	sourceSC, err := v.scCache.Get(source.Status.StorageClassName)
	if err != nil {
		return nil, nil, werror.NewInvalidError(
			fmt.Sprintf("failed to get source image StorageClass: %v", err),
			"spec.securityParameters.sourceImageName",
		)
	}
	return sourceSC, targetSC, nil
}

func (v *securityValidator) getTargetSC(
	vmi *v1beta1.VirtualMachineImage,
) (*storagev1.StorageClass, error) {
	const field = "spec.targetStorageClassName"
	name := vmi.Spec.TargetStorageClassName
	if name == "" {
		return nil, werror.NewInvalidError(
			"targetStorageClassName is required when using security params", field,
		)
	}
	sc, err := v.scCache.Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, werror.NewInvalidError(
				fmt.Sprintf("storage class %s not found", name), field,
			)
		}
		return nil, werror.NewInternalError(
			fmt.Sprintf("failed to get storage class %s: %v", name, err),
		)
	}
	return sc, nil
}

func checkStorageClasses(sourceSC, targetSC *storagev1.StorageClass) error {
	if sourceSC.Provisioner != util.CSIProvisionerLonghorn ||
		targetSC.Provisioner != util.CSIProvisionerLonghorn {
		return werror.NewInvalidError(
			"image encryption and decryption are currently supported only with Longhorn",
			"spec.targetStorageClassName",
		)
	}
	if getDataEngine(sourceSC) != getDataEngine(targetSC) {
		return werror.NewInvalidError(
			"source and target image StorageClasses must use the same Longhorn data engine",
			"spec.targetStorageClassName",
		)
	}
	return nil
}

func getDataEngine(sc *storagev1.StorageClass) string {
	if engine := sc.Parameters[util.ParamDataEngine]; engine != "" {
		return engine
	}
	return "v1"
}

func (v *securityValidator) checkSourceEncryption(
	source *v1beta1.VirtualMachineImage,
	sourceSC *storagev1.StorageClass,
	operation v1beta1.VirtualMachineImageCryptoOperationType,
) error {
	encrypted, err := v.getSourceEncryption(source, sourceSC)
	if err != nil {
		return err
	}
	sourceRef := fmt.Sprintf("%s/%s (%s)", v.vmio.GetNamespace(source),
		v.vmio.GetName(source), v.vmio.GetDisplayName(source))
	if encrypted && operation == v1beta1.VirtualMachineImageCryptoOperationTypeEncrypt {
		return werror.NewInvalidError(
			fmt.Sprintf("can not re-encrypt, source image %s is already encrypted", sourceRef), "",
		)
	}
	if !encrypted && operation == v1beta1.VirtualMachineImageCryptoOperationTypeDecrypt {
		return werror.NewInvalidError(
			fmt.Sprintf("can not re-decrypt, source image %s is not encrypted", sourceRef), "",
		)
	}
	return nil
}

func (v *securityValidator) getSourceEncryption(
	source *v1beta1.VirtualMachineImage,
	sourceSC *storagev1.StorageClass,
) (bool, error) {
	// Backing-image downloads/uploads do not encrypt the backing-image file.
	// Only an explicit encrypt operation marks that file as encrypted.
	if util.GetVMIBackend(source) != v1beta1.VMIBackendCDI {
		return v.vmio.IsEncryptOperation(source), nil
	}
	// CDI stores image data in a PVC whose StorageClass controls encryption.
	encrypted, err := parseEncryption(sourceSC)
	if err != nil {
		return false, werror.NewInvalidError(
			fmt.Sprintf("source image StorageClass %s has an invalid encrypted parameter",
				sourceSC.Name),
			"spec.securityParameters.sourceImageName",
		)
	}
	return encrypted, nil
}

func checkTargetEncryption(
	targetSC *storagev1.StorageClass,
	operation v1beta1.VirtualMachineImageCryptoOperationType,
) error {
	if targetSC.Parameters[util.LonghornOptionBackingImageName] != "" {
		return werror.NewInvalidError(
			fmt.Sprintf("target image StorageClass %s is tied to a specific backing image "+
				"and cannot be used as a target for encryption or decryption", targetSC.Name),
			"spec.targetStorageClassName",
		)
	}
	encrypted, err := parseEncryption(targetSC)
	if err != nil {
		return werror.NewInvalidError(
			fmt.Sprintf("target image StorageClass %s has an invalid encrypted parameter",
				targetSC.Name),
			"spec.targetStorageClassName",
		)
	}
	if encrypted && operation == v1beta1.VirtualMachineImageCryptoOperationTypeDecrypt {
		return werror.NewInvalidError(
			"image decryption requires an unencrypted target StorageClass",
			"spec.targetStorageClassName",
		)
	}
	if !encrypted && operation == v1beta1.VirtualMachineImageCryptoOperationTypeEncrypt {
		return werror.NewInvalidError(
			fmt.Sprintf("target image StorageClass %s is not configured for encryption",
				targetSC.Name),
			"spec.targetStorageClassName",
		)
	}
	return nil
}

func parseEncryption(sc *storagev1.StorageClass) (bool, error) {
	value, ok := sc.Parameters[util.LonghornOptionEncrypted]
	if !ok {
		return false, nil
	}
	return strconv.ParseBool(value)
}
