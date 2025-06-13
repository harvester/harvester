package resourcequota

import (
	"encoding/json"
	"errors"
	"fmt"

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	ctlv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	ctlharvcorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	"github.com/harvester/harvester/pkg/util"
	rqutils "github.com/harvester/harvester/pkg/util/resourcequota"
)

type Handler struct {
	nsCache ctlv1.NamespaceCache
	rqs     ctlharvcorev1.ResourceQuotaClient
	rqCache ctlharvcorev1.ResourceQuotaCache
}

var errSkipScaling = errors.New("skip scaling")

// When Harvester is scaling resourcequota per vmim, the resourcequota can also be reset by Rancher
// Luckily Rancher only resets it when Rancher related POD starts, it does not monitor and reset it all the time
// Harvester controller calculates the resourcequota per Rancher annotation and Harvester annotation
// If someday Rancher keeps resetting it, this controller needs an enhancement to avoid looping update resourcequota with Rancher
func (h *Handler) OnResourceQuotaChanged(_ string, rq *corev1.ResourceQuota) (*corev1.ResourceQuota, error) {
	// not a target resourcequota
	if rq == nil || rq.DeletionTimestamp != nil || rq.Labels == nil || rq.Labels[util.LabelManagementDefaultResourceQuota] != "true" {
		return rq, nil
	}

	// for debugging or manually disabling this feature
	if rq.Annotations[util.AnnotationSkipResourceQuotaAutoScaling] == "true" {
		logrus.Debugf("resourcequota %s/%s annotation %s is set, skip auto scaling", rq.Namespace, rq.Name, util.AnnotationSkipResourceQuotaAutoScaling)
		return rq, nil
	}

	// check namespace ResourceQuota annotation
	ns, err := h.nsCache.Get(rq.Namespace)
	if err != nil {
		return nil, fmt.Errorf("can't find resourcequota %s/%s related namespace, error %w", rq.Namespace, rq.Name, err)
	}

	rqCopy := rq.DeepCopy()
	update, err := scaleResourceQuotaOnDemand(rqCopy, ns.Annotations[util.CattleAnnotationResourceQuota])
	if err != nil {
		if errors.Is(err, errSkipScaling) {
			return rq, nil
		}
		return rq, err
	}
	if update {
		logrus.Debugf("resourcequota %s/%s is updated from %+v to %+v", rq.Namespace, rq.Name, rq.Spec, rqCopy.Spec)
		return h.rqs.Update(rqCopy)
	}

	return rq, nil
}

func scaleResourceQuotaOnDemand(rq *corev1.ResourceQuota, rqStr string) (bool, error) {
	update := false
	if rqStr == "" {
		logrus.Debugf("resourcequota %s/%s related namespace has empty %s annotation, skip scaling", rq.Namespace, rq.Name, util.CattleAnnotationResourceQuota)
		return update, errSkipScaling
	}
	var rqBase *v3.NamespaceResourceQuota
	if err := json.Unmarshal([]byte(rqStr), &rqBase); err != nil {
		logrus.Warnf("resourcequota %s/%s related namespace has invalid %s annotation %s, skip scaling, error %s", rq.Namespace, rq.Name, util.CattleAnnotationResourceQuota, rqStr, err.Error())
		return update, err
	}

	// if both CPU and memory have no limits
	if rqutils.IsEmptyResourceQuota(rq) {
		logrus.Debugf("resourcequota %s/%s has 0 quota, skip scaling", rq.Namespace, rq.Name)
		return update, errSkipScaling
	}

	rCPULimit, rMemoryLimit, err := rqutils.GetCPUMemoryLimitsFromRancherNamespaceResourceQuota(rqBase)
	if err != nil {
		logrus.Warnf("resourcequota %s/%s can't get valid Quantity values from Rancher %s annotations %s, skip scaling, error %s", rq.Namespace, rq.Name, util.CattleAnnotationResourceQuota, rqStr, err.Error())
		return update, errSkipScaling
	}

	cpuDelta, memDelta, _, err := rqutils.GetVMIMResourcesFromRQAnnotation(rq)
	if err != nil {
		logrus.Warnf("resourcequota %s/%s can't get valid Quantity values from Harvester vm annotations, skip scaling, error %s", rq.Namespace, rq.Name, err.Error())
		return update, errSkipScaling
	}

	// note: if any base has no limit, the delta is not added
	_, update = rqutils.CalculateNewResourceQuotaFromBaseDelta(rq, rCPULimit, rMemoryLimit, cpuDelta, memDelta)
	return update, nil
}
