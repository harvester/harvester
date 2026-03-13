package sharemanager

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/pkg/settings"
)

const ControllerName = "harvester-share-manager-static-ip-controller"

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	shareManagers := management.LonghornFactory.Longhorn().V1beta2().ShareManager()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	nads := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()
	ipPoolUsages := management.HarvesterFactory.Harvesterhci().V1beta1().IPPoolUsage()

	handler := &Handler{
		shareManagers:          shareManagers,
		shareManagerController: shareManagers,
		shareManagerCache:      shareManagers.Cache(),
		settingsCache:          settings.Cache(),
		nads:                   nads,
		nadCache:               nads.Cache(),
		ipPoolUsages:           ipPoolUsages,
		ipPoolUsageCache:       ipPoolUsages.Cache(),
	}

	settings.OnChange(ctx, ControllerName+"-storage-network-setting", handler.OnStorageNetworkChange)
	shareManagers.OnChange(ctx, ControllerName, handler.OnShareManagerChange)
	shareManagers.OnRemove(ctx, ControllerName, handler.OnShareManagerRemove)
	return nil
}

func isStorageNetworkSetting(name string) bool {
	return name == settings.StorageNetworkName
}
