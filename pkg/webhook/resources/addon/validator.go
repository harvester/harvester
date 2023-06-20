package addon

import (
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(addons ctlharvesterv1.AddonCache) types.Validator {
	return &addonValidator{
		addons: addons,
	}
}

type addonValidator struct {
	types.DefaultValidator

	addons ctlharvesterv1.AddonCache
}

func (v *addonValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.AddonResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.Addon{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

// Do not allow one addon to be created twice
func (v *addonValidator) Create(request *types.Request, newObj runtime.Object) error {
	newAddon := newObj.(*v1beta1.Addon)

	addons, err := v.addons.List(metav1.NamespaceAll, labels.Everything())
	if err != nil {
		return werror.NewInternalError(fmt.Sprintf("cannot list addons, err: %+v", err))
	}

	return validateNewAddon(newAddon, addons)
}

// Do not allow some fields to be changed, or set to non-existing values
func (v *addonValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	newAddon := newObj.(*v1beta1.Addon)
	oldAddon := oldObj.(*v1beta1.Addon)

	return validateUpdatedAddon(newAddon, oldAddon)
}

func validateNewAddon(newAddon *v1beta1.Addon, addonList []*v1beta1.Addon) error {
	for _, addon := range addonList {
		if addon.Spec.Chart == newAddon.Spec.Chart {
			return werror.NewConflict(fmt.Sprintf("addon with Chart %q has been created, cannot create a new one", addon.Spec.Chart))
		}
	}

	return nil
}

func validateUpdatedAddon(newAddon *v1beta1.Addon, oldAddon *v1beta1.Addon) error {
	if newAddon.Spec.Chart != oldAddon.Spec.Chart {
		return werror.NewBadRequest("chart field cannot be changed.")
	}

	if oldAddon.Status.Status == v1beta1.DisablingAddon && newAddon.Spec.Enabled {
		return werror.NewBadRequest("addon is currently disabling and cannot be enabled")
	}

	return nil
}
