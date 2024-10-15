package virtualmachineimage

import (
	"fmt"
	"reflect"
	"strings"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	authorizationv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/indexeres"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	fieldDisplayName = "spec.displayName"
)

func NewValidator(
	vmimages ctlharvesterv1.VirtualMachineImageCache,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	ssar authorizationv1client.SelfSubjectAccessReviewInterface,
	vmTemplateVersionCache ctlharvesterv1.VirtualMachineTemplateVersionCache,
	secretCache ctlcorev1.SecretCache,
	storageClassCache ctlstoragev1.StorageClassCache,
	vmBackupCache ctlharvesterv1.VirtualMachineBackupCache) types.Validator {
	return &virtualMachineImageValidator{
		vmimages:               vmimages,
		pvcCache:               pvcCache,
		ssar:                   ssar,
		vmTemplateVersionCache: vmTemplateVersionCache,
		secretCache:            secretCache,
		storageClassCache:      storageClassCache,
		vmBackupCache:          vmBackupCache,
	}
}

type virtualMachineImageValidator struct {
	types.DefaultValidator

	vmimages               ctlharvesterv1.VirtualMachineImageCache
	pvcCache               ctlcorev1.PersistentVolumeClaimCache
	ssar                   authorizationv1client.SelfSubjectAccessReviewInterface
	vmTemplateVersionCache ctlharvesterv1.VirtualMachineTemplateVersionCache
	secretCache            ctlcorev1.SecretCache
	storageClassCache      ctlstoragev1.StorageClassCache
	vmBackupCache          ctlharvesterv1.VirtualMachineBackupCache
}

func (v *virtualMachineImageValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.VirtualMachineImageResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.VirtualMachineImage{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
			admissionregv1.Delete,
		},
	}
}

func (v *virtualMachineImageValidator) Create(request *types.Request, newObj runtime.Object) error {
	newImage := newObj.(*v1beta1.VirtualMachineImage)
	if err := v.CheckImageDisplayNameAndURL(newImage); err != nil {
		return err
	}

	if err := v.checkImageSecurityParameters(newImage); err != nil {
		return err
	}

	return v.CheckImagePVC(request, newImage)
}

func (v *virtualMachineImageValidator) CheckImageDisplayNameAndURL(newImage *v1beta1.VirtualMachineImage) error {
	if newImage.Spec.DisplayName == "" {
		return werror.NewInvalidError("displayName is required", fieldDisplayName)
	}

	sameDisplayNameImages, err := v.vmimages.List(newImage.Namespace, labels.SelectorFromSet(map[string]string{
		util.LabelImageDisplayName: newImage.Spec.DisplayName,
	}))
	if err != nil {
		return err
	}
	for _, image := range sameDisplayNameImages {
		if newImage.Name == image.Name {
			continue
		}
		return werror.NewConflict("A resource with the same name exists")
	}

	shouldHaveURL := false
	if newImage.Spec.SourceType == v1beta1.VirtualMachineImageSourceTypeDownload || newImage.Spec.SourceType == v1beta1.VirtualMachineImageSourceTypeRestore {
		shouldHaveURL = true
	}

	if shouldHaveURL && newImage.Spec.URL == "" {
		return werror.NewInvalidError(fmt.Sprintf(`url is required when image source type is "%s"`, newImage.Spec.SourceType), "spec.url")
	} else if !shouldHaveURL && newImage.Spec.URL != "" {
		return werror.NewInvalidError(fmt.Sprintf(`url should be empty when image source type is "%s"`, newImage.Spec.SourceType), "spec.url")
	}

	return nil
}

func (v *virtualMachineImageValidator) checkImageSecurityParameters(newImage *v1beta1.VirtualMachineImage) error {
	if newImage.Spec.SourceType != v1beta1.VirtualMachineImageSourceTypeClone {
		return nil
	}

	sp := newImage.Spec.SecurityParameters
	if sp == nil {
		return werror.NewInvalidError(`sourceParameters is required when image source type is "clone"`, "spec.sourceParameters")
	}

	if sp.CryptoOperation == "" {
		return werror.NewInvalidError(`encryption is required when image source type is "clone"`, "spec.cryptoOperation")
	}

	if sp.SourceImageName == "" {
		return werror.NewInvalidError(`SourceImageName is required when image source type is "clone"`, "spec.security.sourceImageName")
	}

	if sp.SourceImageNamespace == "" {
		return werror.NewInvalidError(`SourceImageNamespace is required when image source type is "clone"`, "spec.security.sourceImageName")
	}

	// Check if the source image exists
	sourceImage, err := v.vmimages.Get(sp.SourceImageNamespace, sp.SourceImageName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return werror.NewInvalidError(fmt.Sprintf("source image %s/%s not found", sp.SourceImageNamespace, sp.SourceImageName), "spec.securityParameters.sourceImageName")
		}
		return werror.NewInternalError(fmt.Sprintf("failed to get source image %s/%s: %v", sp.SourceImageNamespace, sp.SourceImageName, err))
	}

	sourceSp := sourceImage.Spec.SecurityParameters
	if (sourceSp != nil && sourceSp.CryptoOperation == v1beta1.VirtualMachineImageCryptoOperationTypeEncrypt) &&
		sp.CryptoOperation == v1beta1.VirtualMachineImageCryptoOperationTypeEncrypt {
		return werror.NewInvalidError(fmt.Sprintf("can not re-encrypt, source image %s/%s (%s) is already encrypted", sourceImage.Namespace, sourceImage.Name, sourceImage.Spec.DisplayName), "")
	}

	if (sourceSp == nil || sourceSp.CryptoOperation == v1beta1.VirtualMachineImageCryptoOperationTypeDecrypt) &&
		sp.CryptoOperation == v1beta1.VirtualMachineImageCryptoOperationTypeDecrypt {
		return werror.NewInvalidError(fmt.Sprintf("can not re-decrypt, source image %s/%s (%s) is not encrypted", sourceImage.Namespace, sourceImage.Name, sourceImage.Spec.DisplayName), "")
	}

	if !v1beta1.ImageImported.IsTrue(sourceImage) {
		return werror.NewInvalidError(fmt.Sprintf("source image %s/%s (%s) is not ready", sourceImage.Namespace, sourceImage.Name, sourceImage.Spec.DisplayName), "")
	}

	// For decryption, we don't need to check the storage class.
	// We could use sourceImage to track back to get the storage class.
	if sp.CryptoOperation == v1beta1.VirtualMachineImageCryptoOperationTypeDecrypt {
		return nil
	}

	// Check if storage class exits
	scName, ok := newImage.Annotations[util.AnnotationStorageClassName]
	if !ok {
		return werror.NewInvalidError("storageClassName is required when using security params", fmt.Sprintf("metadata.annotations[%s]", util.AnnotationStorageClassName))
	}

	sc, err := v.storageClassCache.Get(scName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return werror.NewInvalidError(fmt.Sprintf("storage class %s not found", scName), fmt.Sprintf("metadata.annotations[%s]", util.AnnotationStorageClassName))
		}
		return werror.NewInternalError(fmt.Sprintf("failed to get storage class %s: %v", scName, err))
	}

	value := sc.Parameters[util.LonghornOptionEncrypted]
	if value != "true" {
		return werror.NewInvalidError(fmt.Sprintf("storage class %s is not for encryption or decryption", scName), fmt.Sprintf("spec.parameters[%s] must be true", util.LonghornOptionEncrypted))
	}

	return nil
}

func (v *virtualMachineImageValidator) CheckImagePVC(request *types.Request, newImage *v1beta1.VirtualMachineImage) error {
	if newImage.Spec.SourceType != v1beta1.VirtualMachineImageSourceTypeExportVolume {
		return nil
	}

	if newImage.Spec.PVCNamespace == "" {
		return werror.NewInvalidError(`pvcNamespace is required when image source type is "export-from-volume"`, "spec.pvcNamespace")
	}
	if newImage.Spec.PVCName == "" {
		return werror.NewInvalidError(`pvcName is required when image source type is "export-from-volume"`, "spec.pvcName")
	}

	ssar, err := v.ssar.Create(request.Context, &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: newImage.Spec.PVCNamespace,
				Verb:      "get",
				Group:     "",
				Version:   "*",
				Resource:  "persistentvolumeclaims",
				Name:      newImage.Spec.PVCName,
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		message := fmt.Sprintf("failed to check user permission, error: %s", err.Error())
		return werror.NewInvalidError(message, "")
	}

	if !ssar.Status.Allowed || ssar.Status.Denied {
		message := fmt.Sprintf("user has no permission to get the pvc resource %s/%s", newImage.Spec.PVCName, newImage.Spec.PVCNamespace)
		return werror.NewInvalidError(message, "")
	}

	_, err = v.pvcCache.Get(newImage.Spec.PVCNamespace, newImage.Spec.PVCName)
	if err != nil {
		message := fmt.Sprintf("failed to get pvc %s/%s, error: %s", newImage.Spec.PVCName, newImage.Spec.PVCNamespace, err.Error())
		return werror.NewInvalidError(message, "")
	}

	return nil
}

func (v *virtualMachineImageValidator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	newImage := newObj.(*v1beta1.VirtualMachineImage)
	oldImage := oldObj.(*v1beta1.VirtualMachineImage)

	if !newImage.DeletionTimestamp.IsZero() {
		return nil
	}

	if !reflect.DeepEqual(newImage.Spec.StorageClassParameters, oldImage.Spec.StorageClassParameters) {
		return werror.NewInvalidError("storageClassParameters of the VM Image cannot be modified", "spec.storageClassParameters")
	}

	if oldImage.Spec.SourceType != newImage.Spec.SourceType {
		return werror.NewInvalidError("sourceType cannot be modified", "spec.sourceType")
	}

	if newImage.Spec.SourceType == v1beta1.VirtualMachineImageSourceTypeExportVolume {
		if oldImage.Spec.PVCNamespace != newImage.Spec.PVCNamespace {
			return werror.NewInvalidError("pvcNamespace cannot be modified", "spec.pvcNamespace")
		}
		if oldImage.Spec.PVCName != newImage.Spec.PVCName {
			return werror.NewInvalidError("pvcName cannot be modified", "spec.pvcName")
		}
	}

	if oldImage.Spec.URL != newImage.Spec.URL {
		return werror.NewInvalidError("url cannot be modified", "spec.url")
	}

	if !reflect.DeepEqual(oldImage.Spec.SecurityParameters, newImage.Spec.SecurityParameters) {
		return werror.NewInvalidError("securityParameters cannot be modified", "spec.securityParameters")
	}

	return v.CheckImageDisplayNameAndURL(newImage)
}

func (v *virtualMachineImageValidator) Delete(_ *types.Request, oldObj runtime.Object) error {
	image := oldObj.(*v1beta1.VirtualMachineImage)

	if image.Status.StorageClassName == "" {
		return nil
	}

	for _, ownerRef := range image.GetOwnerReferences() {
		if ownerRef.Kind == "VirtualMachineTemplateVersion" {
			_, err := v.vmTemplateVersionCache.Get(image.Namespace, ownerRef.Name)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return werror.NewInternalError(err.Error())
			}
			message := fmt.Sprintf("Cannot delete image %s/%s: being used by VMTemplateVersion %s", image.Namespace, image.Spec.DisplayName, ownerRef.Name)
			return werror.NewInvalidError(message, "")
		}
	}

	pvcs, err := v.pvcCache.List(corev1.NamespaceAll, labels.Everything())
	if err != nil {
		return err
	}

	for _, pvc := range pvcs {
		if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName == image.Status.StorageClassName {
			message := fmt.Sprintf("Cannot delete image %s/%s: being used by volume %s/%s", image.Namespace, image.Spec.DisplayName, pvc.Namespace, pvc.Name)
			return werror.NewInvalidError(message, "")
		}
	}

	vmBackups, err := v.vmBackupCache.GetByIndex(indexeres.VMBackupByStorageClassNameIndex, image.Status.StorageClassName)
	if err != nil {
		message := fmt.Sprintf("Failed to get VMBackups by storageClassName %s: %v", image.Status.StorageClassName, err)
		return werror.NewInternalError(message)
	}

	if len(vmBackups) > 0 {
		vmBackupNamespaceAndNames := []string{}
		for _, vmBackup := range vmBackups {
			vmBackupNamespaceAndNames = append(vmBackupNamespaceAndNames, fmt.Sprintf("%s/%s", vmBackup.Namespace, vmBackup.Name))
		}
		return werror.NewInvalidError(fmt.Sprintf("Cannot delete image %s/%s: being used by VMBackups %s", image.Namespace, image.Spec.DisplayName, strings.Join(vmBackupNamespaceAndNames, ",")), "")
	}
	return nil
}
