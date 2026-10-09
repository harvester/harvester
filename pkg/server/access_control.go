package server

import (
	"fmt"

	"github.com/harvester/harvester/pkg/util"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
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

	allowed, err := util.CheckObjectAccess(apiOp.Context(), util.ResourceAccessCheck{
		SAR:       a.sar,
		Username:  userInfo.GetName(),
		Groups:    userInfo.GetGroups(),
		Verb:      util.VerbUpdate,
		GVR:       gvr,
		Namespace: apiOp.Namespace,
		Name:      apiOp.Name,
	})
	if err != nil {
		return err
	}
	if !allowed {
		return apierror.NewAPIError(validation.PermissionDenied, fmt.Sprintf("user %q cannot perform action %s on %s %s/%s", userInfo.GetName(), name, gvr.Resource, apiOp.Namespace, apiOp.Name))
	}

	return a.AccessControl.CanAction(apiOp, schema, name)
}
