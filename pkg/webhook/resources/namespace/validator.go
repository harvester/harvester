package namespace

import (
	"fmt"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	harvestercorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	"github.com/harvester/harvester/pkg/util"
	resourcequotautil "github.com/harvester/harvester/pkg/util/resourcequota"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(resourceQuotaCache harvestercorev1.ResourceQuotaCache) types.Validator {
	return &namespaceValidator{
		resourceQuotaCache: resourceQuotaCache,
	}
}

type namespaceValidator struct {
	types.DefaultValidator
	resourceQuotaCache harvestercorev1.ResourceQuotaCache
}

func (v *namespaceValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"namespaces"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   corev1.SchemeGroupVersion.Group,
		APIVersion: corev1.SchemeGroupVersion.Version,
		ObjectType: &corev1.Namespace{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Update,
		},
	}
}

func (v *namespaceValidator) Update(_ *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	oldNamespace := oldObj.(*corev1.Namespace)
	newNamespace := newObj.(*corev1.Namespace)

	rqOld := oldNamespace.Annotations[util.CattleAnnotationResourceQuota]
	rqNew := newNamespace.Annotations[util.CattleAnnotationResourceQuota]
	// if no change, skip
	if rqNew == "" || (rqOld == rqNew) {
		return nil
	}

	selector := labels.Set{util.LabelManagementDefaultResourceQuota: "true"}.AsSelector()
	rss, err := v.resourceQuotaCache.List(oldNamespace.Name, selector)
	if err != nil {
		return err
	} else if len(rss) == 0 {
		logrus.Debugf("can not found any default ResourceQuota, skip updating namespace %s", newNamespace.Name)
		return nil
	}

	if resourcequotautil.HasMigratingVM(rss[0]) {
		return fmt.Errorf("namespace %s has migrating VMs, so you can't change resource quotas", newNamespace.Name)
	}
	return nil
}
