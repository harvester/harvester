package image

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"
	"github.com/rancher/vm/pkg/config"
	"github.com/rancher/wrangler/pkg/schemas"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server) {
	t := schema.Template{
		ID:    "vm.cattle.io.image",
		Store: Store{next: proxy.NewProxyStore(server.ClientFactory, server.AccessSetLookup)},
		Customize: func(s *types.APISchema) {
			s.CollectionFormatter = CollectionFormatter
			s.CollectionActions = map[string]schemas.Action{
				"upload": {},
			}
			s.Formatter = Formatter
			s.ActionHandlers = map[string]http.Handler{
				"upload": UploadActionHandler{
					Images:     scaled.VMFactory.Vm().V1alpha1().Image(),
					ImageCache: scaled.VMFactory.Vm().V1alpha1().Image().Cache(),
				},
			}
		},
	}
	server.SchemaTemplates = append(server.SchemaTemplates, t)
}
