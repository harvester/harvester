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

	if err := addPublicNamespace(mgmtCtx.Apply); err != nil {
		return err
	}
	if err := addAPIService(mgmtCtx.Apply, options.Namespace); err != nil {
		return err
	}
	if err := addAuthenticatedRoles(mgmtCtx.Apply); err != nil {
		return err
	}

	// Not applying the built-in templates in case users have edited them.
	return createTemplates(mgmtCtx, publicNamespace)
}
