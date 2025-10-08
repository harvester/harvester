package machine

import (
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(client client.Client, upgradeCache ctlharvesterv1.UpgradeCache) types.Validator {
	return &machineValidator{
		client:       client,
		upgradeCache: upgradeCache,
	}
}

type machineValidator struct {
	types.DefaultValidator
	client       client.Client
	upgradeCache ctlharvesterv1.UpgradeCache
}

func (v *machineValidator) Resource() types.Resource {
	return types.Resource{
		Names:    []string{"machines"},
		Scope:    admissionregv1.ClusterScope,
		APIGroup: clusterv1.GroupVersion.Group,
		APIVersion: clusterv1.GroupVersion.Version,
		ObjectType: &clusterv1.Machine{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}

func (v *machineValidator) Create(_ *types.Request, newObj runtime.Object) error {

	upgrading, err := v.isClusterUpgrading()
	if err != nil {
		return fmt.Errorf("failed to check cluster upgrade status: %w", err)
	}

	if upgrading {
		return fmt.Errorf("cluster upgrade is in progress: node registration is temporarily blocked")
	}

	return nil
}

const upgradeLatestLabel = "harvesterhci.io/latestUpgrade"

func (v *machineValidator) isClusterUpgrading() (bool, error) {
	selector := labels.Set{
		upgradeLatestLabel: "true",
	}.AsSelector()

	upgrades, err := v.upgradeCache.List(util.HarvesterSystemNamespaceName, selector)
	if err != nil {
		return false, err
	}

	for _, upgrade := range upgrades {
		// UpgradeCompleted == Unknown == upgrade still running
		if harvesterv1.UpgradeCompleted.IsUnknown(upgrade) {
			return true, nil
		}
	}

	return false, nil
}
