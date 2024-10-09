package data

import (
	"github.com/rancher/wrangler/v3/pkg/apply"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	FleetDefaultNamespace = "fleet-default"
)

// To fix missing namespace error in Rancher v2.9.2.
// https://github.com/harvester/harvester/issues/6692
func addFleetDefaultNamespace(apply apply.Apply) error {
	return apply.
		WithDynamicLookup().
		WithSetID(FleetDefaultNamespace).
		ApplyObjects(
			&v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: FleetDefaultNamespace},
			})
}
