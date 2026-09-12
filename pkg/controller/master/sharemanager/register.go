package sharemanager

import (
	"context"

	"github.com/harvester/harvester/pkg/config"
)

const ControllerName = "share-manager-static-ip"

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	shareManagers := management.LonghornFactory.Longhorn().V1beta2().ShareManager()
	settings := management.HarvesterFactory.Harvesterhci().V1beta1().Setting()
	nads := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()
	pods := management.CoreFactory.Core().V1().Pod()

	handler := &Handler{
		shareManagers:          shareManagers,
		shareManagerController: shareManagers,
		shareManagerCache:      shareManagers.Cache(),
		podCache:               pods.Cache(),
		settingsController:     settings,
		settingsCache:          settings.Cache(),
		nads:                   nads,
		nadCache:               nads.Cache(),
	}

	// Keep this internal default opt-in flow isolated from the main static IP reconciliation logic.
	settings.OnChange(ctx, ControllerName+"-rwx-network-default-opt-in", handler.OnRWXNetworkDefaultOptIn)
	shareManagers.OnChange(ctx, ControllerName+"-default-opt-in", handler.OnShareManagerDefaultOptIn)
	pods.OnRemove(ctx, ControllerName+"-pod", handler.OnPodRemove)

	settings.OnChange(ctx, ControllerName+"-rwx-network-setting", handler.OnRWXNetworkChange)
	shareManagers.OnChange(ctx, ControllerName, handler.OnShareManagerChange)
	shareManagers.OnRemove(ctx, ControllerName, handler.OnShareManagerRemove)
	pods.OnChange(ctx, ControllerName+"-pod", handler.OnPodChange)
	return nil
}
