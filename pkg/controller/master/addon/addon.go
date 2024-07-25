package addon

import (
	"context"
	"fmt"
	"reflect"

	ctlhelmv1 "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	catalogv1 "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
	ctlappsv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io/v1"
	ctlbatchv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/relatedresource"
	"github.com/rancher/wrangler/v3/pkg/slice"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

const (
	managedChartKey = "catalog.cattle.io/managed"
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
	app   ctlappsv1.AppController
	pv    ctlcorev1.PersistentVolumeController
	pvc   ctlcorev1.PersistentVolumeClaimController
}

func Register(ctx context.Context, management *config.Management, _ config.Options) error {
	addonController := management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
	helmController := management.HelmFactory.Helm().V1().HelmChart()
	jobController := management.BatchFactory.Batch().V1().Job()
	appController := management.CatalogFactory.Catalog().V1().App()
	pvController := management.CoreFactory.Core().V1().PersistentVolume()
	pvcController := management.CoreFactory.Core().V1().PersistentVolumeClaim()

	h := &Handler{
		helm:  helmController,
		addon: addonController,
		job:   jobController,
		app:   appController,
		pv:    pvController,
		pvc:   pvcController,
	}

	addonController.OnChange(ctx, "monitor-addon-per-status", h.MonitorAddonPerStatus)
	addonController.OnChange(ctx, "monitor-addon-rancher-monitoring", h.MonitorAddonRancherMonitoring)
	appController.OnChange(ctx, "monitor-apps-catalog", h.PatchApps)
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

// PatchApps will watch apps.catalog.cattle.io and patch Charts.yaml with additional annotation 'catalog.cattle.io/managed'
// this is needed to ensure that addons cannot be upgrade directly from the apps with rancher mcm enabled on harvester
func (h *Handler) PatchApps(key string, app *catalogv1.App) (*catalogv1.App, error) {
	if app == nil || app.DeletionTimestamp != nil {
		return nil, nil
	}

	// check if harvester addon exists //
	var patchNeeded bool
	if slice.ContainsString(additionalApps, key) {
		patchNeeded = true
	} else {
		_, err := h.addon.Cache().Get(app.Namespace, app.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// ignoring app since it doesnt match an addon
				return app, nil
			}
			return app, fmt.Errorf("error fetching app %s: %v", app.Name, err)
		}
		patchNeeded = true
	}

	if patchNeeded {
		appCopy := app.DeepCopy()
		if appCopy.Spec.Chart.Metadata.Annotations == nil {
			appCopy.Spec.Chart.Metadata.Annotations = make(map[string]string)
		}
		appCopy.Spec.Chart.Metadata.Annotations[managedChartKey] = "true"
		if !reflect.DeepEqual(appCopy, app) {
			return h.app.Update(appCopy)
		}
	}

	return app, nil
}
