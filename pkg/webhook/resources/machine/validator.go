package machine

import (
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"

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
		Names:      []string{"machines"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   "cluster.x-k8s.io",
		APIVersion: "v1beta1",
		ObjectType: &corev1.Node{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}

func (v *machineValidator) Create(req *types.Request, newObj runtime.Object) error {

	// if cluster is upgrading, intercept the request
	isUpgrading, err := v.isClusterUpgrading()
	if err != nil {
		return fmt.Errorf("failed to check if cluster is upgrading: %w", err)
	}
	if isUpgrading {
		return fmt.Errorf("cluster is upgrading, cannot create machine")
	}

	return nil
}

func (v *machineValidator) isClusterUpgrading() (bool, error) {

	upgrades, err := v.upgradeCache.List(util.HarvesterSystemNamespaceName, labels.NewSelector())

	if err != nil {
		return false, err
	}
	for _, upgrade := range upgrades {
		if !harvesterv1.UpgradeCompleted.IsFalse(upgrade) {
			return true, nil
		}
	}
	return false, nil
}
