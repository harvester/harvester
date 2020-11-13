package datavolume

import (
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"
)

const (
	dataVolumeSchemaID = "cdi.kubevirt.io.datavolume"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server) error {
	dvStore := &dvStore{
		Store:   proxy.NewProxyStore(server.ClientFactory, server.AccessSetLookup),
		dvCache: scaled.CDIFactory.Cdi().V1beta1().DataVolume().Cache(),
	}

	t := schema.Template{
		ID:    dataVolumeSchemaID,
		Store: dvStore,
	}

	server.SchemaTemplates = append(server.SchemaTemplates, t)
	return nil
}
