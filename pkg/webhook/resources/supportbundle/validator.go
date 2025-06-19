package supportbundle

import (
	"fmt"
	"reflect"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(namespaceCache v1.NamespaceCache) types.Validator {
	return &supportBundleValidator{
		namespaceCache: namespaceCache,
	}
}

type supportBundleValidator struct {
	types.DefaultValidator
	namespaceCache v1.NamespaceCache
}

func (v *supportBundleValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{v1beta1.SupportBundleResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   v1beta1.SchemeGroupVersion.Group,
		APIVersion: v1beta1.SchemeGroupVersion.Version,
		ObjectType: &v1beta1.SupportBundle{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (v *supportBundleValidator) Update(_ *types.Request, oldObj, newObj runtime.Object) error {
	oldSB := oldObj.(*v1beta1.SupportBundle)
	newSB := newObj.(*v1beta1.SupportBundle)

	if !reflect.DeepEqual(oldSB.Spec, newSB.Spec) { // Check if the spec has changed
		return werror.NewBadRequest("modifying the spec of an existing support bundle is not allowed")
	}

	return nil
}

func (v *supportBundleValidator) Create(_ *types.Request, obj runtime.Object) error {
	supportBundle := obj.(*v1beta1.SupportBundle)

	return v.checkExtraCollectionNamespaces(supportBundle)
}

func (v *supportBundleValidator) checkExtraCollectionNamespaces(supportBundle *v1beta1.SupportBundle) error {
	// Check if all namespaces in ExtraCollectionNamespaces exist
	for _, namespace := range supportBundle.Spec.ExtraCollectionNamespaces {
		if namespace == "" {
			continue // Skip empty namespace names
		}

		_, err := v.namespaceCache.Get(namespace)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return werror.NewBadRequest(fmt.Sprintf("namespace %s not found", namespace))
			}
			return werror.NewInternalError(fmt.Sprintf("failed to check namespace %s: %v", namespace, err))
		}
	}

	return nil
}
