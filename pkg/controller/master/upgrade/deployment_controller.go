package upgrade

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

// deploymentHandler syncs upgrade CRD status on repo deployment status changes
type deploymentHandler struct {
	upgradeClient ctlharvesterv1.UpgradeClient
	upgradeCache  ctlharvesterv1.UpgradeCache
}

func (h *deploymentHandler) OnChanged(_ string, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	if deployment == nil || deployment.DeletionTimestamp != nil || deployment.Namespace != upgradeNamespace || !isDeploymentReady(deployment) {
		return deployment, nil
	}

	upgradeControllerLock.Lock()
	defer upgradeControllerLock.Unlock()

	upgrade, err := GetRepoDeploymentUpgrade(deployment, h.upgradeCache)
	if err != nil {
		return nil, err
	}
	if upgrade == nil || upgrade.Labels[upgradeStateLabel] != StatePreparingRepo {
		return deployment, nil
	}
	toUpdate := upgrade.DeepCopy()
	toUpdate.Labels[upgradeStateLabel] = StateRepoPrepared
	setRepoProvisionedCondition(toUpdate, v1.ConditionTrue, "", "")
	_, err = h.upgradeClient.Update(toUpdate)
	return deployment, err
}

// GetRepoDeploymentUpgrade returns the Upgrade that owns the repo Deployment, or nil if none matches.
func GetRepoDeploymentUpgrade(deployment *appsv1.Deployment, upgradeCache ctlharvesterv1.UpgradeCache) (*harvesterv1.Upgrade, error) {
	if deployment.Namespace != upgradeNamespace {
		return nil, nil
	}
	for _, owner := range deployment.OwnerReferences {
		if owner.Kind != "Upgrade" || owner.APIVersion != harvesterv1.SchemeGroupVersion.String() || deployment.Name != repoDeploymentName(owner.Name) {
			continue
		}

		upgrade, err := upgradeCache.Get(deployment.Namespace, owner.Name)
		if err != nil {
			if errors.IsNotFound(err) {
				logrus.Warnf("upgrade %s for deployment %s not found", owner.Name, deployment.Name)
				return nil, nil
			}
			return nil, fmt.Errorf("failed to get upgrade %s/%s: %w", deployment.Namespace, owner.Name, err)
		}
		if owner.UID == upgrade.UID {
			return upgrade, nil
		}
	}
	return nil, nil
}

func isDeploymentReady(deployment *appsv1.Deployment) bool {
	for _, cond := range deployment.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable && cond.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}
