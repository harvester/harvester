package deployment

import (
	"fmt"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
	ctlappsv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/apps/v1"
	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	appv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

func NewValidator(
	upgradeCache ctlharvesterv1.UpgradeCache,
	deploymentCache ctlappsv1.DeploymentCache) types.Validator {
	return &deploymentValidator{
		upgradeCache:    upgradeCache,
		deploymentCache: deploymentCache,
	}
}

type deploymentValidator struct {
	types.DefaultValidator
	upgradeCache    ctlharvesterv1.UpgradeCache
	deploymentCache ctlappsv1.DeploymentCache
}

func (v *deploymentValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"deployments"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   appv1.SchemeGroupVersion.Group,
		APIVersion: appv1.SchemeGroupVersion.Version,
		ObjectType: &appv1.Deployment{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Delete,
		},
	}
}

func (v *deploymentValidator) Delete(request *types.Request, curObj runtime.Object) error {
	if request.IsGarbageCollection() || request.IsFromController() {
		return nil
	}

	deployment := curObj.(*appv1.Deployment)

	upgradeName, hasUpgradeLabel := deployment.Labels[util.LabelHarvesterUpgrade]
	if !hasUpgradeLabel {
		// not managed by upgrade, allow delete
		return nil
	}

	upgrade, err := v.upgradeCache.Get(deployment.Namespace, upgradeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// upgrade not found, allow delete
			return nil
		}
		logrus.WithFields(logrus.Fields{"namespace": deployment.Namespace, "name": deployment.Name}).
			WithError(err).Errorf("failed to get upgrade %s", upgradeName)
		return err
	}

	if harvesterv1.UpgradeCompleted.IsTrue(upgrade) || harvesterv1.UpgradeCompleted.IsFalse(upgrade) {
		// upgrade is completed or failed, allow delete
		return nil
	}

	// check if the deployment belongs to upgrade repo component, if so, prevent delete when upgrade is in progress
	component := deployment.Labels[util.LabelHarvesterUpgradeComponent]
	if component == util.HarvesterUpgradeComponentRepo {
		return fmt.Errorf("cannot delete deployment %s/%s: upgrade %s is still in progress (component: %s)",
			deployment.Namespace, deployment.Name, upgradeName, component)
	}

	return nil
}
