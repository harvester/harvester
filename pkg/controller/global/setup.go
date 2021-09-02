package global

import (
	"context"

	"github.com/rancher/steve/pkg/server"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/controller/global/settings"
	"github.com/harvester/harvester/pkg/indexeres"
)

type registerFunc func(context.Context, *config.Scaled, *server.Server, config.Options) error

var registerFuncs = []registerFunc{
	settings.Register,
}

func Setup(ctx context.Context, server *server.Server, controllers *server.Controllers, options config.Options) error {
	scaled := config.ScaledWithContext(ctx)
	indexeres.RegisterScaledIndexers(scaled)
	for _, f := range registerFuncs {
		if err := f(ctx, scaled, server, options); err != nil {
			return err
		}
	}
	return nil
}
