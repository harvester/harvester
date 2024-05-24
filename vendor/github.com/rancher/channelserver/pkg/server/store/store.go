package store

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/store/empty"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/channelserver/pkg/config"
	"github.com/rancher/wrangler/pkg/schemas/validation"
)

type ChannelStore struct {
	empty.Store
	config *config.Config
}

func New(config *config.Config) *ChannelStore {
	return &ChannelStore{
		config: config,
	}
}

func (c *ChannelStore) List(_ *types.APIRequest, _ *types.APISchema) (types.APIObjectList, error) {
	resp := types.APIObjectList{}
	for _, channel := range c.config.ChannelsConfig().Channels {
		resp.Objects = append(resp.Objects, types.APIObject{
			Type:   "channel",
			ID:     channel.Name,
			Object: channel,
		})
	}
	return resp, nil
}

func (c *ChannelStore) ByID(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	redirect, err := c.config.Redirect(id)
	if err != nil {
		return types.APIObject{}, nil
	}
	if redirect != "" {
		http.Redirect(apiOp.Response, apiOp.Request, redirect, http.StatusFound)
		return types.APIObject{}, validation.ErrComplete
	}
	return c.Store.ByID(apiOp, schema, id)
}
