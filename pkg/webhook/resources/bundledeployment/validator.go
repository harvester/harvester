package bundledeployment

import (
	"fmt"

	fleetv1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	fleetcontrollers "github.com/rancher/rancher/pkg/generated/controllers/fleet.cattle.io/v1alpha1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(clusterCache fleetcontrollers.ClusterCache) types.Validator {
	return &bundleDeploymentValidator{
		clusterCache: clusterCache,
	}
}

type bundleDeploymentValidator struct {
	types.DefaultValidator
	clusterCache fleetcontrollers.ClusterCache
}

func (v *bundleDeploymentValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{fleetv1alpha1.BundleDeploymentResourceNamePlural},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   fleetv1alpha1.SchemeGroupVersion.Group,
		APIVersion: fleetv1alpha1.SchemeGroupVersion.Version,
		ObjectType: &fleetv1alpha1.BundleDeployment{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Delete,
		},
	}
}
func (v *bundleDeploymentValidator) Delete(_ *types.Request, oldObj runtime.Object) error {
	bundleDeployment := oldObj.(*fleetv1alpha1.BundleDeployment)

	// BundleDeployment name is from Bundle.
	// ref: https://github.com/rancher/fleet/blob/4dc66c946ca2f90f5f7a5d360a573698687a3a11/pkg/target/target.go#L370-L376
	// Template of Bundle name is "mcc-<managedchart>".
	// ref: https://github.com/rancher/rancher/blob/68eb5d1267679e02eb2383f0ec965b3f896ae3e3/pkg/controllers/provisioningv2/managedchart/managedchart.go#L95
	// ManagedChart names are from:
	// https://github.com/harvester/harvester-installer/blob/f36c8cfaa68626c85cf4c35f681dd382880f2aa7/pkg/config/templates/rancherd-10-harvester.yaml#L65-L69
	// https://github.com/harvester/harvester-installer/blob/f36c8cfaa68626c85cf4c35f681dd382880f2aa7/pkg/config/templates/rancherd-10-harvester.yaml#L129-L133
	if bundleDeployment != nil &&
		(bundleDeployment.Name == "mcc-harvester" || bundleDeployment.Name == "mcc-harvester-crd") {

		// Fleet cluster namespace and name is from:
		// https://github.com/harvester/harvester-installer/blob/f36c8cfaa68626c85cf4c35f681dd382880f2aa7/pkg/config/templates/rancherd-10-harvester.yaml#L76-L77
		cluster, err := v.clusterCache.Get("fleet-local", "local")
		if err != nil {
			return err
		}
		if bundleDeployment.Namespace == cluster.Status.Namespace {
			message := fmt.Sprintf("Delete bundledeployment %s/%s is prohibited", bundleDeployment.Namespace, bundleDeployment.Name)
			return werror.NewInvalidError(message, "")
		}
	}

	return nil
}
