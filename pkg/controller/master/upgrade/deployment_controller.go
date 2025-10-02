package upgrade

import (
	v1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	upgradev1 "github.com/harvester/harvester/pkg/generated/controllers/upgrade.cattle.io/v1"
)

// deploymentHandler syncs upgrade CRD status on repo deployment status changes
type deploymentHandler struct {
	namespace     string
	planCache     upgradev1.PlanCache
	upgradeClient ctlharvesterv1.UpgradeClient
	upgradeCache  ctlharvesterv1.UpgradeCache
}

func (h *deploymentHandler) OnChanged(_ string, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	if deployment == nil || deployment.DeletionTimestamp != nil || deployment.Labels == nil || deployment.Namespace != upgradeNamespace || deployment.Labels[harvesterUpgradeLabel] == "" {
		return deployment, nil
	}

	upgradeControllerLock.Lock()
	defer upgradeControllerLock.Unlock()

	upgrade, err := h.upgradeCache.Get(upgradeNamespace, deployment.Labels[harvesterUpgradeLabel])
	if err != nil {
		return nil, err
	}

	component := deployment.Labels[harvesterUpgradeComponentLabel]
	switch upgrade.Labels[upgradeStateLabel] {
	case StatePreparingRepo:
		if component == upgradeComponentRepo &&  > 0 {
			if deployment.Status.ContainerStatuses[0].Ready {
				toUpdate := upgrade.DeepCopy()
				toUpdate.Labels[upgradeStateLabel] = StateRepoPrepared
				setRepoProvisionedCondition(toUpdate, v1.ConditionTrue, "", "")
				_, err = h.upgradeClient.Update(toUpdate)
				return pod, err
			}
		}
	}

	return pod, nil
}
