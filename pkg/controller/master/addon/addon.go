package addon

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	ctlhelmv1 "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	"github.com/rancher/wrangler/pkg/relatedresource"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

type Handler struct {
	helm  ctlhelmv1.HelmChartController
	addon ctlharvesterv1.AddonController
	job   ctlcorev1.JobController
}

func Register(ctx context.Context, management *config.Management, opts config.Options) error {
	addonController := management.HarvesterFactory.Harvesterhci().V1beta1().Addon()
	helmController := management.HelmFactory.Helm().V1().HelmChart()
	jobController := management.BatchFactory.Batch().V1().Job()
	h := &Handler{
		helm:  helmController,
		addon: addonController,
		job:   jobController,
	}

	addonController.OnChange(ctx, "deploy-addon", h.OnAddonChange)
	addonController.OnChange(ctx, "monitor-addon", h.MonitorAddon)
	addonController.OnChange(ctx, "monitor-changes", h.MonitorChanges)
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
func (h *Handler) MonitorAddon(key string, aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	if aObj == nil || aObj.DeletionTimestamp != nil {
		return nil, nil
	}

	a := aObj.DeepCopy()

	if a.Status.Status != harvesterv1.AddonEnabled {
		return a, nil
	}

	hc, err := h.helm.Cache().Get(a.Namespace, a.Name)
	if err != nil {
		return a, fmt.Errorf("error querying helmchart in checkHelmChartExists: %v", err)
	}

	if hc.Status.JobName == "" {
		return a, fmt.Errorf("waiting for jobname to be populated in HelmChart status")
	}

	job, err := h.job.Get(a.Namespace, hc.Status.JobName, metav1.GetOptions{})
	if err != nil {
		return a, fmt.Errorf("error fetching job %s: %v", job.Name, err)
	}

	// apply condition //
	if !isJobComplete(job) {
		return a, fmt.Errorf("waiting for job %s to complete", job.Name)
	}

	// check Failed before since in jobs with more than 1 container then
	// even a single failure should count as a failure
	if job.Status.Failed > 0 {
		a.Status.Status = harvesterv1.AddonFailed
	} else if job.Status.Succeeded > 0 {
		a.Status.Status = harvesterv1.AddonDeployed
	}

	return h.addon.UpdateStatus(a)
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
func (h *Handler) checkAndDeleteChart(a *harvesterv1.Addon) error {
	hc, err := h.helm.Cache().Get(a.Namespace, a.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// chart doesn't exist. Nothing to do
			return nil
		}
		return fmt.Errorf("error looking up chart in checkAndDeleteChart: %v", err)
	}

	var addonOwned bool
	for _, v := range hc.GetOwnerReferences() {
		if v.Kind == a.Kind && v.APIVersion == a.APIVersion && v.UID == a.UID && v.Name == a.Name {
			addonOwned = true
			break
		}
	}

	if addonOwned {
		return h.helm.Delete(hc.Namespace, hc.Name, &metav1.DeleteOptions{})
	}

	return nil
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
	a := aObj.DeepCopy()
	err := h.checkAndDeleteChart(a)
	if err != nil {
		return a, fmt.Errorf("error removing helm chart for addon %s: %v", a.Name, err)
	}

	a.Status.Status = ""
	return h.addon.UpdateStatus(a)
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
