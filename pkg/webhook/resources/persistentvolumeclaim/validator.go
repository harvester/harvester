package persistentvolumeclaim

import (
	"fmt"
	"strings"

	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlkv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(pvcCache v1.PersistentVolumeClaimCache, vmCache ctlkv1.VirtualMachineCache) types.Validator {
	return &pvcValidator{
		pvcCache: pvcCache,
		vmCache:  vmCache,
	}
}

type pvcValidator struct {
	types.DefaultValidator
	pvcCache v1.PersistentVolumeClaimCache
	vmCache  ctlkv1.VirtualMachineCache
}

func (v *pvcValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"persistentvolumeclaims"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   corev1.SchemeGroupVersion.Group,
		APIVersion: corev1.SchemeGroupVersion.Version,
		ObjectType: &corev1.PersistentVolumeClaim{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Delete,
			admissionregv1.Update,
		},
	}
}

func (v *pvcValidator) Delete(request *types.Request, oldObj runtime.Object) error {
	if request.IsGarbageCollection() {
		return nil
	}

	oldPVC := oldObj.(*corev1.PersistentVolumeClaim)

	pvc, err := v.pvcCache.Get(oldPVC.Namespace, oldPVC.Name)
	if err != nil {
		return werror.NewInvalidError(err.Error(), "metadata.name")
	}

	annotationSchemaOwners, err := ref.GetSchemaOwnersFromAnnotation(pvc)
	if err != nil {
		return fmt.Errorf("failed to get schema owners from annotation: %v", err)
	}

	attachedList := annotationSchemaOwners.List(kubevirtv1.VirtualMachineGroupVersionKind.GroupKind())
	if len(attachedList) != 0 {
		message := fmt.Sprintf("can not delete the volume %s which is currently attached to VMs: %s", oldPVC.Name, strings.Join(attachedList, ", "))
		return werror.NewInvalidError(message, "")
	}

	if len(pvc.OwnerReferences) == 0 {
		return nil
	}

	var ownerList []string
	for _, owner := range pvc.OwnerReferences {
		if owner.Kind == kubevirtv1.VirtualMachineGroupVersionKind.Kind {
			ownerList = append(ownerList, owner.Name)
		}
	}
	if len(ownerList) > 0 {
		message := fmt.Sprintf("can not delete the volume %s which is currently owned by these VMs: %s", oldPVC.Name, strings.Join(ownerList, ","))
		return werror.NewInvalidError(message, "")
	}

	return nil
}

func (v *pvcValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	oldPVC := oldObj.(*corev1.PersistentVolumeClaim)
	newPVC := newObj.(*corev1.PersistentVolumeClaim)

	newQuantity := newPVC.Spec.Resources.Requests.Storage()
	oldQuantity := oldPVC.Spec.Resources.Requests.Storage()
	if oldQuantity.Cmp(*newQuantity) == 0 {
		return nil
	}

	// Validation for offline resizing
	annotationSchemaOwners, err := ref.GetSchemaOwnersFromAnnotation(oldPVC)
	if err != nil {
		return fmt.Errorf("failed to get schema owners from annotation: %v", err)
	}

	attachedList := annotationSchemaOwners.List(kubevirtv1.VirtualMachineGroupVersionKind.GroupKind())
	for _, vmID := range attachedList {
		ns, name := ref.Parse(vmID)
		vm, err := v.vmCache.Get(ns, name)
		if err != nil {
			return fmt.Errorf("failed to get VM: %v", err)
		}
		if vm.Status.PrintableStatus != kubevirtv1.VirtualMachineStatusProvisioning && vm.Status.PrintableStatus != kubevirtv1.VirtualMachineStatusStopped {
			message := fmt.Sprintf("resizing is only supported for detached volumes. The volume is being used by VM %s. Please stop the VM first.", vmID)
			return werror.NewInvalidError(message, "")
		}
	}

	return nil
}
