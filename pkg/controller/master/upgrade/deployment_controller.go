package upgrade

import (
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"

	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
)

// deploymentHandler syncs upgrade CRD status on repo deployment status changes
type deploymentHandler struct {
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
		if errors.IsNotFound(err) {
			logrus.Warnf("upgrade %s for deployment %s not found, skip syncing upgrade repo deployment condition", deployment.Labels[harvesterUpgradeLabel], deployment.Name)
			return deployment, nil
		}
		return nil, err
	}

	component := deployment.Labels[harvesterUpgradeComponentLabel]
	if upgrade.Labels[upgradeStateLabel] == StatePreparingRepo && component == upgradeComponentRepo && isDeploymentReady(deployment) {
		toUpdate := upgrade.DeepCopy()
		toUpdate.Labels[upgradeStateLabel] = StateRepoPrepared
		setRepoProvisionedCondition(toUpdate, v1.ConditionTrue, "", "")
		_, err = h.upgradeClient.Update(toUpdate)
		return deployment, err
	}

	return deployment, nil
}

func isDeploymentReady(deployment *appsv1.Deployment) bool {
	for _, cond := range deployment.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable && cond.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}
