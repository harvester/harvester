package keypair

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	controllerAgentName = "vm-keypair-controller"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	keyPairs := management.HarvesterFactory.Harvesterhci().V1beta1().KeyPair()
	controller := &Handler{
		keyPairClient: keyPairs,
	}

	keyPairs.OnChange(ctx, controllerAgentName, controller.OnKeyPairChanged)
	return nil
}
