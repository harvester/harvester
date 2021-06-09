package data

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

// Init adds built-in resources
func Init(ctx context.Context, mgmtCtx *config.Management, options config.Options) error {
	if err := createCRDs(ctx, mgmtCtx.RestConfig); err != nil {
		return err
	}
	if err := createPublicNamespace(mgmtCtx); err != nil {
		return err
	}
	if err := createTemplates(mgmtCtx, publicNamespace); err != nil {
		return err
	}
	if err := createAPIService(mgmtCtx, options.Namespace); err != nil {
		return err
	}

	return nil
}
