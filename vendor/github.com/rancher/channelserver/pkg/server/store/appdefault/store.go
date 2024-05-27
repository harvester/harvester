package appdefault

import (
	"github.com/rancher/apiserver/pkg/store/empty"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/channelserver/pkg/config"
)

type Store struct {
	empty.Store
	config *config.Config
}

func New(config *config.Config) *Store {
	return &Store{
		config: config,
	}
}

func (c *Store) List(_ *types.APIRequest, _ *types.APISchema) (types.APIObjectList, error) {
	resp := types.APIObjectList{}
	for _, appDefault := range c.config.AppDefaultsConfig().AppDefaults {
		resp.Objects = append(resp.Objects, types.APIObject{
			Type:   "appdefault",
			ID:     appDefault.AppName,
			Object: appDefault,
		})
	}
	return resp, nil
}
