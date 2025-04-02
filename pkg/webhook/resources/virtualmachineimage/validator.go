package virtualmachineimage

import (
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	authorizationv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/backingimage"
	"github.com/harvester/harvester/pkg/image/cdi"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(
	vmiCache ctlharvesterv1.VirtualMachineImageCache,
	podCache ctlcorev1.PodCache,
	pvcCache ctlcorev1.PersistentVolumeClaimCache,
	ssar authorizationv1client.SelfSubjectAccessReviewInterface,
	vmTemplateVersionCache ctlharvesterv1.VirtualMachineTemplateVersionCache,
	scCache ctlstoragev1.StorageClassCache,
	vmBackupCache ctlharvesterv1.VirtualMachineBackupCache) types.Validator {

	vmiv := common.GetVMIValidator(vmiCache, scCache, ssar, podCache, pvcCache, vmTemplateVersionCache, vmBackupCache)
	validators := map[v1beta1.VMIBackend]backend.Validator{
		v1beta1.VMIBackendBackingImage: backingimage.GetValidator(vmiv),
		v1beta1.VMIBackendCDI:          cdi.GetValidator(vmiv),
	}

	return &virtualMachineImageValidator{
		validators: validators,
	}
}

type virtualMachineImageValidator struct {
	types.DefaultValidator
	validators map[v1beta1.VMIBackend]backend.Validator
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
	vmi := newObj.(*v1beta1.VirtualMachineImage)
	return v.validators[util.GetVMIBackend(vmi)].Create(request, vmi)
}

func (v *virtualMachineImageValidator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	newVMI := newObj.(*v1beta1.VirtualMachineImage)
	oldVMI := oldObj.(*v1beta1.VirtualMachineImage)
	return v.validators[util.GetVMIBackend(oldVMI)].Update(oldVMI, newVMI)
}

func (v *virtualMachineImageValidator) Delete(_ *types.Request, oldObj runtime.Object) error {
	vmi := oldObj.(*v1beta1.VirtualMachineImage)
	return v.validators[util.GetVMIBackend(vmi)].Delete(vmi)
}
