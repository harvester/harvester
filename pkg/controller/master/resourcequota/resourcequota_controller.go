package resourcequota

import (
	"errors"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	ctlharvcorev1 "github.com/harvester/harvester/pkg/generated/controllers/core/v1"
	"github.com/harvester/harvester/pkg/util"

	rqutils "github.com/harvester/harvester/pkg/util/resourcequota"
)

type Handler struct {
	rqs     ctlharvcorev1.ResourceQuotaClient
	rqCache ctlharvcorev1.ResourceQuotaCache
}

var errSkipScaling = errors.New("skip scaling")

func (h *Handler) OnResourceQuotaChanged(_ string, rq *corev1.ResourceQuota) (*corev1.ResourceQuota, error) {
	// not a target rq
	if rq == nil || rq.DeletionTimestamp != nil || rq.Annotations == nil || rq.Labels == nil || rq.Labels[util.LabelManagementDefaultResourceQuota] != "true" {
		return rq, nil
	}

	rqCopy := rq.DeepCopy()
	update, err := scaleResourceOnDemand(rqCopy)
	if err != nil {
		if errors.Is(err, errSkipScaling) {
			return rq, nil
		}
		return rq, err
	}

	if update {
		return h.rqs.Update(rqCopy)
	}

	return rq, nil
}

func scaleResourceOnDemand(rq *corev1.ResourceQuota) (bool, error) {
	// below data is only related to rq itself, if error happens and run reconciller, it will fall into error looping
	update := false
	if rqutils.IsEmptyResourceQuota(rq) {
		logrus.Warnf("resourcequota %s/%s has 0 quota, skip scaling", rq.Namespace, rq.Name)
		return update, errSkipScaling
	}

	carq, _ := rq.Annotations[util.CattleAnnotationResourceQuota]
	// NamespaceResourceQuota
	rqBase, err := rqutils.GetRancherNamespaceResourceQuotaFromRQAnnotations(rq)
	if err != nil {
		logrus.Warnf("resourcequota %s/%s has invalid %s annotation %s, skip scaling, error %s", rq.Namespace, rq.Name, util.CattleAnnotationResourceQuota, carq, err.Error())
		return update, errSkipScaling
	}
	if rqBase == nil {
		logrus.Warnf("resourcequota %s/%s has no %s annotation, skip scaling", rq.Namespace, rq.Name, util.CattleAnnotationResourceQuota)
		return update, errSkipScaling
	}

	rCPULimit, rMemoryLimit, err := rqutils.GetCPUMemoryLimitsFromRancherNamespaceResourceQuota(rqBase)
	if err != nil {
		logrus.Warnf("resourcequota %s/%s can't get valid Quantity values from rancher %s annotations %s, skip scaling, error %s", rq.Namespace, rq.Name, util.CattleAnnotationResourceQuota, carq, err.Error())
		return update, errSkipScaling
	}

	cpuDelta, memDelta, _, err := rqutils.GetVMIMResourcesFromRQAnnotation(rq)
	if err != nil {
		logrus.Warnf("resourcequota %s/%s can't get valid Quantity values from harvester vm annotations, skip scaling, error %s", rq.Namespace, rq.Name, err.Error())
		return update, errSkipScaling
	}

	update = rqutils.CalculateNewResourceQuotaFromBaseDelta(rq, rCPULimit, rMemoryLimit, cpuDelta, memDelta)
	return update, nil
}
