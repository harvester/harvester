package upgrade

import (
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	stateUpgrading    = "Upgrading"
	upgradeStateLabel = "harvesterhci.io/upgradeState"
)

func NewValidator(upgrades ctlharvesterv1.UpgradeCache) types.Validator {
	return &upgradeValidator{
		upgrades: upgrades,
	}
}

type upgradeValidator struct {
	types.DefaultValidator

	upgrades ctlharvesterv1.UpgradeCache
}

func (v *upgradeValidator) Resource() types.Resource {
	return types.Resource{
		Name:       v1beta1.UpgradeResourceName,
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.Upgrade{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}

func (v *upgradeValidator) Create(request *types.Request, newObj runtime.Object) error {
	newUpgrade := newObj.(*v1beta1.Upgrade)

	sets := labels.Set{
		upgradeStateLabel: stateUpgrading,
	}
	upgrades, err := v.upgrades.List(newUpgrade.Namespace, sets.AsSelector())
	if err != nil {
		return err
	}
	if len(upgrades) > 0 {
		msg := fmt.Sprintf("cannot proceed until previous upgrade %q completes", upgrades[0].Name)
		return werror.NewConflict(msg)
	}

	return nil
}
