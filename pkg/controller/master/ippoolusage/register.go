package ippoolusage

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const ControllerName = "harvester-ippoolusage-controller"

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	ipPoolUsages := management.HarvesterFactory.Harvesterhci().V1beta1().IPPoolUsage()

	handler := &Handler{
		ipPoolUsages: ipPoolUsages,
	}

	ipPoolUsages.OnChange(ctx, ControllerName, handler.OnChange)
	return nil
}
