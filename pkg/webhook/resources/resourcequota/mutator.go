package resourcequota

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	ctlharvcorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	resourceQuotaHard         = "/spec/hard"
	harvesterLoadBalancersKey = "count/loadbalancers.loadbalancer.harvesterhci.io"
)

func NewMutator(
	settingCache ctlharvesterv1.SettingCache,
	resourceQuotaCache ctlharvcorev1.ResourceQuotaCache) types.Mutator {
	return &resourceQuotaMutator{
		settingCache:       settingCache,
		resourceQuotaCache: resourceQuotaCache,
	}
}

type resourceQuotaMutator struct {
	types.DefaultMutator
	settingCache       ctlharvesterv1.SettingCache
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
	if resourceQuota == nil || resourceQuota.Spec.Hard == nil {
		return nil, nil
	}

	patchEnable, err := m.getPatchHarvesterServicesInResourceQuotaSettingValue()
	if err != nil {
		return nil, err
	}

	return patchLoadBalancer(resourceQuota, patchEnable), nil
}

func (m *resourceQuotaMutator) Update(_ *types.Request, _ runtime.Object, newObj runtime.Object) (types.PatchOps, error) {
	resourceQuota := newObj.(*corev1.ResourceQuota)
	if resourceQuota == nil || resourceQuota.Spec.Hard == nil {
		return nil, nil
	}

	patchEnable, err := m.getPatchHarvesterServicesInResourceQuotaSettingValue()
	if err != nil {
		return nil, err
	}

	return patchLoadBalancer(resourceQuota, patchEnable), nil
}

func (m *resourceQuotaMutator) getPatchHarvesterServicesInResourceQuotaSettingValue() (bool, error) {
	patchHarvesterServicesInResourceQuotaSetting, err := m.settingCache.Get(settings.PatchHarvesterServicesInResourceQuotaSettingName)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"settingName": settings.PatchHarvesterServicesInResourceQuotaSettingName,
		}).Error("failed to get setting")
		return false, err
	}
	patchEnable, err := strconv.ParseBool(patchHarvesterServicesInResourceQuotaSetting.Value)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"settingName": settings.PatchHarvesterServicesInResourceQuotaSettingName,
			"value":       patchHarvesterServicesInResourceQuotaSetting.Value,
		}).Error("failed to parse setting value as boolean")
	}
	return patchEnable, nil
}

func patchLoadBalancer(resourceQuota *corev1.ResourceQuota, patchEnable bool) types.PatchOps {
	if patchEnable {
		return transferServicesToHarvesterServices(resourceQuota)
	}
	return transferHarvesterServicesToServices(resourceQuota)
}

func transferServicesToHarvesterServices(resourceQuota *corev1.ResourceQuota) types.PatchOps {
	serviceLoadBalancerQuota, ok := resourceQuota.Spec.Hard[corev1.ResourceServicesLoadBalancers]
	if !ok {
		return nil
	}

	hardCopy := resourceQuota.Spec.Hard.DeepCopy()
	hardCopy[harvesterLoadBalancersKey] = serviceLoadBalancerQuota
	delete(hardCopy, corev1.ResourceServicesLoadBalancers)
	if reflect.DeepEqual(hardCopy, resourceQuota.Spec.Hard) {
		return nil
	}

	hardCopyBytes, err := json.Marshal(hardCopy)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"resourceQuotaNamespace": resourceQuota.Namespace,
			"resourceQuotaName":      resourceQuota.Name,
			"hardCopy":               hardCopy,
		}).Error("failed to marshal hard quota")
		return nil
	}
	return []string{fmt.Sprintf(`{"op": "replace", "path": "%s", "value": %s}`, resourceQuotaHard, hardCopyBytes)}
}

func transferHarvesterServicesToServices(resourceQuota *corev1.ResourceQuota) types.PatchOps {
	harvesterServiceLoadBalancerQuota, ok := resourceQuota.Spec.Hard[harvesterLoadBalancersKey]
	if !ok {
		return nil
	}

	hardCopy := resourceQuota.Spec.Hard.DeepCopy()
	hardCopy[corev1.ResourceServicesLoadBalancers] = harvesterServiceLoadBalancerQuota
	delete(hardCopy, harvesterLoadBalancersKey)
	if reflect.DeepEqual(hardCopy, resourceQuota.Spec.Hard) {
		return nil
	}

	hardCopyBytes, err := json.Marshal(hardCopy)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"resourceQuotaNamespace": resourceQuota.Namespace,
			"resourceQuotaName":      resourceQuota.Name,
			"hardCopy":               hardCopy,
		}).Error("failed to marshal hard quota")
		return nil
	}
	return []string{fmt.Sprintf(`{"op": "replace", "path": "%s", "value": %s}`, resourceQuotaHard, hardCopyBytes)}
}
