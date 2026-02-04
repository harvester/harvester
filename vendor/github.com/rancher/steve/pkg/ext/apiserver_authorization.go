package ext

import (
	"context"

	"github.com/rancher/steve/pkg/accesscontrol"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

var _ authorizer.Authorizer = (*AccessSetAuthorizer)(nil)

type AccessSetAuthorizer struct {
	asl accesscontrol.AccessSetLookup
}

func NewAccessSetAuthorizer(asl accesscontrol.AccessSetLookup) *AccessSetAuthorizer {
	return &AccessSetAuthorizer{
		asl: asl,
	}
}

// Authorize implements [authorizer.Authorizer].
func (a *AccessSetAuthorizer) Authorize(ctx context.Context, attrs authorizer.Attributes) (authorized authorizer.Decision, reason string, err error) {
	verb := attrs.GetVerb()
	path := attrs.GetPath()
	accessSet := a.asl.AccessFor(attrs.GetUser())

	if !attrs.IsResourceRequest() {
		if accessSet.GrantsNonResource(verb, path) {
			return authorizer.DecisionAllow, "", nil
		}

		// An empty string reason will still provide enough information such as:
		//
		//    forbidden: User "unknown-user" cannot post path /openapi/v3
		return authorizer.DecisionDeny, "", nil
	}

	namespace := attrs.GetNamespace()
	name := attrs.GetName()
	gr := schema.GroupResource{
		Group:    attrs.GetAPIGroup(),
		Resource: attrs.GetResource(),
	}

	if accessSet.Grants(verb, gr, namespace, name) {
		return authorizer.DecisionAllow, "", nil
	}

	// An empty string reason will still provide enough information such as:
	//
	//     testtypes.ext.cattle.io is forbidden: User "unknown-user" cannot list resource "testtypes" in API group "ext.cattle.io" at the cluster scope
	return authorizer.DecisionDeny, "", nil
}
