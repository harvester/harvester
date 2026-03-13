package ippoolusage

import (
	"fmt"
	"net"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator() types.Validator {
	return &validator{}
}

type validator struct {
	types.DefaultValidator
}

func (v *validator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{harvesterv1beta1.IPPoolUsageResourceName},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   harvesterv1beta1.SchemeGroupVersion.Group,
		APIVersion: harvesterv1beta1.SchemeGroupVersion.Version,
		ObjectType: &harvesterv1beta1.IPPoolUsage{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (v *validator) Create(_ *types.Request, newObj runtime.Object) error {
	pool := newObj.(*harvesterv1beta1.IPPoolUsage)
	return validateSpec(pool)
}

func (v *validator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	oldPool := oldObj.(*harvesterv1beta1.IPPoolUsage)
	newPool := newObj.(*harvesterv1beta1.IPPoolUsage)

	if err := validateSpec(newPool); err != nil {
		return err
	}
	if oldPool.Spec.CIDR != newPool.Spec.CIDR {
		return werror.NewInvalidError("spec.cidr is immutable", "spec.cidr")
	}
	if effectiveReservedIPCount(newPool.Spec.ReservedIPCount) > effectiveReservedIPCount(oldPool.Spec.ReservedIPCount) {
		return werror.NewInvalidError("spec.reservedIPCount cannot be increased", "spec.reservedIPCount")
	}

	return nil
}

func validateSpec(pool *harvesterv1beta1.IPPoolUsage) error {
	if pool == nil {
		return nil
	}
	if _, _, err := net.ParseCIDR(pool.Spec.CIDR); err != nil {
		return werror.NewInvalidError(fmt.Sprintf("invalid spec.cidr %q", pool.Spec.CIDR), "spec.cidr")
	}
	if pool.Spec.ReservedIPCount < 0 {
		return werror.NewInvalidError("spec.reservedIPCount cannot be negative", "spec.reservedIPCount")
	}
	if effectiveReservedIPCount(pool.Spec.ReservedIPCount) < 0 {
		return werror.NewInvalidError(
			"spec.reservedIPCount must be greater than or equal to 0",
			"spec.reservedIPCount",
		)
	}
	return nil
}

func effectiveReservedIPCount(count int) int {
	if count == 0 {
		return util.IPPoolUsageDefaultReservedIPCount
	}
	return count
}
