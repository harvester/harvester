package addon

import (
	"fmt"
	"strings"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/condition"

	"github.com/rancher/wrangler/pkg/relatedresource"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

// this file includes some workarounds on HelmChart

const (
	helmChartJobInstallPrefix = "helm-install"
	helmChartJobDeletePrefix  = "helm-delete"
)

// downstream helmchart change will also trigger Addon OnChange
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

// workaround on upstream bug https://github.com/k3s-io/helm-controller/issues/177
// https://github.com/k3s-io/helm-controller/blob/9f2eeec3c48ab1710ea7b5491f49f47925b3107b/pkg/controllers/chart/chart.go#L370
// func job(chart *v1.HelmChart) (*batch.Job, *corev1.Secret, *corev1.ConfigMap)
func (h *Handler) generateHelmChartJobName(aObj *harvesterv1.Addon, cond condition.Cond) string {
	action := helmChartJobInstallPrefix
	if cond == harvesterv1.AddonDisableCondition {
		action = helmChartJobDeletePrefix
	}
	return fmt.Sprintf("%s-%s", action, aObj.Name)
}

// the previous remaining jobs will affect new operation, remove them first
// after an addon is enabled, hte hc.Status.JobName may be `helm-install-*`;  and the previous round `helm-delete-*` is still existing
// thus both of the jobs need to be removed
// return: *harvesterv1.Addon:addon, no update in this function; bool:wait or not; *helmv1.HelmChart:addon queried helmchart; bool:if helmchart is belonging to this addon
func (h *Handler) tryRemoveAddonHelmChartOldJob(aObj *harvesterv1.Addon, cond condition.Cond) (*harvesterv1.Addon, bool, *helmv1.HelmChart, bool, error) {
	hc, owned, err := h.getAddonHelmChart(aObj)
	if err != nil {
		return aObj, false, nil, false, err
	}

	// not find, or not owned
	if hc == nil || !owned {
		return aObj, false, hc, owned, nil
	}

	nameFromHc := hc.Status.JobName
	// could be `helm-install-*` or `helm-delete-*`
	nameGenerated := h.generateHelmChartJobName(aObj, cond)
	names := []string{nameGenerated}
	if nameFromHc != "" && nameFromHc != nameGenerated {
		names = append(names, nameFromHc)
	}

	wait := false
	for i := range names {
		job, err := h.job.Get(aObj.Namespace, names[i], metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return aObj, false, hc, owned, fmt.Errorf("error querying helmchart related job %s/%s %v", aObj.Namespace, names[i], err)
		}

		// wait job to be deleted
		if job.DeletionTimestamp != nil {
			logrus.Infof("previous job %s/%s is being deleted, wait", job.Namespace, job.Name)
			wait = true
			continue
		}

		tm, err := h.getAddonConditionLastTransitionTime(aObj, cond)
		if err != nil {
			// leave info for debug
			logrus.Infof("failed to convert time per condtion %v, %v, check", cond, err)
		}

		if tm != nil && job.CreationTimestamp.Before(tm) {
			logrus.Infof("previous job %s/%s is to be deleted, wait", job.Namespace, job.Name)
			if err := h.job.Delete(job.Namespace, job.Name, &metav1.DeleteOptions{}); err != nil {
				return aObj, false, hc, owned, fmt.Errorf("error deleting helmchart related job %s/%s %v", job.Namespace, job.Name, err)
			}
			wait = true
		}
	}

	return aObj, wait, hc, owned, nil
}
