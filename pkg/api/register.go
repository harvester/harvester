package api

import (
	"context"

	"github.com/rancher/harvester/pkg/api/image"
	"github.com/rancher/harvester/pkg/api/keypair"
	"github.com/rancher/harvester/pkg/api/vm"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/steve/pkg/server"
)

func Setup(ctx context.Context, server *server.Server) error {
	scaled := config.ScaledWithContext(ctx)
	image.RegisterSchema(scaled, server)
	keypair.RegisterSchema(scaled, server)
	vm.RegisterSchema(scaled, server)
	return nil
}
