package addon

import (
	"context"
	"fmt"
	"strings"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	ctlhelmv1 "github.com/k3s-io/helm-controller/pkg/generated/controllers/helm.cattle.io/v1"

	ctlappsv1 "github.com/rancher/rancher/pkg/generated/controllers/catalog.cattle.io/v1"
	ctlbatchv1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/relatedresource"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	harvesterutil "github.com/harvester/harvester/pkg/util"
)

const (
	enqueueInterval = 5 // enqueue interval, 5s
	// TODO: review timeout
	addonTimeout    = 3 // addon operation timeout, 5 min, range: [1 min ~ 60 min]
	addonTimeoutMin = 1
	addonTimeoutMax = 60
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
	relatedresource.Watch(ctx, "watch-helmcharts", h.ReconcileHelmChartOwners, addonController, helmController)
	return nil
}

// MonitorAddonPerStatus will track the OnChange on addon, route actions per addon status
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
		logrus.Warnf("addon %s has no processing for status %v, check", aObj.Name, aObj.Status.Status)
	}

	return aObj, nil
}

// processAddonOnChange will decide what to do, when an OnChange happens and there is no ongoing operation
func (h *Handler) processAddonOnChange(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	aor := h.getUserOperationFromAnnotations(aObj)
	if aor == nil {
		logrus.Debugf("addon %s OnChange status: %v, no user operation", aObj.Name, aObj.Status.Status)
	} else {
		logrus.Debugf("addon %s OnChange status: %v, user operation: %v", aObj.Name, aObj.Status.Status, *aor)
	}

	// user operation is empty, e.g. newly upgraded no, newly created add in bootstrap stage, or fetch failed from annotations
	if aor == nil {
		return h.deduceAddonOperation(aObj, aor)
	}

	// e.g., previously disabled/enable failed, user enable again
	if h.isNewEnableOperation(aObj, aor) {
		logrus.Infof("OnChange: user enable addon, move from %v to new enable status %v", aObj.Status.Status, harvesterv1.AddonEnabled)
		return h.updateAddonEnableStatus(aObj, harvesterv1.AddonEnabled, corev1.ConditionUnknown, "", "")
	}

	// e.g., previously enabled/disable failed, user disable again
	if h.isNewDisableOperation(aObj, aor) {
		logrus.Infof("OnChange: user disable addon, move from %v to new disable status %v", aObj.Status.Status, harvesterv1.AddonDisabling)
		return h.updateAddonDisableStatus(aObj, harvesterv1.AddonDisabling, corev1.ConditionUnknown, "", "")
	}

	// user update the addon, e.g., change the valuesContent
	if h.isNewUpdateOperation(aObj, aor) {
		logrus.Infof("OnChange: user update addon, current status %v, prepare to preProcessAddonUpdating", aObj.Status.Status)
		return h.preProcessAddonUpdating(aObj)
	}

	// user operation is outdated
	return h.deduceAddonOperation(aObj, aor)
}

// processAddonEnabling finish the ongoing enabling operation
func (h *Handler) processAddonEnabling(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	// status self recovering, generally it should not happen
	if !aObj.Spec.Enabled {
		msg := fmt.Sprintf("aborted, addon %s is enabling, but .Spec.Enabled is changed to false, check", aObj.Name)
		logrus.Warnf(msg)
		return h.updateAddonEnableStatus(aObj, harvesterv1.AddonFailed, corev1.ConditionFalse, "", msg)
	}

	// check timeout
	timeout, needUpdate := h.isAddonOperationTimeout(aObj, harvesterv1.AddonEnableCondition)
	if needUpdate {
		return h.updateAddonConditionLastTransitionTime(aObj, harvesterv1.AddonEnableCondition)
	}

	if timeout {
		return h.updateAddonEnableStatus(aObj, harvesterv1.AddonFailed, corev1.ConditionFalse, "", "failed to enable, timeout")
	}

	// try to delete previous job,
	_, wait, hc, owned, err := h.tryRemoveAddonHelmChartOldJob(aObj, harvesterv1.AddonEnableCondition)
	if err != nil {
		return aObj, err
	}

	// wait for previous to be removed
	if wait {
		return h.enqueueAfter(aObj)
	}

	if hc == nil || !owned {
		// chart is not existing, or not owned by this addon, deploy a new one
		logrus.Infof("create new helmchart %s/%s to deploy addon", aObj.Namespace, aObj.Name)
		err := h.deployHelmChart(aObj)
		if err != nil {
			return aObj, err
		}

		return h.enqueueAfter(aObj)
	}

	// should not happen in general
	if hc.DeletionTimestamp != nil {
		logrus.Infof("helmchart %s/%s is being deleting, wait", hc.Namespace, hc.Name)
		return h.enqueueAfter(aObj)
	}

	// when POD starts, addon data in cache, but helmchart may not be in cache, then it will trigger the enable action
	// but above h.deployHelmChart won't create a new one
	// here check again to return quickly
	tm, err := h.getAddonConditionLastTransitionTime(aObj, harvesterv1.AddonEnableCondition)
	if err == nil && tm != nil {
		if hc.CreationTimestamp.Before(tm) {
			// TODO: how to fetch previous status? assume success now
			logrus.Infof("the helmchart %s exists before enable operation, move to %v directly", hc.Name, harvesterv1.AddonDeployed)
			return h.updateAddonEnableStatus(aObj, harvesterv1.AddonDeployed, corev1.ConditionTrue, "", "")
		}
	}

	return h.waitAddonEnablingFinish(aObj, hc)
}

// waitAddonEnablingFinish wait until finish the ongoing enabing operation
func (h *Handler) waitAddonEnablingFinish(aObj *harvesterv1.Addon, hc *helmv1.HelmChart) (*harvesterv1.Addon, error) {
	if hc.Status.JobName == "" {
		logrus.Debugf("waiting for create jobname to be populated in helmchart status")
		return h.enqueueAfter(aObj)
	}

	// helm is not managing the job well, try debug something
	if strings.HasPrefix(hc.Status.JobName, "helm-delete-") {
		logrus.Infof("the fetched job %s is from last time helm delete, helm may be wrong", hc.Status.JobName)
	}

	job, err := h.job.Get(aObj.Namespace, hc.Status.JobName, metav1.GetOptions{})
	if err != nil {
		logrus.Debugf("waiting for job %s to start", hc.Status.JobName)
		return h.enqueueAfter(aObj)
	}

	if !isJobComplete(job) {
		logrus.Debugf("waiting for job %s to complete", job.Name)
		return h.enqueueAfter(aObj)
	}

	logrus.Infof("enable addon job %s has finished with job.Status.Failed %v", job.Name, job.Status.Failed)
	// check Failed before since in jobs with more than 1 container then
	// even a single failure should count as a failure
	if job.Status.Failed > 0 {
		return h.updateAddonEnableStatus(aObj, harvesterv1.AddonFailed, corev1.ConditionFalse, "", fmt.Sprintf("failed to enable addon, job %s has %v failed", job.Name, job.Status.Failed))
	}

	return h.updateAddonEnableStatus(aObj, harvesterv1.AddonDeployed, corev1.ConditionTrue, "", "")
}

// processAddonDisabling finish the ongoing disabling operation
func (h *Handler) processAddonDisabling(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	// status self recovering, generally it should not happen
	if aObj.Spec.Enabled {
		msg := fmt.Sprintf("aborted, addon %s is disabling, but Spec.Enabled is changed to true, check", aObj.Name)
		logrus.Warnf(msg)
		return h.updateAddonDisableStatus(aObj, harvesterv1.AddonDisableFailed, corev1.ConditionFalse, "", msg)
	}

	// check timeout
	timeout, needUpdate := h.isAddonOperationTimeout(aObj, harvesterv1.AddonDisableCondition)
	if needUpdate {
		return h.updateAddonConditionLastTransitionTime(aObj, harvesterv1.AddonDisableCondition)
	}

	if timeout {
		msg := fmt.Sprintf("failed to disable addon, timeout, detailed reason %s", h.generateAddonDisablingTimeoutMsg(aObj))
		return h.updateAddonDisableStatus(aObj, harvesterv1.AddonDisableFailed, corev1.ConditionFalse, "", msg)
	}

	// try to delete previous job,
	_, wait, hc, owned, err := h.tryRemoveAddonHelmChartOldJob(aObj, harvesterv1.AddonDisableCondition)
	if err != nil {
		return aObj, err
	}

	// wait for previous to be removed
	if wait {
		return h.enqueueAfter(aObj)
	}

	if hc == nil || !owned {
		// only finish when helmchart is gone
		logrus.Infof("addon %s: helmchart is gone %v, or owned %v, addon is in %s state, move to init state", aObj.Name, hc == nil, owned, aObj.Status.Status)
		return h.updateAddonDisableStatus(aObj, harvesterv1.AddonInitState, corev1.ConditionTrue, "", "")
	}

	if hc.DeletionTimestamp == nil {
		logrus.Infof("delete the helmchart %s/%s", hc.Namespace, hc.Name)
		if err := h.helm.Delete(hc.Namespace, hc.Name, &metav1.DeleteOptions{}); err != nil {
			return aObj, fmt.Errorf("failed to delete helmchart %s/%s, %v", hc.Namespace, hc.Name, err)
		}

		return h.enqueueAfter(aObj)
	}

	return h.waitAddonDisablingFinish(aObj, hc)
}

// waitAddonDisablingFinish wait until finish the ongoing disabling operation
// more for debug purpose
func (h *Handler) waitAddonDisablingFinish(aObj *harvesterv1.Addon, hc *helmv1.HelmChart) (*harvesterv1.Addon, error) {
	if hc.Status.JobName == "" {
		logrus.Debugf("waiting for delete jobname to be populated in helmchart status")
		return h.enqueueAfter(aObj)
	}

	if strings.HasPrefix(hc.Status.JobName, helmChartJobInstallPrefix) {
		logrus.Debugf("the fetched job %s is from last time hell install, wait", hc.Status.JobName)
		// helm will update Status.JobName, lets wait
		return h.enqueueAfter(aObj)
	}

	job, err := h.job.Get(aObj.Namespace, hc.Status.JobName, metav1.GetOptions{})
	if err != nil {
		logrus.Debugf("waiting for job %s to start", hc.Status.JobName)
		return h.enqueueAfter(aObj)
	}

	if !isJobComplete(job) {
		logrus.Debugf("waiting for job %s to complete", job.Name)
		return h.enqueueAfter(aObj)
	}

	if job.Status.Failed > 0 {
		logrus.Infof("disable addon job %s/%s has finished, with job.Status.Failed %v", job.Namespace, job.Name, job.Status.Failed)
	}

	// job is finished, does not mean `helmchart` is deleted, need to wait until the `helmchart` is deleted
	// ever encountered: job is finished, but `helmchart` needs additional few seconds to be removed
	// if return `aObj, nil` here, the state machine will shift to `deleting` again
	return h.enqueueAfter(aObj)
}

// generateAddonDisablingTimeoutMsg generatse a message to describe the possible failure
func (h *Handler) generateAddonDisablingTimeoutMsg(aObj *harvesterv1.Addon) string {
	hc, owned, err := h.getAddonHelmChart(aObj)
	if err != nil {
		return fmt.Sprintf("failed to get addon helmchart %v", err)
	}
	// skip not owned
	if hc == nil || !owned {
		return fmt.Sprintf("helmchart is nil %v or owned %v", hc == nil, owned)
	}
	if hc.Status.JobName == "" {
		return "helmchart has no JobName"
	}
	job, err := h.job.Get(aObj.Namespace, hc.Status.JobName, metav1.GetOptions{})
	if err != nil {
		return fmt.Sprintf("failed to get helmchart %s %v", hc.Status.JobName, err)
	}
	if !isJobComplete(job) {
		return fmt.Sprintf("helmchart job %s is not finished", job.Name)
	}
	if job.Status.Failed > 0 {
		// ns = harvesterv1.AddonDisableFailed
		return fmt.Sprintf("helmchart job %s failed %v", job.Name, job.Status.Failed)
	}
	// maybe other reasons
	return "other"
}

// processAddonUpdating finish the ongoing updating operation
func (h *Handler) processAddonUpdating(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	// even when addon is disabled, valuesContent may still be updated
	if !aObj.Spec.Enabled {
		// move to init directly
		return h.updateAddonUpdateStatus(aObj, harvesterv1.AddonInitState, corev1.ConditionTrue, "", "")
	}

	// addon is enabled
	// check timeout
	timeout, needUpdate := h.isAddonOperationTimeout(aObj, harvesterv1.AddonUpdateCondition)
	if needUpdate {
		return h.updateAddonConditionLastTransitionTime(aObj, harvesterv1.AddonUpdateCondition)
	}

	if timeout {
		return h.updateAddonUpdateStatus(aObj, harvesterv1.AddonUpdateFaild, corev1.ConditionFalse, "", "failed to update addon, timeout")
	}

	// need an real update, try to delete previous job
	_, wait, hc, owned, err := h.tryRemoveAddonHelmChartOldJob(aObj, harvesterv1.AddonUpdateCondition)
	if err != nil {
		return aObj, err
	}

	// wait for old job to be removed
	if wait {
		logrus.Infof("addon %s updating wait for old job to be deleted", aObj.Name)
		return h.enqueueAfter(aObj)
	}

	if hc == nil || !owned {
		// chart is not existing, or not owned by this addon, deploy a new one
		logrus.Infof("create new helmchart %s/%s to update addon", aObj.Namespace, aObj.Name)
		err := h.deployHelmChart(aObj)
		if err != nil {
			return aObj, err
		}
		return h.enqueueAfter(aObj)
	}

	// should not happen in general
	if hc.DeletionTimestamp != nil {
		logrus.Infof("helmchart %s/%s is being deleting, wait", hc.Namespace, hc.Name)
		return h.enqueueAfter(aObj)
	}

	// check if hc needs updating, for above newly created one, should not re-upgrade here
	need, vals, err := h.isHelmchartUpdateNeeded(aObj, hc)
	if err != nil {
		return aObj, err
	}

	// update is needed
	if need {
		hcCopy := hc.DeepCopy()
		hcCopy.Spec.Chart = aObj.Spec.Chart
		hcCopy.Spec.Repo = aObj.Spec.Repo
		hcCopy.Spec.Version = aObj.Spec.Version
		hcCopy.Spec.ValuesContent = vals
		logrus.Infof("addon %s has been changed, updating the helmchart %s", aObj.Name, hc.Name)
		_, err = h.helm.Update(hcCopy)
		if err != nil {
			return aObj, fmt.Errorf("error updating helmchart %s when updating: %v", hc.Name, err)
		}

		return h.enqueueAfter(aObj)
	}

	return h.waitAddonUpdatingFinish(aObj, hc)
}

// waitAddonUpdatingFinish wait until finish the ongoing updating operation
func (h *Handler) waitAddonUpdatingFinish(aObj *harvesterv1.Addon, hc *helmv1.HelmChart) (*harvesterv1.Addon, error) {
	if hc.Status.JobName == "" {
		logrus.Debugf("waiting for update jobname to be populated in helmchart status")

		return h.enqueueAfter(aObj)
	}

	// helm is not managing the job well, try debug something
	if strings.HasPrefix(hc.Status.JobName, helmChartJobDeletePrefix) {
		//	logrus.Warnf("the fetched job %s is from last time helm delete, helm may be wrong", hc.Status.JobName)
		return h.enqueueAfter(aObj)
	}

	job, err := h.job.Get(aObj.Namespace, hc.Status.JobName, metav1.GetOptions{})
	if err != nil {
		logrus.Debugf("waiting for job %s to start", hc.Status.JobName)

		return h.enqueueAfter(aObj)
	}

	if !isJobComplete(job) {
		logrus.Debugf("waiting for job %s to complete", job.Name)

		return h.enqueueAfter(aObj)
	}

	logrus.Infof("update addon job %s has finished with job.Status.Failed %v", job.Name, job.Status.Failed)

	// check Failed before since in jobs with more than 1 container then
	// even a single failure should count as a failure
	if job.Status.Failed > 0 {
		return h.updateAddonUpdateStatus(aObj, harvesterv1.AddonUpdateFaild, corev1.ConditionFalse, "", fmt.Sprintf("failed to update addon, job %s has %v failed", job.Name, job.Status.Failed))
	}

	return h.updateAddonUpdateStatus(aObj, harvesterv1.AddonDeployed, corev1.ConditionTrue, "", "")
}

// preProcessAddonUpdating before trigger the actual updating, do a serie of checking
// only called when there is an `update` user operation waiting for process
func (h *Handler) preProcessAddonUpdating(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	// even when addon is disabled, valuesContent may still be updated
	if !aObj.Spec.Enabled {
		// move to init directly
		return h.updateAddonUpdateStatusDirectlyToFinal(aObj, harvesterv1.AddonInitState, corev1.ConditionTrue, "", "")
	}

	// addon is enabled
	need, err := h.isAddonUpdateNeeded(aObj)
	if err != nil {
		return aObj, err
	}
	// no need to trully update, do not change current status
	if !need {
		logrus.Infof("addon %s is not needed to update, keep current status %v", aObj.Name, aObj.Status.Status)
		return h.updateAddonUpdateStatusDirectlyToFinal(aObj, aObj.Status.Status, corev1.ConditionTrue, "", "")
	}
	logrus.Infof("addon %s is needed to update, move from %v to new update status %v", aObj.Name, aObj.Status.Status, harvesterv1.AddonUpdating)

	// now move the the true updating
	return h.updateAddonUpdateStatus(aObj, harvesterv1.AddonUpdating, corev1.ConditionUnknown, "", "")
}

// no `running` status, no user operation, or out-dated user operation
// but OnChange happens, try to deduce one
func (h *Handler) deduceAddonOperation(aObj *harvesterv1.Addon, aor *harvesterutil.AddonOperationRecord) (*harvesterv1.Addon, error) {
	logrus.Debugf("addon %s user operaton is empty %v or outdated, deduce next operation, current status is %v", aObj.Name, aor == nil, aObj.Status.Status)
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

		logrus.Warnf("addon %s is enabled, unexpected status %v processing, check", aObj.Name, aObj.Status.Status)
		return aObj, nil
	}

	// not enabled
	switch aObj.Status.Status {
	case harvesterv1.AddonDisableFailed:
		// should only be re-enabled/disabled by user operation, otherwise, it will loop
		return aObj, nil

	case harvesterv1.AddonInitState:
		// check, if helm-chart exists, disable again
		return h.disableAddonWhenNeeded(aObj)
	}

	// any other case, do nothing
	logrus.Warnf("addon %s is disabled, unexpected status %v processing, check", aObj.Name, aObj.Status.Status)
	return aObj, nil
}

// updateAddonWhenNeeded will redeploy/update the addon when needed
// only called when aObj.Spec.Enabled
func (h *Handler) updateAddonWhenNeeded(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	hc, owned, err := h.getAddonHelmChart(aObj)
	if err != nil {
		return aObj, err
	}
	if hc == nil || !owned {
		// move to new AddonEnabled status
		// AddonEnabled can recover when hc is nil now but fetched later (normally happened when POD just starts)
		return h.updateAddonEnableStatus(aObj, harvesterv1.AddonEnabled, corev1.ConditionUnknown, "", "")
	}
	// wait
	if hc.DeletionTimestamp != nil {
		return h.enqueueAfter(aObj)
	}

	// e.g. downstream helmchart was manually changed, addon will re-sync
	need, _, err := h.isHelmchartUpdateNeeded(aObj, hc)
	if err != nil {
		return aObj, err
	}
	if need {
		// move to updating status
		// updating depends on an real diff to fulfill the task
		return h.updateAddonUpdateStatus(aObj, harvesterv1.AddonUpdating, corev1.ConditionUnknown, "", "")
	}

	return aObj, nil
}

// disableAddonWhenNeeded will disable the addon when needed
// only called when ! aObj.Spec.Enabled
func (h *Handler) disableAddonWhenNeeded(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	hc, owned, err := h.getAddonHelmChart(aObj)
	if err != nil {
		return aObj, err
	}
	if hc == nil || !owned {
		return aObj, nil
	}
	// wait
	if hc.DeletionTimestamp != nil {
		return h.enqueueAfter(aObj)
	}

	// trigger delete
	logrus.Infof("addon %v is disabled, but helmchart is still existing, trigger disable", aObj.Name)
	return h.updateAddonDisableStatus(aObj, harvesterv1.AddonDisabling, corev1.ConditionUnknown, "", "")
}
