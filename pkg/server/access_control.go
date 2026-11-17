package server

import (
	"fmt"

	"github.com/harvester/harvester/pkg/util"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	authorizationv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

type actionAccessControl struct {
	types.AccessControl
	sar authorizationv1client.SubjectAccessReviewInterface
}

var _ types.AccessControl = (*actionAccessControl)(nil)

func (a *actionAccessControl) CanAction(apiOp *types.APIRequest, schema *types.APISchema, name string) error {
	userInfo, ok := apiOp.GetUserInfo()
	if !ok {
		return apierror.NewAPIError(validation.Unauthorized, "failed to get user from request")
	}

	gvr := attributes.GVR(schema)
	if gvr.Resource == "" || gvr.Version == "" {
		return fmt.Errorf("schema %q is missing Kubernetes group/version/resource attributes", schema.ID)
	}

	check := util.ResourceAccessCheck{
		SAR:       a.sar,
		Username:  userInfo.GetName(),
		Groups:    userInfo.GetGroups(),
		Verb:      util.VerbUpdate,
		GVR:       gvr,
		Namespace: apiOp.Namespace,
		Name:      apiOp.Name,
	}
	// Some actions map to a narrower permission than update on the whole resource,
	// for example VM start maps to update on subresources.kubevirt.io virtualmachines/start.
	if perm, ok := util.LookupActionPermission(gvr.GroupResource(), name); ok {
		check.Verb = perm.Verb
		check.GVR = k8sschema.GroupVersionResource{Group: perm.Group, Resource: perm.Resource}
		check.Subresource = perm.Subresource
		if !perm.TargetsObject() {
			check.Name = ""
		}
	}

	allowed, err := util.CheckObjectAccess(apiOp.Context(), check)
	if err != nil {
		return apierror.NewAPIError(validation.ServerError, fmt.Sprintf("failed to check object access: %v", err))
	}
	if !allowed {
		return apierror.NewAPIError(validation.PermissionDenied, fmt.Sprintf("user %q cannot perform action %s on %s %s/%s", userInfo.GetName(), name, gvr.Resource, apiOp.Namespace, apiOp.Name))
	}

	return a.AccessControl.CanAction(apiOp, schema, name)
}
