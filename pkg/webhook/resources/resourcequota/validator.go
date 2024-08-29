package resourcequota

import (
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator() types.Validator {
	return &resourceQuotaValidator{}
}

type resourceQuotaValidator struct {
	types.DefaultValidator
}

func (v *resourceQuotaValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.ResourceQuotaResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.ResourceQuota{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (v *resourceQuotaValidator) Create(_ *types.Request, newObj runtime.Object) error {
	resourceQuota := newObj.(*v1beta1.ResourceQuota)
	if resourceQuota == nil {
		return nil
	}

	// Now, only support "default-resource-quota" in each namespace
	if resourceQuota.Name != util.DefaultResourceQuotaName {
		return werror.NewInvalidError(fmt.Sprintf("name must be %s", util.DefaultResourceQuotaName), "name")
	}
	return checkSnapshotLimit(resourceQuota.Spec.SnapshotLimit)
}

func (v *resourceQuotaValidator) Update(_ *types.Request, _ runtime.Object, newObj runtime.Object) error {
	resourceQuota := newObj.(*v1beta1.ResourceQuota)
	if resourceQuota == nil {
		return nil
	}

	return checkSnapshotLimit(resourceQuota.Spec.SnapshotLimit)
}

func checkSnapshotLimit(snapshotLimit v1beta1.SnapshotLimit) error {
	for vmName, size := range snapshotLimit.VMTotalSnapshotSizeQuota {
		if size < 0 {
			return werror.NewInvalidError("must be greater than or equal to 0", fmt.Sprintf("spec.snapshotLimit.vmTotalSnapshotSizeQuota[%s]", vmName))
		}
	}
	return nil
}
