package machine

import (
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/sirupsen/logrus"
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
	machine, ok := newObj.(*clusterv1.Machine)
	if !ok {
		return fmt.Errorf("expected Machine, but got %T", newObj)
	}

	upgrading, err := v.isClusterUpgrading()
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"machine":   machine.Name,
			"namespace": machine.Namespace,
			"error":     err,
		}).Error("failed to check cluster upgrade status")
		return nil
	}

	if upgrading {
		fields := logrus.Fields{
			"machine":   machine.Name,
			"namespace": machine.Namespace,
		}

		if machine.Spec.ProviderID != nil && *machine.Spec.ProviderID != "" {
			fields["providerID"] = *machine.Spec.ProviderID
		}

		if len(machine.Status.Addresses) > 0 {
			addrParts := []string{}
			for _, addr := range machine.Status.Addresses {
				// MachineAddressType gives type (HostName, ExternalIP, InternalIP…)
				addrParts = append(addrParts, fmt.Sprintf("%s=%s", addr.Type, addr.Address))
			}
			fields["addresses"] = addrParts
		}

		logrus.WithFields(fields).Warn("blocking creation of new Harvester Machine: upgrade in progress")

		return fmt.Errorf("cluster upgrade in progress: cannot create machine %s", machine.Name)
	}

	return nil
}

func (v *machineValidator) isClusterUpgrading() (bool, error) {
	selector := labels.Set{
		util.LabelUpgradeLatest: "true",
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
