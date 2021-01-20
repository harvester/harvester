package store

import (
	"github.com/rancher/apiserver/pkg/types"
)

type NamespaceStore struct {
	types.Store
	Namespace string
}

func (s NamespaceStore) Create(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject) (types.APIObject, error) {
	ns := data.Data().String("metadata", "namespace")
	if ns == "" {
		data.Data().SetNested(s.Namespace, "metadata", "namespace")
	}
	return s.Store.Create(apiOp, schema, data)
}
