package image

import (
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"

	"github.com/rancher/harvester/pkg/api/store"
	"github.com/rancher/harvester/pkg/config"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	t := schema.Template{
		ID: "harvester.cattle.io.virtualmachineimage",
		Store: store.DisplayNameValidatorStore{
			Store: store.NamespaceStore{
				Store:     proxy.NewProxyStore(server.ClientFactory, nil, server.AccessSetLookup),
				Namespace: options.Namespace,
			},
		},
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
