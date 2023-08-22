package data

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

// Init adds built-in resources, the `basic` part, which does not rely on webhook
func InitBasicData(ctx context.Context, mgmtCtx *config.Management, options config.Options) error {
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

	// createTemplates and createSecrets are called later in InitAdditional
	return nil
}

// Init adds built-in resources, the `additional` part, which relies on webhook
// Called after the `harvester-webhook` is ready
func InitAdditionalData(ctx context.Context, mgmtCtx *config.Management, options config.Options) error {
	// Not applying the built-in templates and secrets in case users have edited them.
	if err := createTemplates(mgmtCtx, publicNamespace); err != nil {
		return err
	}
	if err := createSecrets(mgmtCtx); err != nil {
		return err
	}
	return nil
}
