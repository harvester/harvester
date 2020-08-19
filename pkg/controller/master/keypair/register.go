package keypair

import (
	"context"

	"github.com/rancher/harvester/pkg/config"
)

const (
	controllerAgentName = "vm-keypair-controller"
)

func Register(ctx context.Context, management *config.Management) error {
	keyPairs := management.HarvesterFactory.Harvester().V1alpha1().KeyPair()
	controller := &Handler{
		keyPairClient: keyPairs,
	}

	keyPairs.OnChange(ctx, controllerAgentName, controller.OnKeyPairChanged)
	return nil
}
