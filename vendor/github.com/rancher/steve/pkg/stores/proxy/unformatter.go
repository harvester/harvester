package proxy

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/v3/pkg/data"
	"github.com/rancher/wrangler/v3/pkg/data/convert"
)

// unformatterStore removes fields added by the formatter that kubernetes cannot recognize.
type unformatterStore struct {
	types.Store
}

// NewUnformatterStore returns a store which removes fields added by the formatter that kubernetes cannot recognize.
func NewUnformatterStore(s types.Store) types.Store {
	return &unformatterStore{Store: s}
}

// ByID looks up a single object by its ID.
func (u *unformatterStore) ByID(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	return u.Store.ByID(apiOp, schema, id)
}

// List returns a list of resources.
func (u *unformatterStore) List(apiOp *types.APIRequest, schema *types.APISchema) (types.APIObjectList, error) {
	return u.Store.List(apiOp, schema)
}

// Create creates a single object in the store.
func (u *unformatterStore) Create(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject) (types.APIObject, error) {
	return u.Store.Create(apiOp, schema, data)
}

// Update updates a single object in the store.
func (u *unformatterStore) Update(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject, id string) (types.APIObject, error) {
	data = unformat(data)
	return u.Store.Update(apiOp, schema, data, id)
}

// Delete deletes an object from a store.
func (u *unformatterStore) Delete(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	return u.Store.Delete(apiOp, schema, id)

}

// Watch returns a channel of events for a list or resource.
func (u *unformatterStore) Watch(apiOp *types.APIRequest, schema *types.APISchema, wr types.WatchRequest) (chan types.APIEvent, error) {
	return u.Store.Watch(apiOp, schema, wr)
}

func unformat(obj types.APIObject) types.APIObject {
	unst, ok := obj.Object.(map[string]interface{})
	if !ok {
		return obj
	}
	data.RemoveValue(unst, "metadata", "fields")
	data.RemoveValue(unst, "metadata", "relationships")
	data.RemoveValue(unst, "metadata", "state")
	conditions, ok := data.GetValue(unst, "status", "conditions")
	if ok {
		conditionsSlice := convert.ToMapSlice(conditions)
		for i := range conditionsSlice {
			data.RemoveValue(conditionsSlice[i], "error")
			data.RemoveValue(conditionsSlice[i], "transitioning")
			data.RemoveValue(conditionsSlice[i], "lastUpdateTime")
		}
		data.PutValue(unst, conditionsSlice, "status", "conditions")
	}
	obj.Object = unst
	return obj
}
