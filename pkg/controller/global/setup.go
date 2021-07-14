package global

import (
	"context"

	"github.com/rancher/steve/pkg/server"
	corev1 "k8s.io/api/core/v1"

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
	if options.RancherEmbedded {
		registerControllers(ctx, scaled)
	}
	for _, f := range registerFuncs {
		if err := f(ctx, scaled, server, options); err != nil {
			return err
		}
	}
	return nil
}

func registerControllers(ctx context.Context, scaled *config.Scaled) {
	// to start secret controller so that it syncs in
	// https://github.com/rancher/dynamiclistener/blob/9b1b7d3132e8b0dad6493f257386aa3bfb7b69f8/storage/kubernetes/controller.go#L78
	scaled.CoreFactory.Core().V1().Secret().OnChange(ctx, "harvester-tls-storage",
		func(key string, secret *corev1.Secret) (*corev1.Secret, error) {
			return secret, nil
		})
}
