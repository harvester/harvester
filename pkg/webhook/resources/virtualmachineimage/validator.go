package virtualmachineimage

import (
	"fmt"
	"reflect"

	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
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
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	fieldDisplayName = "spec.displayName"
)

func NewValidator(
	vmimages ctlharvesterv1.VirtualMachineImageCache,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	ssar authorizationv1client.SelfSubjectAccessReviewInterface,
	vmTemplateVersionCache ctlharvesterv1.VirtualMachineTemplateVersionCache) types.Validator {
	return &virtualMachineImageValidator{
		vmimages:               vmimages,
		pvcCache:               pvcCache,
		ssar:                   ssar,
		vmTemplateVersionCache: vmTemplateVersionCache,
	}
}

type virtualMachineImageValidator struct {
	types.DefaultValidator

	vmimages               ctlharvesterv1.VirtualMachineImageCache
	pvcCache               ctlcorev1.PersistentVolumeClaimCache
	ssar                   authorizationv1client.SelfSubjectAccessReviewInterface
	vmTemplateVersionCache ctlharvesterv1.VirtualMachineTemplateVersionCache
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

	if newImage.Spec.SourceType == v1beta1.VirtualMachineImageSourceTypeDownload && newImage.Spec.URL == "" {
		return werror.NewInvalidError(`url is required when image source type is "download"`, "spec.url")
	}

	if newImage.Spec.SourceType != v1beta1.VirtualMachineImageSourceTypeDownload && newImage.Spec.URL != "" {
		return werror.NewInvalidError(`url should be empty when image source type is not "download"`, "spec.url")
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

func (v *virtualMachineImageValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
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

	return v.CheckImageDisplayNameAndURL(newImage)
}

func (v *virtualMachineImageValidator) Delete(request *types.Request, oldObj runtime.Object) error {
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

	return nil
}
