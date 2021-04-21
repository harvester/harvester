package image

import (
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"

	"github.com/harvester/harvester/pkg/api/store"
	"github.com/harvester/harvester/pkg/config"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	t := schema.Template{
		ID: "harvesterhci.io.virtualmachineimage",
		Store: store.DisplayNameValidatorStore{
			Store: proxy.NewProxyStore(server.ClientFactory, nil, server.AccessSetLookup),
		},
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
