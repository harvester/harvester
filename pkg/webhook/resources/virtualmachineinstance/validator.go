package virtualmachineinstance

import (
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	vmstate "github.com/harvester/harvester/pkg/util/vmstate"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(scCache ctlstoragev1.StorageClassCache, pvcCache ctlcorev1.PersistentVolumeClaimCache) types.Validator {
	return &vmiValidator{
		scCache:  scCache,
		pvcCache: pvcCache,
	}
}

type vmiValidator struct {
	types.DefaultValidator
	scCache  ctlstoragev1.StorageClassCache
	pvcCache ctlcorev1.PersistentVolumeClaimCache
}

func (v *vmiValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"virtualmachineinstances"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   kubevirtv1.SchemeGroupVersion.Group,
		APIVersion: kubevirtv1.SchemeGroupVersion.Version,
		ObjectType: &kubevirtv1.VirtualMachineInstance{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}

func (v *vmiValidator) Create(_ *types.Request, newObj runtime.Object) error {
	vmi := newObj.(*kubevirtv1.VirtualMachineInstance)
	return v.validateVMStateStorageClass(vmi)
}

func (v *vmiValidator) validateVMStateStorageClass(vmi *kubevirtv1.VirtualMachineInstance) error {
	if err := vmstate.CheckVMStateStorageClass(vmi, v.scCache, v.pvcCache); err != nil {
		if vmstate.IsVMStateStorageClassMissing(err) {
			return werror.NewInvalidError(err.Error(), "spec.domain")
		}
		return werror.NewInternalError(err.Error())
	}
	return nil
}
