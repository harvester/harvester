package resourcequota

import (
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	ctlharvcorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	harvesterLoadBalancerPath = "/spec/hard/\"count/count/loadbalancers.loadbalancer.harvesterhci.io\""
)

func NewMutator(resourceQuotaCache ctlharvcorev1.ResourceQuotaCache) types.Mutator {
	return &resourceQuotaMutator{
		resourceQuotaCache: resourceQuotaCache,
	}
}

// podMutator injects Harvester settings like http proxy envs and trusted CA certs to system pods that may access
// external services. It includes harvester apiserver and longhorn backing-image-data-source pods.
type resourceQuotaMutator struct {
	types.DefaultMutator
	resourceQuotaCache ctlharvcorev1.ResourceQuotaCache
}

func newResource(ops []admissionregv1.OperationType) types.Resource {
	return types.Resource{
		Names:          []string{"resourcequotas"},
		Scope:          admissionregv1.NamespacedScope,
		APIGroup:       corev1.SchemeGroupVersion.Group,
		APIVersion:     corev1.SchemeGroupVersion.Version,
		ObjectType:     &corev1.ResourceQuota{},
		OperationTypes: ops,
	}
}

func (m *resourceQuotaMutator) Resource() types.Resource {
	return newResource([]admissionregv1.OperationType{
		admissionregv1.Create,
		admissionregv1.Update,
	})
}

func (m *resourceQuotaMutator) Create(_ *types.Request, newObj runtime.Object) (types.PatchOps, error) {
	resourceQuota := newObj.(*corev1.ResourceQuota)

	return patchLoadBalancer(resourceQuota), nil
}

func (m *resourceQuotaMutator) Update(_ *types.Request, _ runtime.Object, newObj runtime.Object) (types.PatchOps, error) {
	resourceQuota := newObj.(*corev1.ResourceQuota)

	return patchLoadBalancer(resourceQuota), nil
}

func patchLoadBalancer(resourceQuota *corev1.ResourceQuota) types.PatchOps {
	if resourceQuota == nil || resourceQuota.Spec.Hard == nil {
		return nil
	}

	serviceLoadBalancerQuota, ok := resourceQuota.Spec.Hard[corev1.ResourceServicesLoadBalancers]
	if !ok {
		return nil
	}

	_, ok = resourceQuota.Spec.Hard["count/loadbalancers.loadbalancer.harvesterhci.io"]
	if !ok {
		return []string{fmt.Sprintf(`{"op": "add", "path": "%s", "value": %s}`, harvesterLoadBalancerPath, &serviceLoadBalancerQuota)}
	}
	return []string{fmt.Sprintf(`{"op": "replace", "path": "%s", "value": %s}`, harvesterLoadBalancerPath, &serviceLoadBalancerQuota)}
}
