package namespace

import (
	"encoding/json"
	"fmt"
	"reflect"

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	harvestercorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	harvesterctlv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	rqutils "github.com/harvester/harvester/pkg/util/resourcequota"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(resourceQuotaCache harvestercorev1.ResourceQuotaCache, settingsCache harvesterctlv1beta1.SettingCache) types.Validator {
	return &namespaceValidator{
		resourceQuotaCache: resourceQuotaCache,
		settingsCache:      settingsCache,
	}
}

// account name passed by harvester
// can be used to filter non controller operations
const harvesterSAName = "system:serviceaccount:harvester-system:harvester"

type namespaceValidator struct {
	types.DefaultValidator
	resourceQuotaCache harvestercorev1.ResourceQuotaCache
	settingsCache      harvesterctlv1beta1.SettingCache
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

func (v *namespaceValidator) Update(req *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	oldNamespace := oldObj.(*corev1.Namespace)
	newNamespace := newObj.(*corev1.Namespace)

	// perform resource quota validation
	if err := v.resourceQuotaValidation(oldNamespace, newNamespace); err != nil {
		return err
	}

	// perform pss validation to ensure only harvester controller can apply pss labels
	// when setting is enabled
	return v.validatePSSOnNamespaceUpdate(req, oldNamespace, newNamespace)
}

func (v *namespaceValidator) resourceQuotaValidation(oldNamespace, newNamespace *corev1.Namespace) error {
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

// validate PSS label is only being applied by the harvester controller, attempts to change this
// from non controller service account will be blocked if the setting is enabled
func (v *namespaceValidator) validatePSSOnNamespaceUpdate(req *types.Request, oldNamespace, newNamespace *corev1.Namespace) error {
	settingObj, err := v.settingsCache.Get(settings.ClusterPodSecurityStandardSettingName)
	if err != nil {
		return fmt.Errorf("error fetching setting %s from cache: %v", settings.ClusterPodSecurityStandardSettingName, err)
	}

	pssSetting, err := settings.GetPodSecuritySetting(settingObj)
	if err != nil {
		return err
	}

	// setting is not enabled, nothing need to be done
	if !pssSetting.Enabled {
		return nil
	}

	oldNamespaceCopy := oldNamespace.DeepCopy()
	if oldNamespaceCopy.Labels == nil {
		oldNamespaceCopy.Labels = make(map[string]string)
	}
	// verify pss label is not being changed
	for _, key := range []string{util.PSSEnforcementKey, util.PSSEnforcementVersionKey, util.HarvesterManagedPSSKey} {
		val, ok := newNamespace.Labels[key]
		if ok {
			oldNamespaceCopy.Labels[key] = val
		}

	}

	// check if labels have changed from oldNamespace by running a reflect
	if !reflect.DeepEqual(oldNamespaceCopy.Labels, oldNamespace.Labels) && req.Username() != harvesterSAName {
		return fmt.Errorf("%s is enabled, PSS level can only be changed by cluster admin via the setting", settings.ClusterPodSecurityStandardSettingName)
	}

	return nil
}

// Create ensures user cannot apply PSS label during namespace creation there by ignoring the underlying cluster level
// settings
func (v *namespaceValidator) Create(req *types.Request, newObj runtime.Object) error {
	newNamespace := newObj.(*corev1.Namespace)

	settingObj, err := v.settingsCache.Get(settings.ClusterPodSecurityStandardSettingName)
	if err != nil {
		return fmt.Errorf("error fetching setting %s from cache: %v", settings.ClusterPodSecurityStandardSettingName, err)
	}

	pssSetting, err := settings.GetPodSecuritySetting(settingObj)
	if err != nil {
		return err
	}

	// setting is not enabled, nothing need to be done
	if !pssSetting.Enabled {
		return nil
	}

	for _, key := range []string{util.PSSEnforcementKey, util.PSSEnforcementVersionKey, util.HarvesterManagedPSSKey} {
		_, ok := newNamespace.Labels[key]
		// only allow harvester service account to apply pss labels when setting is enabled
		if ok && req.Username() != harvesterSAName {
			return fmt.Errorf("%s is enabled, PSS level can only be applied by cluster admin via the setting", settings.ClusterPodSecurityStandardSettingName)
		}

	}

	return nil
}
