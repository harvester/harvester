package global

import (
	"context"

	"github.com/rancher/steve/pkg/server"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/controller/global/settings"
)

type registerFunc func(context.Context, *config.Scaled, *server.Server, config.Options) error

var registerFuncs = []registerFunc{
	settings.Register,
}

func Setup(ctx context.Context, server *server.Server, _ *server.Controllers, options config.Options) error {
	scaled := config.ScaledWithContext(ctx)

	for _, f := range registerFuncs {
		if err := f(ctx, scaled, server, options); err != nil {
			return err
		}
	}
	return nil
}
