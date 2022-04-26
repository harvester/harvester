package bundle

import (
	"fmt"

	fleetv1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator() types.Validator {
	return &bundleValidator{}
}

type bundleValidator struct {
	types.DefaultValidator
}

func (v *bundleValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{fleetv1alpha1.BundleResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   fleetv1alpha1.SchemeGroupVersion.Group,
		APIVersion: fleetv1alpha1.SchemeGroupVersion.Version,
		ObjectType: &fleetv1alpha1.Bundle{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Delete,
		},
	}
}
func (v *bundleValidator) Delete(request *types.Request, oldObj runtime.Object) error {
	bundle := oldObj.(*fleetv1alpha1.Bundle)

	// Template of Bundle name is "mcc-<managedchart>".
	// ref: https://github.com/rancher/rancher/blob/68eb5d1267679e02eb2383f0ec965b3f896ae3e3/pkg/controllers/provisioningv2/managedchart/managedchart.go#L95
	// ManagedChart namespaces and names are from:
	// https://github.com/harvester/harvester-installer/blob/f36c8cfaa68626c85cf4c35f681dd382880f2aa7/pkg/config/templates/rancherd-10-harvester.yaml#L65-L69
	// https://github.com/harvester/harvester-installer/blob/f36c8cfaa68626c85cf4c35f681dd382880f2aa7/pkg/config/templates/rancherd-10-harvester.yaml#L129-L133
	if bundle != nil &&
		bundle.Namespace == "fleet-local" &&
		(bundle.Name == "mcc-harvester" || bundle.Name == "mcc-harvester-crd") {
		message := fmt.Sprintf("Delete bundle %s/%s is prohibited", bundle.Namespace, bundle.Name)
		return werror.NewInvalidError(message, "")
	}

	return nil
}
