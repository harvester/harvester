package user

import (
	"sync"

	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"
)

const (
	userSchemaID = "harvester.cattle.io.user"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server) error {
	userStore := &userStore{
		mu:        sync.Mutex{},
		Store:     proxy.NewProxyStore(server.ClientFactory, server.AccessSetLookup),
		userCache: scaled.HarvesterFactory.Harvester().V1alpha1().User().Cache(),
	}

	t := schema.Template{
		ID:        userSchemaID,
		Store:     userStore,
		Formatter: formatter,
	}

	server.SchemaTemplates = append(server.SchemaTemplates, t)
	return nil
}
