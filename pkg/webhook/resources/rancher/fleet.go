package rancher

import (
	fleetv1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"

	"github.com/harvester/harvester/pkg/webhook/types"
)

// fleetLocalCluster validation rules ensure no changes are allowed to the clusters.fleet.cattle.io objects other
// than by the list of predefined service account
type fleetLocalCluster struct {
	rancherValidator
}

func NewFleetLocalClusterValidator() types.Validator {
	return &fleetLocalCluster{}
}

func (r *fleetLocalCluster) Resource() types.Resource {
	return types.Resource{
		Names:          []string{"clusters"},
		Scope:          admissionregv1.NamespacedScope,
		APIGroup:       "fleet.cattle.io",
		APIVersion:     "*",
		ObjectType:     &fleetv1alpha1.Cluster{},
		OperationTypes: []admissionregv1.OperationType{"*"},
	}
}
