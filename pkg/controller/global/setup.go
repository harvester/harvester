package global

import (
	"context"

	"github.com/rancher/steve/pkg/server"

	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/controller/global/settings"
)

type registerFunc func(context.Context, *config.Scaled, *server.Server) error

var registerFuncs = []registerFunc{
	settings.Register,
}

func Setup(ctx context.Context, server *server.Server) error {
	scaled := config.ScaledWithContext(ctx)
	for _, f := range registerFuncs {
		if err := f(ctx, scaled, server); err != nil {
			return err
		}
	}
	return nil
}
