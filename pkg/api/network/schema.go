package network

import (
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"
)

const (
	nadSchemaID = "k8s.cni.cncf.io.networkattachmentdefinition"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server) error {
	nadStore := &networkStore{
		Store:    proxy.NewProxyStore(server.ClientFactory, server.AccessSetLookup),
		nadCache: scaled.CniFactory.K8s().V1().NetworkAttachmentDefinition().Cache(),
	}

	t := schema.Template{
		ID:    nadSchemaID,
		Store: nadStore,
	}

	server.SchemaTemplates = append(server.SchemaTemplates, t)
	return nil
}
