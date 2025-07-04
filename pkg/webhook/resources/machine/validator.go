package machine

import (
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(client client.Client) types.Validator {
	return &machineValidator{
		client: client,
	}
}

type machineValidator struct {
	types.DefaultValidator
	client client.Client
}

func (v *machineValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"machines"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   corev1.SchemeGroupVersion.Group,
		APIVersion: corev1.SchemeGroupVersion.Version,
		ObjectType: &corev1.Node{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}

func (v *machineValidator) Create(req *types.Request, newObj runtime.Object) error {

	// if cluster is upgrading, intercept the request

	return nil
}
