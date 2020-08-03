package keypair

import (
	"context"

	"github.com/rancher/vm/pkg/config"
)

const (
	controllerAgentName = "vm-keypair-controller"
)

func Register(ctx context.Context, management *config.Management) error {
	keyPairs := management.VMFactory.Vm().V1alpha1().KeyPair()
	controller := &Handler{
		keyPairs:     keyPairs,
		keyPairCache: keyPairs.Cache(),
	}

	keyPairs.OnChange(ctx, controllerAgentName, controller.OnKeyPairChanged)
	return nil
}
