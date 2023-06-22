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

	addonController.OnChange(ctx, "monitor-addon-per-status", h.MonitorAddonPerStatus)
	addonController.OnChange(ctx, "monitor-addon-rancher-monitoring", h.MonitorAddonRancherMonitoring)
	appController.OnChange(ctx, "monitor-apps-catalog", h.PatchApps)
	relatedresource.Watch(ctx, "watch-helmcharts", h.ReconcileHelmChartOwners, addonController, helmController)
	return nil
}

// MonitorAddonPerStatus will track the operation on Addon: update/enable/disable
func (h *Handler) MonitorAddonPerStatus(key string, aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	if aObj == nil || aObj.DeletionTimestamp != nil {
		return nil, nil
	}

	switch aObj.Status.Status {
	// finish the onging enable
	case harvesterv1.AddonEnabled:
		return h.processAddonEnabling(aObj)

	// finish the onging disable
	case harvesterv1.AddonDisabling:
		return h.processAddonDisabling(aObj)

	// finish the onging update
	case harvesterv1.AddonUpdating:
		return h.processAddonUpdating(aObj)

	// no ongoing operation
	case harvesterv1.AddonDeployed, harvesterv1.AddonFailed, harvesterv1.AddonDisableFailed, harvesterv1.AddonUpdateFaild, harvesterv1.AddonInitState:
		return h.processAddonOnChange(aObj)

	default:
		logrus.Errorf("has no processing for status %v, check", aObj.Status.Status)
	}

	return aObj, nil
}

// processAddonEnabling finish the enabling operation
func (h *Handler) processAddonEnabling(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	// TODO: should not happen
	if !aObj.Spec.Enabled {
		logrus.Warnf("addon %s is disabled, but the operation is enable, check", aObj.Name)
		// continue to finish first
		//return aObj, nil
	}

	hc, owned, err := h.getAddonHelmChart(aObj)
	if err != nil {
		return aObj, err
	}

	if hc == nil || !owned {
		// chart is not existing, or not owned by this addon, deploy now
		// but same name and same namespace, will be a problem
		logrus.Infof("create new helm chart %s/%s to deploy addon", aObj.Namespace, aObj.Name)
		err := h.deployHelmChart(aObj)
		if err != nil {
			return aObj, err
		}

		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	if hc.Status.JobName == "" {
		logrus.Debugf("waiting for create jobname to be populated in helm chart status")
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	// helm is not managing the job well, try debug something
	if strings.HasPrefix(hc.Status.JobName, "helm-delete-") {
		logrus.Warnf("the fetched job %s is from last time helm delete, helm may be wrong", hc.Status.JobName)
	}

	job, err := h.job.Get(aObj.Namespace, hc.Status.JobName, metav1.GetOptions{})
	if err != nil {
		logrus.Debugf("waiting for job %s to start", hc.Status.JobName)
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	if !isJobComplete(job) {
		logrus.Infof("waiting for job %s to complete", job.Name)
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	logrus.Infof("addon job %s has finished", job.Name)

	// TODO: add timeout

	// check Failed before since in jobs with more than 1 container then
	// even a single failure should count as a failure
	if job.Status.Failed > 0 {
		return h.updateAddonEnableStatus(aObj, harvesterv1.AddonFailed, corev1.ConditionFalse, "", "failed to enable, job failed")
	}

	return h.updateAddonEnableStatus(aObj, harvesterv1.AddonDeployed, corev1.ConditionTrue, "", "")
}

// processAddonDisabling finish disabling operation
func (h *Handler) processAddonDisabling(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	// TODO:
	if aObj.Spec.Enabled {
		logrus.Errorf("addon %s is enabled, but the operation is disable, check", aObj.Name)
		return aObj, nil
	}

	// try to delete previous job,
	_, wait, err := h.tryRemoveAddonOldJob(aObj)
	if err != nil {
		return aObj, err
	}

	// wait for previous to be removed
	if wait {
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	hc, owned, err := h.getAddonHelmChart(aObj)
	if err != nil {
		return aObj, err
	}

	// skip not owned
	if hc == nil || !owned {
		logrus.Infof("addon %s: helm chart is gone, or owned %v, addon is in %s status, move to init state", aObj.Name, owned, aObj.Status.Status)
		return h.updateAddonDisableStatus(aObj, harvesterv1.AddonInitState, corev1.ConditionTrue, "", "")
	}

	if hc.DeletionTimestamp == nil {
		logrus.Infof("delete the helm chart %s/%s", hc.Namespace, hc.Name)
		if err := h.helm.Delete(hc.Namespace, hc.Name, &metav1.DeleteOptions{}); err != nil {
			return aObj, fmt.Errorf("fail to delete helm chart %v", err)
		}

		// wait helm to work
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	if hc.Status.JobName == "" {
		logrus.Infof("waiting for delete jobname to be populated in helm chart status")
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	// helm is not managing the job well, try debug something
	if strings.HasPrefix(hc.Status.JobName, "helm-install-") {
		logrus.Warnf("the fetched job %s is from last time hell install, helm may be wrong", hc.Status.JobName)
	}

	job, err := h.job.Get(aObj.Namespace, hc.Status.JobName, metav1.GetOptions{})
	if err != nil {
		logrus.Infof("waiting for job %s to start", hc.Status.JobName)
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	if !isJobComplete(job) {
		logrus.Infof("waiting for job %s to complete", job.Name)
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	logrus.Infof("addon job %s/%s has finished", job.Namespace, job.Name)

	// check Failed before since in jobs with more than 1 container then
	// even a single failure should count as a failure
	if job.Status.Failed > 0 {
		// ns = harvesterv1.AddonDisableFailed
		return h.updateAddonDisableStatus(aObj, harvesterv1.AddonDisableFailed, corev1.ConditionFalse, "", "failed to disable, job failed")
	}

	return h.updateAddonDisableStatus(aObj, harvesterv1.AddonInitState, corev1.ConditionTrue, "", "")
}

// processAddonUpdating finish the updating operation
func (h *Handler) processAddonUpdating(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	// even when addon is disabled, valuesContent may still be updated
	if !aObj.Spec.Enabled {
		// move to init directly
		return h.updateAddonUpdateStatus(aObj, harvesterv1.AddonInitState, corev1.ConditionTrue, "", "")
	}

	// addon is enabled
	hc, owned, err := h.getAddonHelmChart(aObj)
	if err != nil {
		return aObj, err
	}

	if hc == nil || !owned {
		// chart is not existing, or not owned by this addon, deploy now
		// but same name and same namespace, will be a problem
		logrus.Infof("create new helm chart %s/%s to update addon", aObj.Namespace, aObj.Name)
		err := h.deployHelmChart(aObj)
		if err != nil {
			return aObj, err
		}
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	// check if hc needs updating, for above newly created one, should not re-upgrade here
	need, vals, err := h.isUpdateNeeded(aObj, hc)
	if err != nil {
		return aObj, err
	}

	if need {
		hcCopy := hc.DeepCopy()
		hcCopy.Spec.Chart = aObj.Spec.Chart
		hcCopy.Spec.Repo = aObj.Spec.Repo
		hcCopy.Spec.Version = aObj.Spec.Version
		hcCopy.Spec.ValuesContent = vals
		logrus.Infof("addon %s has changed, updating helm chart %s", aObj.Name, hc.Name)
		_, err = h.helm.Update(hc)
		if err != nil {
			return aObj, fmt.Errorf("error updating helm chart during monitor changes: %v", err)
		}

		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	if hc.Status.JobName == "" {
		logrus.Debugf("waiting for update jobname to be populated in helm chart status")
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	// helm is not managing the job well, try debug something
	if strings.HasPrefix(hc.Status.JobName, "helm-delete-") {
		logrus.Warnf("the fetched job %s is from last time helm delete, helm may be wrong", hc.Status.JobName)
	}

	job, err := h.job.Get(aObj.Namespace, hc.Status.JobName, metav1.GetOptions{})
	if err != nil {
		logrus.Debugf("waiting for job %s to start", hc.Status.JobName)
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	if !isJobComplete(job) {
		logrus.Infof("waiting for job %s to complete", job.Name)
		h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, 5*time.Second)
		return aObj, nil
	}

	logrus.Infof("addon job %s has finished", job.Name)

	// TODO: add timeout

	// check Failed before since in jobs with more than 1 container then
	// even a single failure should count as a failure
	if job.Status.Failed > 0 {
		return h.updateAddonUpdateStatus(aObj, harvesterv1.AddonUpdateFaild, corev1.ConditionFalse, "", "failed to update, job failed")
	}

	return h.updateAddonUpdateStatus(aObj, harvesterv1.AddonDeployed, corev1.ConditionTrue, "", "")
}

// processAddonOnChange will decide what to do, when an OnChange happens and there is no ongoing operation
func (h *Handler) processAddonOnChange(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	aor := h.getUserOperationFromAnnotations(aObj)

	if aor == nil {
		logrus.Debugf("onchange status: %v, no user operation", aObj.Status.Status)
	} else {
		logrus.Debugf("onchange status: %v, user operation: %v", aObj.Status.Status, *aor)
	}

	// no user operation
	if aor == nil {
		return h.deduceAddonOperation(aObj)
	}

	// e.g., previously enabled failed, , user enable again
	if h.isNewEnableOperation(aObj, aor) {
		logrus.Infof("prepare new enable %v", aObj.Status.Status)
		return h.updateAddonEnableStatus(aObj, harvesterv1.AddonEnabled, corev1.ConditionUnknown, "", "start to enable")
	}

	// e.g., previously disable failed, user disable again
	if h.isNewDisableOperation(aObj, aor) {
		logrus.Infof("prepare new disable %v", aObj.Status.Status)
		return h.updateAddonDisableStatus(aObj, harvesterv1.AddonDisabling, corev1.ConditionUnknown, "", "start to disable")
	}

	// user update the addon, e.g., change the valuesContent
	if h.isNewUpdateOperation(aObj, aor) {
		logrus.Infof("prepare new update %v", aObj.Status.Status)
		return h.updateAddonUpdateStatus(aObj, harvesterv1.AddonUpdating, corev1.ConditionUnknown, "", "start to update")
	}

	// user operation is finished or outdated
	return h.deduceAddonOperation(aObj)
}

// no `running` status , no/out-dated user operation
// but OnChange happens, do what ?
func (h *Handler) deduceAddonOperation(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	logrus.Infof("user operaton is empyt or outdated, deduce next operation, current status is %v", aObj.Status.Status)

	// check if updated needed
	if aObj.Spec.Enabled {
		switch aObj.Status.Status {
		case harvesterv1.AddonFailed, harvesterv1.AddonUpdateFaild:
			// should only be re-enabled/disabled by user operation, otherwise, it will loop
			return aObj, nil

		case harvesterv1.AddonDeployed, harvesterv1.AddonInitState:
			// re-deploy / update when needed
			return h.updateAddonWhenNeeded(aObj)
		}

		logrus.Warnf("Unexpected enabled status %v processing, check", aObj.Status.Status)
		return aObj, nil
	}

	// not enabled
	switch aObj.Status.Status {
	case harvesterv1.AddonUpdateFaild:
		// should only be re-enabled/disabled by user operation, otherwise, it will loop
		return aObj, nil

	case harvesterv1.AddonInitState:
		// check, if helm-chart exists, delete again
		exists, err := h.checkHelmChartExists(aObj)
		if err != nil {
			return aObj, err
		}

		// trigger delete
		if exists {
			return h.updateAddonDisableStatus(aObj, harvesterv1.AddonDisabling, corev1.ConditionUnknown, "", "start to disable")
		}

		return aObj, nil
	}

	// any other case, do nothing
	logrus.Warnf("Unexpected disabled status %v processing, check", aObj.Status.Status)
	return aObj, nil
}

// updateAddonWhenNeeded will redeploy/update
// only called when aObj.Spec.Enabled
func (h *Handler) updateAddonWhenNeeded(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	hc, owned, err := h.getAddonHelmChart(aObj)
	if err != nil {
		return aObj, err
	}

	if hc == nil || !owned {
		// move to new deploy status
		return h.updateAddonEnableStatus(aObj, harvesterv1.AddonEnabled, corev1.ConditionUnknown, "", "start to enable")
	}

	// e.g. downstream helm chart was manually changed, addon will re-sync
	need, _, err := h.isUpdateNeeded(aObj, hc)
	if err != nil {
		return aObj, err
	}

	if need {
		// move to updating status
		return h.updateAddonUpdateStatus(aObj, harvesterv1.AddonUpdating, corev1.ConditionUnknown, "", "start to update")
	}

	return aObj, nil
}

// update addon status and the related condition
func (h *Handler) updateAddonEnableStatus(aObj *harvesterv1.Addon, addonStatus harvesterv1.AddonState, status corev1.ConditionStatus, reason, message string) (*harvesterv1.Addon, error) {
	a := aObj.DeepCopy()
	a.Status.Status = addonStatus

	if harvesterv1.AddonEnableCondition.GetStatus(a) == "" {
		harvesterv1.AddonEnableCondition.CreateUnknownIfNotExists(a)
	}

	harvesterv1.AddonEnableCondition.SetStatus(a, string(status))
	harvesterv1.AddonEnableCondition.Reason(a, reason)
	harvesterv1.AddonEnableCondition.Message(a, message)

	logrus.Debugf("addon enable %s will be set to new status %s", a.Name, a.Status.Status)

	return h.addon.UpdateStatus(a)
}

func (h *Handler) updateAddonDisableStatus(aObj *harvesterv1.Addon, addonStatus harvesterv1.AddonState, status corev1.ConditionStatus, reason, message string) (*harvesterv1.Addon, error) {
	a := aObj.DeepCopy()
	a.Status.Status = addonStatus

	if harvesterv1.AddonDisableCondition.GetStatus(a) == "" {
		harvesterv1.AddonDisableCondition.CreateUnknownIfNotExists(a)
	}

	harvesterv1.AddonDisableCondition.SetStatus(a, string(status))
	harvesterv1.AddonDisableCondition.Reason(a, reason)
	harvesterv1.AddonDisableCondition.Message(a, message)

	logrus.Debugf("addon disable %s will be set to new status %s", a.Name, a.Status.Status)

	return h.addon.UpdateStatus(a)
}

func (h *Handler) updateAddonUpdateStatus(aObj *harvesterv1.Addon, addonStatus harvesterv1.AddonState, status corev1.ConditionStatus, reason, message string) (*harvesterv1.Addon, error) {
	a := aObj.DeepCopy()
	a.Status.Status = addonStatus

	if harvesterv1.AddonUpdateCondition.GetStatus(a) == "" {
		harvesterv1.AddonUpdateCondition.CreateUnknownIfNotExists(a)
	}

	harvesterv1.AddonUpdateCondition.SetStatus(a, string(status))
	harvesterv1.AddonUpdateCondition.Reason(a, reason)
	harvesterv1.AddonUpdateCondition.Message(a, message)

	logrus.Debugf("addon update %s will be set to new status %s", a.Name, a.Status.Status)

	return h.addon.UpdateStatus(a)
}

// the previous remaing job will affect new operation, remove them first
// bool: should wait or not
func (h *Handler) tryRemoveAddonOldJob(aObj *harvesterv1.Addon) (*harvesterv1.Addon, bool, error) {
	hc, owned, err := h.getAddonHelmChart(aObj)
	if err != nil {
		return aObj, false, err
	}

	// not find, or not owned, enable a new one
	if hc == nil || !owned {
		return aObj, false, nil
	}

	if hc.Status.JobName == "" {
		return aObj, false, nil
	}

	job, err := h.job.Get(aObj.Namespace, hc.Status.JobName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return aObj, false, nil
		}

		return aObj, false, fmt.Errorf("error querying helm chart related job %s/%s %v", aObj.Namespace, hc.Status.JobName, err)
	}

	// already checked before
	tm, _ := time.Parse(time.RFC3339, aObj.Annotations[util.AnnotationAddonLastOperationTimestamp])

	//if job.CreationTimestamp.Time.Before(tm) {
	//if isObjectCreationTimestampBeforeAddonOperation(job.CreationTimestamp.Time, aObj) {

	//if job.CreationTimestamp.Before(&aObj.Status.OperationTimestamp) {
	if job.CreationTimestamp.Time.Before(tm) {
		// wait to be deleted
		if job.DeletionTimestamp != nil {
			logrus.Infof("previous job %s/%s is being deleted, wait", job.Namespace, job.Name)
			return aObj, true, nil
		}
		logrus.Infof("previous job %s/%s to be deleted, wait", job.Namespace, job.Name)
		return aObj, true, h.job.Delete(job.Namespace, job.Name, &metav1.DeleteOptions{})
	}

	return aObj, false, nil
}

func (h *Handler) getUserOperationFromAnnotations(aObj *harvesterv1.Addon) *util.AddonOperationRecord {
	if aObj.Annotations == nil {
		return nil
	}

	op, _ := aObj.Annotations[util.AnnotationAddonLastOperation]
	tm, _ := aObj.Annotations[util.AnnotationAddonLastOperationTimestamp]

	if op == "" || tm == "" {
		return nil
	}

	aor, err := util.NewAddonOperationRecord(op, tm)
	if err != nil {
		logrus.Infof("addon %s has no valid user operation %v, check", aObj.Name, err)
		return nil
	}

	logrus.Debugf("addon %s has valid user operation %v", aObj.Name, *aor)
	return aor
}

// user operation is newer than the last condition timestamp
// TODO: NEED TO compare with `LastTransitionTime`, but that field seems not exposed
func (h *Handler) isNewUpdateOperation(aObj *harvesterv1.Addon, aor *util.AddonOperationRecord) bool {
	if aor.Operation != harvesterv1.AddonUpdateOperation {
		return false
	}
	if harvesterv1.AddonUpdateCondition.GetStatus(aObj) == "" {
		return true
	}
	// TODO: compare converted time, than string
	// harvesterv1.AddonUpdateCondition.LastUpdateTime is string
	logrus.Debugf("update addon time, condtion %s, user operation %s", harvesterv1.AddonUpdateCondition.GetLastUpdated(aObj), aor.OperationTimestampString)
	return harvesterv1.AddonUpdateCondition.GetLastUpdated(aObj) < aor.OperationTimestampString
}

// user operation is newer than the last condition timestamp
func (h *Handler) isNewEnableOperation(aObj *harvesterv1.Addon, aor *util.AddonOperationRecord) bool {
	if aor.Operation != harvesterv1.AddonEnableOperation {
		return false
	}
	if harvesterv1.AddonEnableCondition.GetStatus(aObj) == "" {
		return true
	}
	logrus.Debugf("enable addon time, condtion %s, user operation %s", harvesterv1.AddonEnableCondition.GetLastUpdated(aObj), aor.OperationTimestampString)
	return harvesterv1.AddonEnableCondition.GetLastUpdated(aObj) < aor.OperationTimestampString
}

// user operation is newer than the last condition timestamp
func (h *Handler) isNewDisableOperation(aObj *harvesterv1.Addon, aor *util.AddonOperationRecord) bool {
	if aor.Operation != harvesterv1.AddonDisableOperation {
		return false
	}
	if harvesterv1.AddonDisableCondition.GetStatus(aObj) == "" {
		return true
	}
	logrus.Debugf("disable addon time, condtion %s, user operation %s", harvesterv1.AddonDisableCondition.GetLastUpdated(aObj), aor.OperationTimestampString)
	return harvesterv1.AddonDisableCondition.GetLastUpdated(aObj) < aor.OperationTimestampString
}

// get the current addon related helm chart
// bool: if addonOwned or not
func (h *Handler) getAddonHelmChart(aObj *harvesterv1.Addon) (*helmv1.HelmChart, bool, error) {
	hc, err := h.helm.Cache().Get(aObj.Namespace, aObj.Name)
	if err != nil {
		// chart is gone
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("error querying helm chart %v", err)
	}

	addonOwned := false
	for _, v := range hc.GetOwnerReferences() {
		if v.Kind == aObj.Kind && v.APIVersion == aObj.APIVersion && v.UID == aObj.UID && v.Name == aObj.Name {
			addonOwned = true
			break
		}
	}
	return hc, addonOwned, nil
}

// check if update is needed, when needed, also return values related string
func (h *Handler) isUpdateNeeded(aObj *harvesterv1.Addon, hc *helmv1.HelmChart) (bool, string, error) {
	vals, err := defaultValues(aObj)
	if err != nil {
		return false, "", fmt.Errorf("error generating default values of addon %s/%s: %v", aObj.Namespace, aObj.Name, err)
	}

	return (hc.Spec.ValuesContent != vals || hc.Spec.Version != aObj.Spec.Version || hc.Spec.Chart != aObj.Spec.Chart || hc.Spec.Repo != aObj.Spec.Repo), vals, nil
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
	if !aObj.Spec.Enabled && aObj.Status.Status == harvesterv1.AddonInitState {
		logrus.Debugf("start to patch grafana pv ClaimRef")
		return h.patchGrafanaPVClaimRef(aObj)
	}

	return aObj, nil
}

func (h *Handler) checkHelmChartExists(aObj *harvesterv1.Addon) (bool, error) {
	_, err := h.helm.Cache().Get(aObj.Namespace, aObj.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// deploy a new chart
func (h *Handler) deployHelmChart(aObj *harvesterv1.Addon) error {
	vals, err := defaultValues(aObj)
	if err != nil {
		return err
	}

	hc := &helmv1.HelmChart{
		ObjectMeta: metav1.ObjectMeta{
			Name:      aObj.Name,
			Namespace: aObj.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: aObj.APIVersion,
					Kind:       aObj.Kind,
					Name:       aObj.Name,
					UID:        aObj.UID,
				},
			},
		},
		Spec: helmv1.HelmChartSpec{
			Chart:         aObj.Spec.Chart,
			Repo:          aObj.Spec.Repo,
			ValuesContent: vals,
			Version:       aObj.Spec.Version,
		},
	}
	_, err = h.helm.Create(hc)
	if err != nil {
		return fmt.Errorf("error creating helm chart object %v", err)
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

// return:
// bool: really patched;  string: pv-name;    error
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
		logrus.Debugf("grafana PV %s ReclaimPolicy has already been %s", pv.Name, corev1.PersistentVolumeReclaimRetain)
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
		// if p is not find, the return p is ""
		if p, _ := aObj.Annotations[util.AnnotationGrafanaPVName]; p == pvname {
			return aObj, nil
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
		logrus.Infof("there is no pvname found on addon %s annotations, skip patch ClaimRef", aObj.Name)
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
		logrus.Debugf("grafana PV %s ClaimPolicy is %s, skip patch ClaimRef", pvname, pv.Spec.PersistentVolumeReclaimPolicy)
		return aObj, nil
	}

	// already updated, no op
	if pv.Spec.ClaimRef == nil || pv.Spec.ClaimRef.UID == "" {
		return aObj, nil
	}

	if pv.Status.Phase == corev1.VolumeAvailable {
		logrus.Debugf("grafana PV %s status is %s, no need to patch, skip patch ClaimRef", pvname, pv.Status.Phase)
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

	logrus.Debugf("grafana PV %s is patched with null ClaimRef", pvname)
	return aObj, nil
}
