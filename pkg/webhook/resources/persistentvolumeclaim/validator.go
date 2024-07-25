package persistentvolumeclaim

import (
	"fmt"
	"strings"

	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlkv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctllonghornv1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta2"
	"github.com/harvester/harvester/pkg/ref"
	indexeresutil "github.com/harvester/harvester/pkg/util/indexeres"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/indexeres"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(pvcCache v1.PersistentVolumeClaimCache,
	vmCache ctlkv1.VirtualMachineCache,
	imageCache ctlharvesterv1.VirtualMachineImageCache) types.Validator {
	return &pvcValidator{
		pvcCache:   pvcCache,
		vmCache:    vmCache,
		imageCache: imageCache,
	}
}

type pvcValidator struct {
	types.DefaultValidator
	pvcCache    v1.PersistentVolumeClaimCache
	vmCache     ctlkv1.VirtualMachineCache
	imageCache  ctlharvesterv1.VirtualMachineImageCache
	volumeCache ctllonghornv1.VolumeCache
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

	images, err := v.imageCache.GetByIndex(indexeres.ImageByExportSourcePVCIndex,
		fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name))
	if err != nil {
		return werror.NewInvalidError(err.Error(), "value")
	}

	for _, image := range images {
		if !harvesterv1beta1.ImageImported.IsTrue(image) {
			message := fmt.Sprintf("can not delete volume %s which is exporting for image %s/%s",
				pvc.Name, image.Namespace, image.Name)
			return werror.NewInvalidError(message, "value")
		}
	}

	vms, err := v.vmCache.GetByIndex(indexeresutil.VMByPVCIndex, ref.Construct(oldPVC.Namespace, oldPVC.Name))
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get VMs by index: %s, PVC: %s/%s, err: %s", indexeresutil.VMByPVCIndex, oldPVC.Namespace, oldPVC.Name, err))
	}

	for _, vm := range vms {
		if vm.DeletionTimestamp == nil {
			message := fmt.Sprintf("can not delete the volume %s which is currently attached to VM: %s/%s", oldPVC.Name, vm.Namespace, vm.Name)
			return werror.NewInvalidError(message, "")
		}
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

func (v *pvcValidator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	oldPVC := oldObj.(*corev1.PersistentVolumeClaim)
	newPVC := newObj.(*corev1.PersistentVolumeClaim)

	newQuantity := newPVC.Spec.Resources.Requests.Storage()
	oldQuantity := oldPVC.Spec.Resources.Requests.Storage()
	if oldQuantity.Cmp(*newQuantity) == 0 {
		return nil
	}

	vms, err := v.vmCache.GetByIndex(indexeresutil.VMByPVCIndex, ref.Construct(newPVC.Namespace, newPVC.Name))
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("failed to get VMs by index: %s, PVC: %s/%s, err: %s", indexeresutil.VMByPVCIndex, newPVC.Namespace, newPVC.Name, err))
	}
	for _, vm := range vms {
		if vm.Status.PrintableStatus != kubevirtv1.VirtualMachineStatusProvisioning && vm.Status.PrintableStatus != kubevirtv1.VirtualMachineStatusStopped {
			message := fmt.Sprintf("resizing is only supported for detached volumes. The volume is being used by VM %s/%s. Please stop the VM first.", vm.Namespace, vm.Name)
			return werror.NewInvalidError(message, "")
		}
	}
	return nil
}
