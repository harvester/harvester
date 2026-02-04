package accesscontrol

import (
	apiserver "github.com/rancher/apiserver/pkg/server"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/attributes"
	"github.com/rancher/wrangler/v3/pkg/kv"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const accessSetAttribute = "accessSet"

type AccessControl struct {
	apiserver.SchemaBasedAccess
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
	accessSet := AccessSetFromAPIRequest(apiOp)
	if accessSet != nil && accessSet.Grants(verb, schema.GroupResource{
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

// SetAccessSetAttribute stores the provided accessSet using a predefined attribute
func SetAccessSetAttribute(schemas *types.APISchemas, accessSet *AccessSet) {
	if schemas.Attributes == nil {
		schemas.Attributes = map[string]interface{}{}
	}
	schemas.Attributes[accessSetAttribute] = accessSet
}

// AccessSetFromAPIRequest retrieves an AccessSet from the APIRequest Schemas attributes, if defined.
// This attribute must have been previously set by using SetAccessSetAttribute
func AccessSetFromAPIRequest(req *types.APIRequest) *AccessSet {
	if req == nil || req.Schemas == nil {
		return nil
	}
	if v, ok := req.Schemas.Attributes[accessSetAttribute]; ok {
		return v.(*AccessSet)
	}
	return nil
}
