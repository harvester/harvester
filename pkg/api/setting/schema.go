package setting

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/resources/common"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"

	"github.com/rancher/harvester/pkg/config"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server) error {
	t := schema.Template{
		ID:    "harvester.cattle.io.setting",
		Store: proxy.NewProxyStore(server.ClientFactory, server.AccessSetLookup),
		Formatter: func(request *types.APIRequest, resource *types.RawResource) {
			common.Formatter(request, resource)
		},
		Customize: func(s *types.APISchema) {},
	}
	server.SchemaTemplates = append(server.SchemaTemplates, t)
	return nil
}
