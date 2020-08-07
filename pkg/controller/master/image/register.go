package image

import (
	"context"

	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/util"
)

func Register(ctx context.Context, management *config.Management) error {
	RegisterController(ctx, management)
	return util.InitMinio()
}
