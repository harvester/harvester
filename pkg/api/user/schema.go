package user

import (
	"sync"

	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"

	"github.com/rancher/harvester/pkg/config"
)

const (
	userSchemaID = "harvesterhci.io.user"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	userStore := &userStore{
		mu:        sync.Mutex{},
		Store:     proxy.NewProxyStore(server.ClientFactory, nil, server.AccessSetLookup),
		userCache: scaled.HarvesterFactory.Harvesterhci().V1beta1().User().Cache(),
	}

	t := schema.Template{
		ID:        userSchemaID,
		Store:     userStore,
		Formatter: formatter,
	}

	server.SchemaFactory.AddTemplate(t)
	return nil
}
