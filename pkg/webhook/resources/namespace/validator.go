package namespace

import (
	"encoding/json"
	"fmt"

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	harvestercorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	"github.com/harvester/harvester/pkg/util"
	rqutils "github.com/harvester/harvester/pkg/util/resourcequota"
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
		logrus.Debugf("can't find any default ResourceQuota, skip updating namespace %s", newNamespace.Name)
		return nil
	}

	if rqutils.HasMigratingVM(rss[0]) {
		return fmt.Errorf("namespace %s has migrating VMs, can't change resource quotas", newNamespace.Name)
	}

	if err := v.checkIfNewResourceQuotaIsSufficient(rss[0], rqNew); err != nil {
		return fmt.Errorf("can't update the resource quota of namespace %s, error: %w",
			newNamespace.Name,
			err)
	}

	return nil
}

// Check if used resource quota is larger than new resource quota
func (v *namespaceValidator) checkIfNewResourceQuotaIsSufficient(rq *corev1.ResourceQuota, nrqStr string) error {
	var nrq *v3.NamespaceResourceQuota
	if err := json.Unmarshal([]byte(nrqStr), &nrq); err != nil {
		return fmt.Errorf("invalid NamespaceResourceQuota %s, error: %w", nrqStr, err)
	}

	if nrq.Limit.LimitsCPU == "" && nrq.Limit.LimitsMemory == "" {
		logrus.Debugf("namespace %s resource quota has no limit, skip checking", rq.Namespace)
		return nil
	}

	usedCPU := rq.Status.Used.Name(corev1.ResourceLimitsCPU, resource.DecimalSI)
	usedMem := rq.Status.Used.Name(corev1.ResourceLimitsMemory, resource.BinarySI)

	if nrq.Limit.LimitsCPU != "" {
		newCPU, err := resource.ParseQuantity(nrq.Limit.LimitsCPU)
		if err != nil {
			return fmt.Errorf("invalid LimitsCPU %s, error: %w", nrq.Limit.LimitsCPU, err)
		}
		if usedCPU.Cmp(newCPU) == 1 {
			return fmt.Errorf("new CPU limit %s is lower than the current used CPU limit %s",
				newCPU.String(),
				usedCPU.String())
		}
	}

	if nrq.Limit.LimitsMemory != "" {
		newMem, err := resource.ParseQuantity(nrq.Limit.LimitsMemory)
		if err != nil {
			return fmt.Errorf("invalid LimitsMemory %s, error: %w", nrq.Limit.LimitsMemory, err)
		}
		if usedMem.Cmp(newMem) == 1 {
			return fmt.Errorf("new Memory limit %s is lower than the current used Memory limit %s",
				newMem.String(),
				usedMem.String())
		}
	}

	return nil
}
