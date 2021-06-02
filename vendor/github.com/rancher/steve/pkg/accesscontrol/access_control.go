package accesscontrol

import (
	"github.com/rancher/apiserver/pkg/server"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/wrangler/pkg/kv"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type AccessControl struct {
	server.SchemaBasedAccess
}

func NewAccessControl() *AccessControl {
	return &AccessControl{}
}

func (a *AccessControl) CanDo(apiOp *types.APIRequest, resource, verb, namespace, name string) error {
	apiSchema := apiOp.Schemas.LookupSchema(resource)
	if apiSchema != nil && attributes.GVK(apiSchema).Kind != "" {
		access := GetAccessListMap(apiSchema)
		if access[verb].Grants(namespace, name) {
			return nil
		}
	}
	group, resource := kv.Split(resource, "/")
	accessSet := apiOp.Schemas.Attributes["accessSet"].(*AccessSet)
	if accessSet.Grants(verb, schema.GroupResource{
		Group:    group,
		Resource: resource,
	}, namespace, name) {
		return nil
	}
	return a.SchemaBasedAccess.CanDo(apiOp, resource, verb, namespace, name)
}

func (a *AccessControl) CanWatch(apiOp *types.APIRequest, schema *types.APISchema) error {
	if attributes.GVK(schema).Kind != "" {
		access := GetAccessListMap(schema)
		if _, ok := access["watch"]; ok {
			return nil
		}
	}
	return a.SchemaBasedAccess.CanWatch(apiOp, schema)
}
