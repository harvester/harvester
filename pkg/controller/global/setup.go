package global

import (
	"context"

	"github.com/rancher/steve/pkg/server"

	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/controller/global/auth"
	"github.com/rancher/harvester/pkg/controller/global/settings"
	"github.com/rancher/harvester/pkg/controller/global/template"
	"github.com/rancher/harvester/pkg/indexeres"
)

type registerFunc func(context.Context, *config.Scaled, *server.Server) error

var registerFuncs = []registerFunc{
	settings.Register,
	template.Register,
	auth.Register,
}

func Setup(ctx context.Context, server *server.Server, controllers *server.Controllers) error {
	scaled := config.ScaledWithContext(ctx)
	indexeres.RegisterScaledIndexers(scaled)
	for _, f := range registerFuncs {
		if err := f(ctx, scaled, server); err != nil {
			return err
		}
	}
	return nil
}
