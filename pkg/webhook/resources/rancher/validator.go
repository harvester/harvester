package rancher

import (
	"fmt"

	fleetv1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	"github.com/rancher/wrangler/pkg/slice"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	podNameKey = "authentication.kubernetes.io/pod-name"
)

var (
	whitelistedServiceAccounts = []string{"system:serviceaccount:cattle-system:rancher"}
	whitelistedPodPrefixes     = []string{"rancher", "fleet", "harvester", "longhorn", "pcidevices", "node-manager"}
)

type rancherValidator struct {
	types.DefaultValidator
}

func NewValidator() types.Validator {
	return &rancherValidator{}
}

func (r *rancherValidator) Resource() types.Resource {
	return types.Resource{
		Names:          []string{"*"},
		Scope:          admissionregv1.NamespacedScope,
		APIGroup:       "fleet.cattle.io",
		APIVersion:     "*",
		ObjectType:     &fleetv1alpha1.Cluster{},
		OperationTypes: []admissionregv1.OperationType{"*"},
	}
}

func (r *rancherValidator) Create(request *types.Request, newObj runtime.Object) error {
	if !requestIsNotFromInfra(request) {
		return fmt.Errorf("user %s cannot perform action", request.Request.UserInfo.Username)
	}
	return nil
}

func (r *rancherValidator) Update(request *types.Request, oldObj, newObj runtime.Object) error {
	if !requestIsNotFromInfra(request) {
		return fmt.Errorf("user %s cannot perform action", request.Request.UserInfo.Username)
	}
	return nil
}

func (r *rancherValidator) Delete(request *types.Request, oldObj runtime.Object) error {
	if !requestIsNotFromInfra(request) {
		return fmt.Errorf("user %s cannot perform action", request.Request.UserInfo.Username)
	}
	return nil
}

func (r *rancherValidator) Connect(request *types.Request, oldObj runtime.Object) error {
	if !requestIsNotFromInfra(request) {
		return fmt.Errorf("user %s cannot perform action", request.Request.UserInfo.Username)
	}
	return nil
}

// query extra info, for requests coming from pods this is set.
func requestIsNotFromInfra(request *types.Request) bool {
	if val, ok := request.Request.UserInfo.Extra[podNameKey]; ok {
		if slice.ContainsString(whitelistedPodPrefixes, val[0]) {
			return true
		}
	}

	return false
}
