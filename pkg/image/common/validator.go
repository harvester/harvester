package common

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

type VMIValidator interface {
	GetStatusSC(vmi *v1beta1.VirtualMachineImage) string

	CheckDisplayName(vmi *v1beta1.VirtualMachineImage) error
	CheckUpdateDisplayName(oldVMI, newVMI *v1beta1.VirtualMachineImage) error
	CheckURL(vmi *v1beta1.VirtualMachineImage) error
	CheckSecurityParameters(vmi *v1beta1.VirtualMachineImage) error
	CheckImagePVC(request *types.Request, vmi *v1beta1.VirtualMachineImage) error
	CheckPVCInUse(vmi *v1beta1.VirtualMachineImage) error

	IsExportVolume(vmi *v1beta1.VirtualMachineImage) bool

	SCConsistency(oldVMI, newVMI *v1beta1.VirtualMachineImage) error
	SCParametersConsistency(oldVMI, newVMI *v1beta1.VirtualMachineImage) error
	SourceTypeConsistency(oldVMI, newVMI *v1beta1.VirtualMachineImage) error
	PVCConsistency(oldVMI, newVMI *v1beta1.VirtualMachineImage) error
	URLConsistency(oldVMI, newVMI *v1beta1.VirtualMachineImage) error
	SecurityParameterConsistency(oldVMI, newVMI *v1beta1.VirtualMachineImage) error

	VMTemplateVersionOccupation(vmi *v1beta1.VirtualMachineImage) error
	PVCOccupation(vmi *v1beta1.VirtualMachineImage) error
	VMBackupOccupation(vmi *v1beta1.VirtualMachineImage) error
}

type vmiValidator struct {
	vmiCache               ctlharvesterv1.VirtualMachineImageCache
	scCache                ctlstoragev1.StorageClassCache
	ssar                   authorizationv1client.SelfSubjectAccessReviewInterface
	podCache               ctlcorev1.PodCache
	pvcCache               ctlcorev1.PersistentVolumeClaimCache
	vmTemplateVersionCache ctlharvesterv1.VirtualMachineTemplateVersionCache
	vmBackupCache          ctlharvesterv1.VirtualMachineBackupCache
}

func GetVMIValidator(vmiCache ctlharvesterv1.VirtualMachineImageCache,
	scCache ctlstoragev1.StorageClassCache,
	ssar authorizationv1client.SelfSubjectAccessReviewInterface,
	podCache ctlcorev1.PodCache,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	vmTemplateVersionCache ctlharvesterv1.VirtualMachineTemplateVersionCache,
	vmBackupCache ctlharvesterv1.VirtualMachineBackupCache) VMIValidator {
	vmiv := &vmiValidator{
		vmiCache:               vmiCache,
		scCache:                scCache,
		ssar:                   ssar,
		podCache:               podCache,
		pvcCache:               pvcCache,
		vmTemplateVersionCache: vmTemplateVersionCache,
		vmBackupCache:          vmBackupCache,
	}
	vmiv.podCache.AddIndexer(util.IndexPodByPVC, util.IndexPodByPVCFunc)
	return vmiv
}

func (v *vmiValidator) GetStatusSC(vmi *v1beta1.VirtualMachineImage) string {
	return vmi.Status.StorageClassName
}

func (v *vmiValidator) CheckDisplayName(vmi *v1beta1.VirtualMachineImage) error {
	if vmi.Spec.DisplayName == "" {
		return werror.NewInvalidError("displayName is required", fieldDisplayName)
	}

	sameDisplayNameImages, err := v.vmiCache.List(vmi.Namespace, labels.SelectorFromSet(map[string]string{
		util.LabelImageDisplayName: vmi.Spec.DisplayName,
	}))
	if err != nil {
		return err
	}
	for _, image := range sameDisplayNameImages {
		if vmi.UID == image.UID {
			continue
		}
		return werror.NewConflict("A resource with the same name exists")
	}

	return nil
}

func (v *vmiValidator) CheckUpdateDisplayName(oldVMI, newVMI *v1beta1.VirtualMachineImage) error {
	if oldVMI.Spec.DisplayName != newVMI.Spec.DisplayName {
		return werror.NewInvalidError("displayName cannot be modified", fieldDisplayName)
	}
	return v.CheckDisplayName(newVMI)
}

func (v *vmiValidator) CheckURL(vmi *v1beta1.VirtualMachineImage) error {
	shouldHaveURL := false
	if vmi.Spec.SourceType == v1beta1.VirtualMachineImageSourceTypeDownload || vmi.Spec.SourceType == v1beta1.VirtualMachineImageSourceTypeRestore {
		shouldHaveURL = true
	}

	if shouldHaveURL {
		if vmi.Spec.URL == "" {
			return werror.NewInvalidError("url is required", "spec.url")
		}

		if _, err := url.Parse(vmi.Spec.URL); err != nil {
			return werror.NewInvalidError(fmt.Sprintf("url is invalid: %s", err.Error()), "spec.url")
		}
		return nil
	}

	// means !shouldHaveURL
	if vmi.Spec.URL != "" {
		return werror.NewInvalidError(fmt.Sprintf(`url should be empty when image source type is "%s"`, vmi.Spec.SourceType), "spec.url")
	}
	return nil
}

func (v *vmiValidator) CheckSecurityParameters(vmi *v1beta1.VirtualMachineImage) error {
	if vmi.Spec.SourceType != v1beta1.VirtualMachineImageSourceTypeClone {
		return nil
	}

	sp := vmi.Spec.SecurityParameters
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
	sourceImage, err := v.vmiCache.Get(sp.SourceImageNamespace, sp.SourceImageName)
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
	scName, ok := vmi.Annotations[util.AnnotationStorageClassName]
	if !ok {
		return werror.NewInvalidError("storageClassName is required when using security params", fmt.Sprintf("metadata.annotations[%s]", util.AnnotationStorageClassName))
	}

	sc, err := v.scCache.Get(scName)
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

// CheckPVCInUse checks if the PVC is in use by any pods, this is only used to CDI backend
func (v *vmiValidator) CheckPVCInUse(vmi *v1beta1.VirtualMachineImage) error {
	if vmi.Spec.Backend != v1beta1.VMIBackendCDI {
		return nil
	}
	index := fmt.Sprintf("%s-%s", vmi.Spec.PVCNamespace, vmi.Spec.PVCName)
	if pods, err := v.podCache.GetByIndex(util.IndexPodByPVC, index); err == nil && len(pods) > 0 {
		podList := []string{}
		for _, pod := range pods {
			indexedPod := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
			podList = append(podList, indexedPod)
		}
		return fmt.Errorf("PVC %s is used by Pods %v, cannot export volume when it's running", vmi.Spec.PVCName, podList)
	}
	return nil
}

func (v *vmiValidator) CheckImagePVC(request *types.Request, vmi *v1beta1.VirtualMachineImage) error {
	if vmi.Spec.SourceType != v1beta1.VirtualMachineImageSourceTypeExportVolume {
		return nil
	}

	if vmi.Spec.PVCNamespace == "" {
		return werror.NewInvalidError(`pvcNamespace is required when image source type is "export-from-volume"`, "spec.pvcNamespace")
	}
	if vmi.Spec.PVCName == "" {
		return werror.NewInvalidError(`pvcName is required when image source type is "export-from-volume"`, "spec.pvcName")
	}

	ssar, err := v.ssar.Create(request.Context, &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: vmi.Spec.PVCNamespace,
				Verb:      "get",
				Group:     "",
				Version:   "*",
				Resource:  "persistentvolumeclaims",
				Name:      vmi.Spec.PVCName,
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		message := fmt.Sprintf("failed to check user permission, error: %s", err.Error())
		return werror.NewInvalidError(message, "")
	}

	if !ssar.Status.Allowed || ssar.Status.Denied {
		message := fmt.Sprintf("user has no permission to get the pvc resource %s/%s", vmi.Spec.PVCName, vmi.Spec.PVCNamespace)
		return werror.NewInvalidError(message, "")
	}

	_, err = v.pvcCache.Get(vmi.Spec.PVCNamespace, vmi.Spec.PVCName)
	if err != nil {
		message := fmt.Sprintf("failed to get pvc %s/%s, error: %s", vmi.Spec.PVCName, vmi.Spec.PVCNamespace, err.Error())
		return werror.NewInvalidError(message, "")
	}

	return nil
}

func (v *vmiValidator) IsExportVolume(vmi *v1beta1.VirtualMachineImage) bool {
	return vmi.Spec.SourceType == v1beta1.VirtualMachineImageSourceTypeExportVolume
}

func (v *vmiValidator) SCParametersConsistency(oldVMI, newVMI *v1beta1.VirtualMachineImage) error {
	if !reflect.DeepEqual(oldVMI.Spec.StorageClassParameters, newVMI.Spec.StorageClassParameters) {
		return werror.NewInvalidError("storageClassParameters of the VM Image cannot be modified", "spec.storageClassParameters")
	}
	return nil
}

func (v *vmiValidator) SourceTypeConsistency(oldVMI, newVMI *v1beta1.VirtualMachineImage) error {
	if oldVMI.Spec.SourceType != newVMI.Spec.SourceType {
		return werror.NewInvalidError("sourceType cannot be modified", "spec.sourceType")
	}
	return nil
}

func (v *vmiValidator) PVCConsistency(oldVMI, newVMI *v1beta1.VirtualMachineImage) error {
	if oldVMI.Spec.PVCNamespace != newVMI.Spec.PVCNamespace {
		return werror.NewInvalidError("pvcNamespace cannot be modified", "spec.pvcNamespace")
	}

	if oldVMI.Spec.PVCName != newVMI.Spec.PVCName {
		return werror.NewInvalidError("pvcName cannot be modified", "spec.pvcName")
	}

	return nil
}

func (v *vmiValidator) URLConsistency(oldVMI, newVMI *v1beta1.VirtualMachineImage) error {
	if oldVMI.Spec.URL != newVMI.Spec.URL {
		return werror.NewInvalidError("url cannot be modified", "spec.url")
	}
	return nil
}

func (v *vmiValidator) SecurityParameterConsistency(oldVMI, newVMI *v1beta1.VirtualMachineImage) error {
	if !reflect.DeepEqual(oldVMI.Spec.SecurityParameters, newVMI.Spec.SecurityParameters) {
		return werror.NewInvalidError("securityParameters cannot be modified", "spec.securityParameters")
	}
	return nil
}

func (v *vmiValidator) VMTemplateVersionOccupation(vmi *v1beta1.VirtualMachineImage) error {
	for _, ownerRef := range vmi.GetOwnerReferences() {
		if ownerRef.Kind == "VirtualMachineTemplateVersion" {
			_, err := v.vmTemplateVersionCache.Get(vmi.Namespace, ownerRef.Name)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return werror.NewInternalError(err.Error())
			}
			message := fmt.Sprintf("Cannot delete image %s/%s: being used by VMTemplateVersion %s", vmi.Namespace, vmi.Spec.DisplayName, ownerRef.Name)
			return werror.NewInvalidError(message, "")
		}
	}
	return nil
}

func (v *vmiValidator) PVCOccupation(vmi *v1beta1.VirtualMachineImage) error {
	pvcs, err := v.pvcCache.List(corev1.NamespaceAll, labels.Everything())
	if err != nil {
		return err
	}

	for _, pvc := range pvcs {
		if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName == vmi.Status.StorageClassName {
			message := fmt.Sprintf("Cannot delete image %s/%s: being used by volume %s/%s", vmi.Namespace, vmi.Spec.DisplayName, pvc.Namespace, pvc.Name)
			return werror.NewInvalidError(message, "")
		}
	}
	return nil
}

func (v *vmiValidator) VMBackupOccupation(vmi *v1beta1.VirtualMachineImage) error {
	vmBackups, err := v.vmBackupCache.GetByIndex(indexeres.VMBackupByStorageClassNameIndex, vmi.Status.StorageClassName)
	if err != nil {
		message := fmt.Sprintf("Failed to get VMBackups by storageClassName %s: %v", vmi.Status.StorageClassName, err)
		return werror.NewInternalError(message)
	}

	if len(vmBackups) > 0 {
		vmBackupNamespaceAndNames := []string{}
		for _, vmBackup := range vmBackups {
			vmBackupNamespaceAndNames = append(vmBackupNamespaceAndNames, fmt.Sprintf("%s/%s", vmBackup.Namespace, vmBackup.Name))
		}
		return werror.NewInvalidError(fmt.Sprintf("Cannot delete image %s/%s: being used by VMBackups %s", vmi.Namespace, vmi.Spec.DisplayName, strings.Join(vmBackupNamespaceAndNames, ",")), "")
	}
	return nil
}

func scConsistencyCreation(newVMI *v1beta1.VirtualMachineImage) error {
	scInAnnotations := ""
	if v, find := newVMI.Annotations[util.AnnotationStorageClassName]; find {
		scInAnnotations = v
	}
	if scInAnnotations != "" && newVMI.Spec.TargetStorageClassName != "" && scInAnnotations != newVMI.Spec.TargetStorageClassName {
		return werror.NewInvalidError("storageClassName should keep the consistency", "spec.targetStorageClassName")
	}

	return nil
}

func (v *vmiValidator) SCConsistency(oldVMI, newVMI *v1beta1.VirtualMachineImage) error {
	if oldVMI == nil {
		return scConsistencyCreation(newVMI)
	}
	oldSCInAnnotations := ""
	if v, find := oldVMI.Annotations[util.AnnotationStorageClassName]; find {
		oldSCInAnnotations = v
	}

	// we should leave the capability to add the StorageClassName if previously it was empty
	if oldSCInAnnotations == "" && oldVMI.Spec.TargetStorageClassName == "" {
		return nil
	}

	if oldSCInAnnotations != "" {
		if v, find := newVMI.Annotations[util.AnnotationStorageClassName]; find {
			if oldSCInAnnotations != v {
				return werror.NewInvalidError("storageClassName cannot be modified", "metadata.annotations[harvesterhci.io/storageClassName]")
			}
		} else {
			return werror.NewInvalidError("storageClassName cannot be modified", "metadata.annotations[harvesterhci.io/storageClassName]")
		}
	}

	if oldVMI.Spec.TargetStorageClassName != "" {
		if oldVMI.Spec.TargetStorageClassName != newVMI.Spec.TargetStorageClassName {
			return werror.NewInvalidError("storageClassName cannot be modified", "spec.targetStorageClassName")
		}
	}

	// Only check consistency if these two fields are not empty
	if oldSCInAnnotations != "" && newVMI.Spec.TargetStorageClassName != "" && oldSCInAnnotations != newVMI.Spec.TargetStorageClassName {
		return werror.NewInvalidError("storageClassName cannot be modified", "spec.targetStorageClassName")
	}
	return nil
}
