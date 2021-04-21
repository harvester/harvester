package vmtemplate

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"

	"github.com/harvester/harvester/pkg/config"
)

const (
	templateSchemaID        = "harvesterhci.io.virtualmachinetemplate"
	templateVersionSchemaID = "harvesterhci.io.virtualmachinetemplateversion"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	templates := scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplate()
	templateVersionCache := scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplateVersion().Cache()
	th := &templateLinkHandler{
		templateVersionCache: templateVersionCache,
	}

	templateVersionStore := &templateVersionStore{
		Store:                proxy.NewProxyStore(server.ClientFactory, nil, server.AccessSetLookup),
		templateCache:        templates.Cache(),
		templateVersionCache: templateVersionCache,
		keyPairCache:         scaled.HarvesterFactory.Harvesterhci().V1beta1().KeyPair().Cache(),
	}

	t := []schema.Template{
		{
			ID:        templateSchemaID,
			Formatter: formatter,
			Customize: func(apiSchema *types.APISchema) {
				apiSchema.ByIDHandler = th.byIDHandler
			},
			Store: proxy.NewProxyStore(server.ClientFactory, nil, server.AccessSetLookup),
		},
		{
			ID:        templateVersionSchemaID,
			Formatter: versionFormatter,
			Store:     templateVersionStore,
		},
	}

	server.SchemaFactory.AddTemplate(t...)
	return nil
}
