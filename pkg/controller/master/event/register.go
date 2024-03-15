package event

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const (
	eventControllerSyncEvent = "EventController.SyncEvent"
)

func Register(ctx context.Context, management *config.Management, options config.Options) error {
	var (
		vmClient    = management.VirtFactory.Kubevirt().V1().VirtualMachine()
		vmCache     = management.VirtFactory.Kubevirt().V1().VirtualMachine().Cache()
		eventClient = management.CoreFactory.Core().V1().Event()
	)

	eventCtrl := &Controller{
		vmClient:    vmClient,
		vmCache:     vmCache,
		eventClient: eventClient,
	}

	eClient := management.CoreFactory.Core().V1().Event()
	eClient.OnChange(ctx, eventControllerSyncEvent, eventCtrl.SyncEvent)

	return nil
}
