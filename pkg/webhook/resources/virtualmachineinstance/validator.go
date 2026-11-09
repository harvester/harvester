package virtualmachineinstance

import (
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(scCache ctlstoragev1.StorageClassCache) types.Validator {
	return &vmiValidator{
		scCache: scCache,
	}
}

type vmiValidator struct {
	types.DefaultValidator
	scCache ctlstoragev1.StorageClassCache
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
	if err := util.CheckVMStateStorageClass(vmi, v.scCache); err != nil {
		if util.IsVMStateStorageClassMissing(err) {
			return werror.NewInvalidError(err.Error(), "spec.domain")
		}
		return werror.NewInternalError(err.Error())
	}
	return nil
}
