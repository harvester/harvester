package upgrade

import (
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"

	"github.com/rancher/harvester/pkg/config"
)

const (
	upgradeSchemaID = "harvesterhci.io.upgrade"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	upgradeStore := &store{
		namespace:    options.Namespace,
		Store:        proxy.NewProxyStore(server.ClientFactory, nil, server.AccessSetLookup),
		upgradeCache: scaled.Management.HarvesterFactory.Harvesterhci().V1beta1().Upgrade().Cache(),
	}
	t := schema.Template{
		ID:    upgradeSchemaID,
		Store: upgradeStore,
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
