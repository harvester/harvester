package vmtemplate

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/harvester/pkg/api/store"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"
)

const (
	templateSchemaID        = "vm.cattle.io.template"
	templateVersionSchemaID = "vm.cattle.io.templateversion"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server) {
	templateCache := scaled.VMFactory.Vm().V1alpha1().TemplateVersion().Cache()
	th := &templateLinkHandler{
		templateVersionCache: templateCache,
	}

	templateVersionStore := &templateVersionStore{
		Store:                proxy.NewProxyStore(server.ClientFactory, server.AccessSetLookup),
		templateCache:        scaled.VMFactory.Vm().V1alpha1().Template().Cache(),
		templateVersionCache: templateCache,
	}

	t := []schema.Template{
		{
			ID:        templateSchemaID,
			Formatter: formatter,
			Customize: func(apiSchema *types.APISchema) {
				apiSchema.ByIDHandler = th.byIDHandler
			},
		},
		{
			ID:        templateVersionSchemaID,
			Formatter: versionFormatter,
			Store:     store.NamespaceStore{Store: templateVersionStore},
		},
	}

	server.SchemaTemplates = append(server.SchemaTemplates, t...)
}
