package deployment

import (
	"fmt"

	"github.com/harvester/harvester/pkg/controller/master/upgrade"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
	ctlappsv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/apps/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	appv1 "k8s.io/api/apps/v1"
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

	owner, err := upgrade.GetRepoDeploymentUpgrade(deployment, v.upgradeCache)
	if err != nil {
		return err
	}
	if util.IsUpgradeInProgress(owner) {
		return fmt.Errorf("cannot delete deployment %s/%s: upgrade %s is still in progress (component: %s)",
			deployment.Namespace, deployment.Name, owner.Name, util.HarvesterUpgradeComponentRepo)
	}

	return nil
}
