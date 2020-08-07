package keypair

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/harvester/pkg/api/store"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"
	"github.com/rancher/wrangler/pkg/schemas"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server) {
	t := schema.Template{
		ID: "vm.cattle.io.keypair",
		Store: store.KeyPairStore{
			Store: store.NamespaceStore{
				Store: proxy.NewProxyStore(server.ClientFactory, server.AccessSetLookup),
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
					KeyPairs:     scaled.VMFactory.Vm().V1alpha1().KeyPair(),
					KeyPairCache: scaled.VMFactory.Vm().V1alpha1().KeyPair().Cache(),
				},
			}
		},
	}
	server.SchemaTemplates = append(server.SchemaTemplates, t)
}
