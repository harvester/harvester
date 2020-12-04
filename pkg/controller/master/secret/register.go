package secret

import (
	"context"

	"github.com/rancher/harvester/pkg/config"
)

// Register registers the secret controller
func Register(ctx context.Context, management *config.Management) error {
	secrets := management.CoreFactory.Core().V1().Secret()
	controller := &Handler{
		secretCache: secrets.Cache(),
		secrets:     secrets,
	}

	secrets.OnChange(ctx, controllerName, controller.OnSecretChanged)
	return nil
}
