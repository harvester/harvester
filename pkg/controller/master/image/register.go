package image

import (
	"context"

	"github.com/rancher/harvester/pkg/config"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	RegisterController(ctx, management, options)
	return nil
}
