package store

import (
	"github.com/rancher/apiserver/pkg/types"

	"github.com/rancher/harvester/pkg/config"
)

type NamespaceStore struct {
	types.Store
}

func (s NamespaceStore) Create(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject) (types.APIObject, error) {
	ns := data.Data().String("metadata", "namespace")
	if ns == "" {
		data.Data().SetNested(config.Namespace, "metadata", "namespace")
	}
	return s.Store.Create(apiOp, schema, data)
}
