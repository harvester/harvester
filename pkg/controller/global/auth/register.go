package auth

import (
	"context"

	"github.com/rancher/steve/pkg/server"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/settings"
)

func Register(ctx context.Context, scaled *config.Scaled, server *server.Server, options config.Options) error {
	go WatchSecret(ctx, scaled, options.Namespace, settings.AuthSecretName.Get())
	return nil
}
