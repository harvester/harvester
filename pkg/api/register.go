package api

import (
	"context"

	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/vm/pkg/api/image"
	"github.com/rancher/vm/pkg/api/keypair"
	"github.com/rancher/vm/pkg/api/vm"
	"github.com/rancher/vm/pkg/config"
)

func Setup(ctx context.Context, server *server.Server) error {
	scaled := config.ScaledWithContext(ctx)
	image.RegisterSchema(scaled, server)
	keypair.RegisterSchema(scaled, server)
	vm.RegisterSchema(scaled, server)
	return nil
}
