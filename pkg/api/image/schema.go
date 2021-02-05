package image

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"
	"github.com/rancher/wrangler/pkg/schemas"

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
		Customize: func(s *types.APISchema) {
			s.ResourceActions = map[string]schemas.Action{
				actionUpload: {},
			}
			s.ActionHandlers = map[string]http.Handler{
				actionUpload: UploadActionHandler{
					Images:     scaled.HarvesterFactory.Harvester().V1alpha1().VirtualMachineImage(),
					ImageCache: scaled.HarvesterFactory.Harvester().V1alpha1().VirtualMachineImage().Cache(),
					Options:    options,
				},
			}
		},
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
