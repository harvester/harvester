package addon

import (
	"context"

	ctlhelmv1 "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/relatedresource"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

var (
	// aditionalApps contains list of apps which need to be annotated to managed
	// but are not related to a harvester addon
	additionalApps = []string{"cattle-system/rancher", "cattle-system/rancher-webhook"}
)

type Handler struct {
	helm  ctlhelmv1.HelmChartController
	addon ctlharvesterv1.AddonController
	job   ctlbatchv1.JobController
	pv    ctlcorev1.PersistentVolumeController
	pvc   ctlcorev1.PersistentVolumeClaimController
}

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	addonController := management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
	helmController := management.HelmFactory.Helm().V1().HelmChart()
	jobController := management.BatchFactory.Batch().V1().Job()
	pvController := management.CoreFactory.Core().V1().PersistentVolume()
	pvcController := management.CoreFactory.Core().V1().PersistentVolumeClaim()

	h := &Handler{
		helm:  helmController,
		addon: addonController,
		job:   jobController,
		pv:    pvController,
		pvc:   pvcController,
	}

	addonController.OnChange(ctx, "monitor-addon-per-status", h.MonitorAddonPerStatus)
	addonController.OnChange(ctx, "monitor-addon-rancher-monitoring", h.MonitorAddonRancherMonitoring)
	relatedresource.Watch(ctx, "watch-helmcharts", h.ReconcileHelmChartOwners, addonController, helmController)
	return nil
}

// MonitorAddonPerStatus will track the OnChange on addon, route actions per addon status
func (h *Handler) MonitorAddonPerStatus(_ string, aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	if aObj == nil || aObj.DeletionTimestamp != nil {
		return nil, nil
	}

	// if Addon is enabled process addon
	if aObj.Spec.Enabled {
		return h.processEnabledAddons(aObj)
	}

	// if Addon is disabled process disabling of addon
	return h.processDisabledAddons(aObj)
}
