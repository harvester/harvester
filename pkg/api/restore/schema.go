package restore

import (
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"

	"github.com/rancher/harvester/pkg/config"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	vmRestore := &vmRestoreStore{
		Store:   proxy.NewProxyStore(server.ClientFactory, nil, server.AccessSetLookup),
		vmCache: scaled.VirtFactory.Kubevirt().V1().VirtualMachine().Cache(),
	}
	t := schema.Template{
		ID:    "harvester.cattle.io.virtualmachinerestore",
		Store: vmRestore,
	}
	server.SchemaFactory.AddTemplate(t)
	return nil
}
