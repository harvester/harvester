package image

import (
	"context"

	"github.com/rancher/vm/pkg/config"
	"github.com/rancher/vm/pkg/util"
)

func Register(ctx context.Context, management *config.Management) error {
	RegisterController(ctx, management)
	return util.InitMinio()
}
