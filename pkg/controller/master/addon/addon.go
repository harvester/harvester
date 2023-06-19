package addon

import (
	"context"
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"
	"time"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	ctlhelmv1 "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	"github.com/rancher/norman/types/slice"
	catalogv1 "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
	ctlappsv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io/v1"
	ctlbatchv1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/relatedresource"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
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

func Register(ctx context.Context, management *config.Management, opts config.Options) error {
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

	addonController.OnChange(ctx, "deploy-addon", h.OnAddonChange)
	addonController.OnChange(ctx, "monitor-addon-enabled", h.MonitorAddonEnabled)
	addonController.OnChange(ctx, "monitor-addon-disabled", h.MonitorAddonDisabled)
	addonController.OnChange(ctx, "monitor-changes", h.MonitorChanges)
	addonController.OnChange(ctx, "monitor-addon-rancher-monitoring", h.MonitorAddonRancherMonitoring)
	appController.OnChange(ctx, "monitor-apps-catalog", h.PatchApps)
	relatedresource.Watch(ctx, "watch-helmcharts", h.ReconcileHelmChartOwners, addonController, helmController)
	return nil
}

// MonitorChanges will trigger updates to helm chart based on changes in values
func (h *Handler) MonitorChanges(key string, aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	if aObj == nil || aObj.DeletionTimestamp != nil {
		return nil, nil
	}

	a := aObj.DeepCopy()
	if a.Status.Status != harvesterv1.AddonDeployed && a.Status.Status != harvesterv1.AddonFailed {
		return a, nil
	}

	hc, redeployChart, err := h.redeployNeeded(a)
	if err != nil {
		return a, err
	}

	if redeployChart {
		logrus.Infof("redeploy chart %s, current status %s", a.Name, a.Status.Status)
		a.Status.DeepCopy()
		a.Status.Status = ""
		return h.addon.UpdateStatus(a)
	}

	vals, err := defaultValues(a)
	if err != nil {
		return a, fmt.Errorf("error generating default values during monitor changes: %v", err)
	}

	if hc.Spec.ValuesContent != vals || hc.Spec.Version != a.Spec.Version || hc.Spec.Chart != a.Spec.Chart || hc.Spec.Repo != a.Spec.Repo {
		hc.Spec.Chart = a.Spec.Chart
		hc.Spec.Repo = a.Spec.Repo
		hc.Spec.Version = a.Spec.Version
		hc.Spec.ValuesContent = vals
		logrus.Infof("addon %s has changed, upgrading helm chart %s", a.Name, hc.Name)
		_, err = h.helm.Update(hc)
		if err != nil {
			return a, fmt.Errorf("error updating helm chart during monitor changes: %v", err)
		}
		// reset status to AddonEnable to trigger reconcile for job
		a.Status.Status = harvesterv1.AddonEnabled
		return h.addon.UpdateStatus(a)
	}

	return a, nil
}

// MonitorAddon will track the deployment of Addon
func (h *Handler) MonitorAddonEnabled(key string, aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	if aObj == nil || aObj.DeletionTimestamp != nil {
		return nil, nil
	}

	if !aObj.Spec.Enabled {
		return aObj, nil
	}

	// if AddonDeployed or AddonFailed, return
	if aObj.Status.Status != harvesterv1.AddonEnabled {
		// logrus.Infof("addon %s status is", aObj.Name, aObj.Status.Status)
		return aObj, nil
	}

	hc, err := h.helm.Cache().Get(aObj.Namespace, aObj.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("waiting for helmchart %s to be created", aObj.Name)
			h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
			return aObj, nil
		}

		return aObj, fmt.Errorf("error querying helmchart %v", err)
	}

	if hc.Status.JobName == "" {
		logrus.Infof("waiting for create jobname to be populated in helmchart status")
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	if strings.HasPrefix(hc.Status.JobName, "helm-delete-") {
		logrus.Infof("the fetched job %s is from last time helmdelete, wait install job", hc.Status.JobName)
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	job, err := h.job.Get(aObj.Namespace, hc.Status.JobName, metav1.GetOptions{})
	if err != nil {
		logrus.Infof("waiting for job %s to start", job.Name)
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	if !isJobComplete(job) {
		logrus.Infof("waiting for job %s to complete", job.Name)
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	logrus.Infof("addon job %s has finished", job.Name)

	a := aObj.DeepCopy()

	// check Failed before since in jobs with more than 1 container then
	// even a single failure should count as a failure
	if job.Status.Failed > 0 {
		a.Status.Status = harvesterv1.AddonFailed
	} else if job.Status.Succeeded > 0 {
		// a.Status.Status = harvesterv1.AddonDeployed
		a.Status.Status = harvesterv1.AddonDeployed
	}

	logrus.Infof("addon %s will be set to new status %s", a.Name, a.Status.Status)

	return h.addon.UpdateStatus(a)
}

// MonitorAddon will track the deployment of Addon
func (h *Handler) MonitorAddonDisabled(key string, aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	if aObj == nil || aObj.DeletionTimestamp != nil {
		return nil, nil
	}

	if aObj.Spec.Enabled {
		return aObj, nil
	}

	// has already been successful
	if aObj.Status.Status == "" {
		return aObj, nil
	}

	hc, err := h.helm.Cache().Get(aObj.Namespace, aObj.Name)
	if err != nil {
		// disabled
		if apierrors.IsNotFound(err) {
			if aObj.Status.Status == "" {
				logrus.Infof("addon %s: helm chart is gone, addon has already been in init status", aObj.Name)
				return aObj, nil
			}

			// move status to nil
			logrus.Infof("addon %s: helm chart is gone, addon is in %s status, move to init status", aObj.Name, aObj.Status.Status)

			a := aObj.DeepCopy()
			a.Status.Status = ""

			return h.addon.UpdateStatus(a)
		}

		return aObj, fmt.Errorf("error querying helmchart %v", err)
	}

	if hc.Status.JobName == "" {
		logrus.Infof("waiting for delete jobname to be populated in HelmChart status")
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		//return aObj, fmt.Errorf("waiting for jobname to be populated in HelmChart status")
		return aObj, nil
	}

	if strings.HasPrefix(hc.Status.JobName, "helm-install-") {
		logrus.Infof("the fetched job %s is from last time hell install, wait delete job", hc.Status.JobName)
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	job, err := h.job.Get(aObj.Namespace, hc.Status.JobName, metav1.GetOptions{})
	if err != nil {
		logrus.Infof("waiting for job %s to start", job.Name)
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	// apply condition //
	if !isJobComplete(job) {
		logrus.Infof("waiting for job %s to complete", job.Name)
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		//return aObj, fmt.Errorf("waiting for jobname to be populated in HelmChart status")
		return aObj, nil
		//return aObj, fmt.Errorf("waiting for job %s to complete", job.Name)
	}

	logrus.Infof("addon job %s has finished", job.Name)

	a := aObj.DeepCopy()
	// check Failed before since in jobs with more than 1 container then
	// even a single failure should count as a failure
	if job.Status.Failed > 0 {
		a.Status.Status = harvesterv1.AddonDisableFaild
	} else if job.Status.Succeeded > 0 {
		// back to init
		a.Status.Status = ""
	}

	logrus.Infof("addon %s will be set to new status %s", a.Name, a.Status.Status)

	return h.addon.UpdateStatus(a)
}

// MonitorAddonRancherMonitoring will track status change of rancher-monitroing addon
func (h *Handler) MonitorAddonRancherMonitoring(key string, aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	if aObj == nil || aObj.DeletionTimestamp != nil {
		return nil, nil
	}

	if aObj.Name != util.RancherMonitoringName || aObj.Namespace != util.CattleMonitoringSystemNamespaceName {
		return aObj, nil
	}

	// deployed successfully
	if aObj.Spec.Enabled && aObj.Status.Status == harvesterv1.AddonDeployed {
		// patch pv ReclaimPolicy
		_, pvname, err := h.patchGrafanaPVReclaimPolicy(util.CattleMonitoringSystemNamespaceName, util.GrafanaPVCName)

		if err != nil {
			return aObj, err
		}

		// annotate pv name
		if pvname != "" {
			return h.annotateGrafanaPVName(pvname, aObj)
		}
	}

	// disabled successfully, the pv has already from Bound to Released/Available
	if !aObj.Spec.Enabled && aObj.Status.Status == "" {
		logrus.Infof("start to patch grafana pv ClaimRef")
		return h.patchGrafanaPVClaimRef(aObj)
	}

	return aObj, nil
}

// OnAddonChange will reconcile the Addon CRDs
func (h *Handler) OnAddonChange(key string, a *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	if a == nil || a.DeletionTimestamp != nil {
		return nil, nil
	}

	// if addon is enabled reconcile HelmChart
	ok, err := h.checkHelmChartExists(a)
	if err != nil {
		return a, fmt.Errorf("error during lookup of helm chart: %v", err)
	}

	if a.Spec.Enabled && !ok {
		return h.deployHelmChart(a)
	}

	if !a.Spec.Enabled && ok {
		return h.removeHelmChart(a)
	}

	return nil, nil
}

func (h *Handler) checkHelmChartExists(a *harvesterv1.Addon) (bool, error) {
	_, err := h.helm.Cache().Get(a.Namespace, a.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (h *Handler) deployHelmChart(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	a := aObj.DeepCopy()
	vals, err := defaultValues(a)
	if err != nil {
		return a, err
	}

	hc := &helmv1.HelmChart{
		ObjectMeta: metav1.ObjectMeta{
			Name:      a.Name,
			Namespace: a.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: a.APIVersion,
					Kind:       a.Kind,
					Name:       a.Name,
					UID:        a.UID,
				},
			},
		},
		Spec: helmv1.HelmChartSpec{
			Chart:         a.Spec.Chart,
			Repo:          a.Spec.Repo,
			ValuesContent: vals,
			Version:       a.Spec.Version,
		},
	}
	_, err = h.helm.Create(hc)
	if err != nil {
		return a, fmt.Errorf("error creating helm chart object %v", err)
	}

	a.Status.Status = harvesterv1.AddonEnabled
	return h.addon.UpdateStatus(a)
}

// checkAndDeleteChart will remove the helmChart only if the HelmChart is owned by the Addon
// bool:  true: delete/deleting; false: noting to do
func (h *Handler) checkAndDeleteChart(a *harvesterv1.Addon) (bool, error) {
	hc, err := h.helm.Cache().Get(a.Namespace, a.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// chart doesn't exist. Nothing to do
			return false, nil
		}
		return false, fmt.Errorf("error looking up chart in checkAndDeleteChart: %v", err)
	}

	var addonOwned bool
	for _, v := range hc.GetOwnerReferences() {
		if v.Kind == a.Kind && v.APIVersion == a.APIVersion && v.UID == a.UID && v.Name == a.Name {
			addonOwned = true
			break
		}
	}

	if addonOwned {
		logrus.Infof("delete the helm chart %s/%s now", hc.Namespace, hc.Name)
		if hc.DeletionTimestamp != nil {
			logrus.Infof("the charting is being deleted, wait")
			return true, nil
		}
		return true, h.helm.Delete(hc.Namespace, hc.Name, &metav1.DeleteOptions{})
	}

	logrus.Infof("did not get target helm chart to delete")

	return false, nil
}

func defaultValues(a *harvesterv1.Addon) (string, error) {
	if a.Spec.ValuesContent != "" {
		return a.Spec.ValuesContent, nil
	}

	valsEncoded, ok := a.Annotations[util.AddonValuesAnnotation]
	if ok {
		valByte, err := base64.StdEncoding.DecodeString(valsEncoded)
		if err != nil {
			return "", fmt.Errorf("error decoding addon defaults: %v", err)
		}

		return string(valByte), nil
	}
	// no overrides. Use packaged chart defaults
	return "", nil
}

func (h *Handler) ReconcileHelmChartOwners(_ string, _ string, obj runtime.Object) ([]relatedresource.Key, error) {
	if hc, ok := obj.(*helmv1.HelmChart); ok {
		for _, v := range hc.GetOwnerReferences() {
			if strings.ToLower(v.Kind) == "addon" {
				return []relatedresource.Key{
					{
						Name:      v.Name,
						Namespace: hc.Namespace,
					},
				}, nil
			}
		}
	}

	return nil, nil
}

func (h *Handler) removeHelmChart(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	// a := aObj.DeepCopy()
	d, err := h.checkAndDeleteChart(aObj)
	if err != nil {
		return aObj, fmt.Errorf("error removing helm chart for addon %s: %v", aObj.Name, err)
	}

	// delete or deleting, make sure status is AddonDisabling
	if d {
		if aObj.Status.Status != harvesterv1.AddonDisabling {
			a := aObj.DeepCopy()

			logrus.Infof("addon %s is being disabled now, update status from %s to %s", a.Name, a.Status.Status, harvesterv1.AddonDisabling)
			a.Status.Status = harvesterv1.AddonDisabling
			return h.addon.UpdateStatus(a)
		}
	}

	// who cleared it ? wait

	logrus.Infof("addon's status is: %s", aObj.Status.Status)

	// may be failed

	return aObj, nil
}

func isJobComplete(j *batchv1.Job) bool {
	if j.Status.CompletionTime != nil {
		return true
	}

	for _, v := range j.Status.Conditions {
		if v.Type == batchv1.JobFailed && v.Reason == "BackoffLimitExceeded" {
			return true
		}
	}

	return false
}

func (h *Handler) redeployNeeded(a *harvesterv1.Addon) (*helmv1.HelmChart, bool, error) {
	var redeployChart bool
	hc, err := h.helm.Cache().Get(a.Namespace, a.Name)
	if err != nil {
		// HelmChart is missing. Reset Addon to allow normal reconcile
		if apierrors.IsNotFound(err) {
			redeployChart = true
		} else {
			return nil, false, fmt.Errorf("error fetching helm chart during monitor changes: %v", err)
		}
	}

	if hc != nil && hc.DeletionTimestamp != nil {
		redeployChart = true
	}

	return hc, redeployChart, nil
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

// return: bool: patched;  string: pv-name; error
func (h *Handler) patchGrafanaPVReclaimPolicy(namespace, name string) (bool, string, error) {
	pvc, err := h.pvc.Cache().Get(namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// grafana may not enable pvc
			logrus.Infof("grafana PVC %s/%s is not found, skip patch ReclaimPolicy", namespace, name)
			return false, "", nil
		}

		return false, "", fmt.Errorf("error fetching grafana pvc %s/%s: %v", namespace, name, err)
	}

	// pv name format: volumeName: pvc-a95cdf6b-26bc-4ed6-8aaf-5f5d6b0b6dbd
	pv, err := h.pv.Cache().Get(pvc.Spec.VolumeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("grafana PV %s is not found, retry", pvc.Spec.VolumeName)
			return false, "", fmt.Errorf("grafana PV %s is not found, retry", pvc.Spec.VolumeName)
		}

		return false, "", fmt.Errorf("error fetching grafana pv %s: %v", pvc.Spec.VolumeName, err)
	}

	// already be PersistentVolumeReclaimRetain PersistentVolumeReclaimPolicy = "Retain"
	if pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimRetain {
		logrus.Infof("grafana PV %s ReclaimPolicy has already been %s", pv.Name, corev1.PersistentVolumeReclaimRetain)
		return false, pvc.Spec.VolumeName, nil
	}

	pvCopy := pv.DeepCopy()
	pvCopy.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimRetain
	_, err = h.pv.Update(pvCopy)
	if err != nil {
		return false, pvc.Spec.VolumeName, fmt.Errorf("grafana PV %s is failed to patch ReclaimPolicy: %v", pvc.Spec.VolumeName, err)
	}

	logrus.Infof("grafana PV %s is patched with new ReclaimPolicy %s", pvc.Spec.VolumeName, corev1.PersistentVolumeReclaimRetain)
	return true, pvc.Spec.VolumeName, nil
}

// MonitorAddon will track the deployment of Addon
func (h *Handler) annotateGrafanaPVName(pvname string, aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	if aObj == nil || aObj.DeletionTimestamp != nil {
		return nil, nil
	}

	if aObj.Annotations != nil {
		if p, ok := aObj.Annotations[util.AnnotationGrafanaPVName]; ok {
			if p == pvname {
				return aObj, nil
			}
		}
	}

	a := aObj.DeepCopy()
	if a.Annotations == nil {
		a.Annotations = make(map[string]string, 1)
	}

	a.Annotations[util.AnnotationGrafanaPVName] = pvname

	return h.addon.Update(a)
}

func (h *Handler) patchGrafanaPVClaimRef(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	if aObj == nil || aObj.DeletionTimestamp != nil {
		return nil, nil
	}

	// get pvname from addon annotation, format: pvc-a95cdf6b-26bc-4ed6-8aaf-5f5d6b0b6dbd
	// the pvc has been deleted now; unless we iterate all pv to try to find grafana pv
	pvname := ""
	if aObj.Annotations != nil {
		if p, ok := aObj.Annotations[util.AnnotationGrafanaPVName]; ok {
			pvname = p
		}
	}

	// nothing to do
	if pvname == "" {
		logrus.Infof("there is no pvname found on addon annotations, skip patch ClaimRef")
		return aObj, nil
	}

	// pv name format: volumeName: pvc-a95cdf6b-26bc-4ed6-8aaf-5f5d6b0b6dbd
	pv, err := h.pv.Cache().Get(pvname)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("grafana PV %s is not found, skip patch ClaimRef", pvname)
			return aObj, nil
		}

		return aObj, fmt.Errorf("error fetching grafana pv %s: %v", pvname, err)
	}

	// check ReclaimPolicy first
	if pv.Spec.PersistentVolumeReclaimPolicy != corev1.PersistentVolumeReclaimRetain {
		logrus.Infof("grafana PV %s ClaimPolicy is %s, skip patch ClaimRef", pvname, pv.Spec.PersistentVolumeReclaimPolicy)
		return aObj, nil
	}

	// already updated, no op
	if pv.Spec.ClaimRef == nil || pv.Spec.ClaimRef.UID == "" {
		return aObj, nil
	}

	if pv.Status.Phase == corev1.VolumeAvailable {
		logrus.Infof("grafana PV %s status is %s, no need to patch, skip patch ClaimRef", pvname, pv.Status.Phase)
		return aObj, nil
	}

	// only patch when this status is Released
	if pv.Status.Phase != corev1.VolumeReleased {
		logrus.Infof("grafana PV %s status is %s, CAN NOT patch ClaimRef, skip patch ClaimRef", pvname, pv.Status.Phase)
		return aObj, nil
	}

	// patch pv to remove ClaimRef now
	pvCopy := pv.DeepCopy()
	pvCopy.Spec.ClaimRef.UID = ""
	_, err = h.pv.Update(pvCopy)
	if err != nil {
		return aObj, fmt.Errorf("grafana PV %s is failed to patch: %v", pvname, err)
	}

	logrus.Infof("grafana PV %s is patched with null ClaimRef", pvname)
	return aObj, nil
}
