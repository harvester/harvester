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
	if err := addFleetDefaultNamespace(mgmtCtx.Apply); err != nil {
		return err
	}
	if err := addAPIService(mgmtCtx.Apply, options.Namespace); err != nil {
		return err
	}
	if err := addAuthenticatedRoles(mgmtCtx.Apply); err != nil {
		return err
	}

	// Not applying the built-in templates and secrets in case users have edited them.
	if err := createTemplates(mgmtCtx, publicNamespace); err != nil {
		return err
	}
	return createSecrets(mgmtCtx)
}
