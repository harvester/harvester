package datavolume

import (
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/steve/pkg/stores/proxy"

	"github.com/harvester/harvester/pkg/config"
)

const (
	dataVolumeSchemaID = "cdi.kubevirt.io.datavolume"
)

func RegisterSchema(scaled *config.Scaled, server *server.Server, options config.Options) error {
	dvStore := &dvStore{
		Store:   proxy.NewProxyStore(server.ClientFactory, nil, server.AccessSetLookup),
		dvCache: scaled.CDIFactory.Cdi().V1beta1().DataVolume().Cache(),
	}

	t := schema.Template{
		ID:    dataVolumeSchemaID,
		Store: dvStore,
	}

	server.SchemaFactory.AddTemplate(t)
	return nil
}
