package image

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/vm/pkg/config"
)

type Store struct {
	next types.Store
}

func (s Store) Create(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject) (types.APIObject, error) {
	ns := data.Data().String("metadata", "namespace")
	if ns == "" {
		data.Data().SetNested(config.Namespace, "metadata", "namespace")
	}
	return s.next.Create(apiOp, schema, data)
}

func (s Store) ByID(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	return s.next.ByID(apiOp, schema, id)
}

func (s Store) List(apiOp *types.APIRequest, schema *types.APISchema) (types.APIObjectList, error) {
	return s.next.List(apiOp, schema)
}

func (s Store) Update(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject, id string) (types.APIObject, error) {
	return s.next.Update(apiOp, schema, data, id)
}

func (s Store) Delete(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	return s.next.Delete(apiOp, schema, id)
}

func (s Store) Watch(apiOp *types.APIRequest, schema *types.APISchema, w types.WatchRequest) (chan types.APIEvent, error) {
	return s.next.Watch(apiOp, schema, w)
}
