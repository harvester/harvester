package cluster

import (
	"fmt"

	"github.com/rancher/apiserver/pkg/types"
)

type Store struct {
	types.Store
}

func (f *Store) ByID(_ *types.APIRequest, _ *types.APISchema, id string) (types.APIObject, error) {
	if id != localClusterName {
		return types.APIObject{}, fmt.Errorf("only supported cluster id is local")
	}
	return types.APIObject{
		Type: "cluster",
		ID:   "local",
		Object: Cluster{
			Name: "local",
		},
	}, nil
}

func (f *Store) List(_ *types.APIRequest, _ *types.APISchema) (types.APIObjectList, error) {
	return types.APIObjectList{
		Objects: []types.APIObject{
			{
				Type: "cluster",
				ID:   "local",
				Object: Cluster{
					Name: "local",
				},
			},
		},
	}, nil
}
