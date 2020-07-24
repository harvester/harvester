package api

import (
	"context"
	"github.com/rancher/vm/pkg/api/image"

	"github.com/rancher/steve/pkg/server"
	"github.com/rancher/vm/pkg/config"
)

func Setup(ctx context.Context, server *server.Server) error {
	scaled := config.ScaledWithContext(ctx)
	image.RegisterSchema(scaled, server)
	return nil
}
