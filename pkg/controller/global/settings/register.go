package settings

import (
	"context"

	"github.com/rancher/steve/pkg/server"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/settings"
)

func Register(ctx context.Context, scaled *config.Scaled, server *server.Server, options config.Options) error {
	sp := settings.NewSettingsProvider(
		ctx,
		scaled.HarvesterFactory.Harvesterhci().V1beta1().Setting(),
		scaled.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache(),
	)

	if err := settings.SetProvider(&sp); err != nil {
		return err
	}

	return nil
}
