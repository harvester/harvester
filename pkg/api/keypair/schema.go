package keypair

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"
	"github.com/rancher/wrangler/pkg/schemas"

	"github.com/harvester/harvester/pkg/api/store"
	"github.com/harvester/harvester/pkg/config"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	t := schema.Template{
		ID: "harvesterhci.io.keypair",
		Store: store.KeyPairStore{
			Store: store.NamespaceStore{
				Store:     proxy.NewProxyStore(server.ClientFactory, nil, server.AccessSetLookup),
				Namespace: options.Namespace,
			},
		},
		Customize: func(s *types.APISchema) {
			s.CollectionFormatter = CollectionFormatter
			s.CollectionActions = map[string]schemas.Action{
				"keygen": {},
			}
			s.Formatter = Formatter
			s.ActionHandlers = map[string]http.Handler{
				"keygen": KeyGenActionHandler{
					KeyPairs:     scaled.HarvesterFactory.Harvesterhci().V1beta1().KeyPair(),
					KeyPairCache: scaled.HarvesterFactory.Harvesterhci().V1beta1().KeyPair().Cache(),
					Namespace:    options.Namespace,
				},
			}
		},
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
