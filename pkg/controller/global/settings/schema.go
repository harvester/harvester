package settings

import (
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/steve/pkg/resources/common"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"
)

func setSchema(scaled *config.Scaled, server *server.Server) error {
	t := schema.Template{
		ID: "vm.cattle.io.setting",
		Store: store{
			Store: proxy.NewProxyStore(server.ClientFactory, server.AccessSetLookup),
		},
		Formatter: func(request *types.APIRequest, resource *types.RawResource) {
			common.Formatter(request, resource)
		},
		Customize: func(s *types.APISchema) {},
	}
	server.SchemaTemplates = append(server.SchemaTemplates, t)
	return nil
}
