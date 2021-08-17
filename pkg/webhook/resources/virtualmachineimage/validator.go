package virtualmachineimage

import (
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	fieldDisplayName = "spec.displayName"
)

func NewValidator(vmimages ctlharvesterv1.VirtualMachineImageCache, pvcCache ctlcorev1.PersistentVolumeClaimCache) types.Validator {
	return &virtualMachineImageValidator{
		vmimages: vmimages,
		pvcCache: pvcCache,
	}
}

type virtualMachineImageValidator struct {
	types.DefaultValidator

	vmimages ctlharvesterv1.VirtualMachineImageCache
	pvcCache ctlcorev1.PersistentVolumeClaimCache
}

func (v *virtualMachineImageValidator) Resource() types.Resource {
	return types.Resource{
		Name:       v1beta1.VirtualMachineImageResourceName,
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

	if newImage.Spec.DisplayName == "" {
		return werror.NewInvalidError("displayName is required", fieldDisplayName)
	}

	allImages, err := v.vmimages.List(newImage.Namespace, labels.Everything())
	if err != nil {
		return err
	}
	for _, image := range allImages {
		if newImage.Name == image.Name {
			continue
		}
		if newImage.Spec.DisplayName == image.Spec.DisplayName {
			return werror.NewConflict("A resource with the same name exists")
		}
	}

	if newImage.Spec.SourceType == v1beta1.VirtualMachineImageSourceTypeDownload && newImage.Spec.URL == "" {
		return werror.NewInvalidError(`url is required when image source type is "download"`, "spec.url")
	} else if newImage.Spec.SourceType == v1beta1.VirtualMachineImageSourceTypeUpload && newImage.Spec.URL != "" {
		return werror.NewInvalidError(`url should be empty when image source type is "upload"`, "spec.url")
	}

	return nil
}

func (v *virtualMachineImageValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	return v.Create(request, newObj)
}

func (v *virtualMachineImageValidator) Delete(request *types.Request, oldObj runtime.Object) error {
	image := oldObj.(*v1beta1.VirtualMachineImage)

	if image.Status.StorageClassName == "" {
		return nil
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
