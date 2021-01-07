package upgrade

import (
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"

	"github.com/rancher/harvester/pkg/config"
)

const (
	upgradeSchemaID = "harvester.cattle.io.upgrade"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	upgradeStore := &store{
		namespace:    options.Namespace,
		Store:        proxy.NewProxyStore(server.ClientFactory, nil, server.AccessSetLookup),
		upgradeCache: scaled.Management.HarvesterFactory.Harvester().V1alpha1().Upgrade().Cache(),
	}
	t := schema.Template{
		ID:    upgradeSchemaID,
		Store: upgradeStore,
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
