package managedchart

import (
	"fmt"

	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator() types.Validator {
	return &managedChartValidator{}
}

type managedChartValidator struct {
	types.DefaultValidator
}

func (v *managedChartValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{managementv3.ManagedChartResourceName},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   managementv3.SchemeGroupVersion.Group,
		APIVersion: managementv3.SchemeGroupVersion.Version,
		ObjectType: &managementv3.ManagedChart{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Delete,
		},
	}
}
func (v *managedChartValidator) Delete(_ *types.Request, oldObj runtime.Object) error {
	managedChart := oldObj.(*managementv3.ManagedChart)

	// ManagedChart namespaces and names are from:
	// https://github.com/harvester/harvester-installer/blob/f36c8cfaa68626c85cf4c35f681dd382880f2aa7/pkg/config/templates/rancherd-10-harvester.yaml#L65-L69
	// https://github.com/harvester/harvester-installer/blob/f36c8cfaa68626c85cf4c35f681dd382880f2aa7/pkg/config/templates/rancherd-10-harvester.yaml#L129-L133
	if managedChart != nil &&
		managedChart.Namespace == "fleet-local" &&
		(managedChart.Name == "harvester" || managedChart.Name == "harvester-crd") {
		message := fmt.Sprintf("Delete managedchart %s/%s is prohibited", managedChart.Namespace, managedChart.Name)
		return werror.NewInvalidError(message, "")
	}

	return nil
}
