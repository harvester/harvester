package network

import (
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"

	"github.com/rancher/harvester/pkg/config"
)

const (
	nadSchemaID = "k8s.cni.cncf.io.networkattachmentdefinition"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	nadStore := &networkStore{
		Store:    proxy.NewProxyStore(server.ClientFactory, nil, server.AccessSetLookup),
		nadCache: scaled.CniFactory.K8s().V1().NetworkAttachmentDefinition().Cache(),
		vmCache:  scaled.VirtFactory.Kubevirt().V1().VirtualMachine().Cache(),
	}

	t := schema.Template{
		ID:    nadSchemaID,
		Store: nadStore,
	}

	server.SchemaFactory.AddTemplate(t)
	return nil
}
