package addon

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/condition"

	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	// addon enqueue self interval, defaults to 5s
	enqueueInterval = 5
	// addon operation timeout, defaults to 15 min as cluster fisrt setup takes long time, range: [1 (min) ~ 60 (min)]
	addonTimeout    = 15
	addonTimeoutMin = 1
	addonTimeoutMax = 60
)

type addonOperationRecord struct {
	Operation                harvesterv1.AddonOperation `json:"operation,omitempty"`
	OperationTimestamp       metav1.Time                `json:"operationTimestamp,omitempty"`
	OperationTimestampString string                     `json:"operationTimestampstr,omitempty"`
}

func newAddonOperationRecord(operation string, tm string) (*addonOperationRecord, error) {
	aor := addonOperationRecord{}

	if operation == string(harvesterv1.AddonEnableOperation) {
		aor.Operation = harvesterv1.AddonEnableOperation
	} else if operation == string(harvesterv1.AddonDisableOperation) {
		aor.Operation = harvesterv1.AddonDisableOperation
	} else if operation == string(harvesterv1.AddonUpdateOperation) {
		aor.Operation = harvesterv1.AddonUpdateOperation
	} else {
		return nil, fmt.Errorf("invalid operation %s new addonOperationRecord", operation)
	}

	t, err := time.Parse(time.RFC3339, tm)
	if err != nil {
		return nil, fmt.Errorf("invalid time %s to new addonOperationRecord", tm)
	}

	aor.OperationTimestamp.Time = t
	aor.OperationTimestampString = tm

	return &aor, nil
}

func (h *Handler) updateAddonConditions(aObj *harvesterv1.Addon, cond condition.Cond, status corev1.ConditionStatus, reason, message string, updateLastTransitionTime bool) {
	// wrangler's cond function is not very fit for here, any read/write needs to loop; play condition directly
	for i := range aObj.Status.Conditions {
		if aObj.Status.Conditions[i].Type == cond {
			aObj.Status.Conditions[i].Status = status
			aObj.Status.Conditions[i].Reason = reason
			aObj.Status.Conditions[i].Message = message

			// always update last update time
			aObj.Status.Conditions[i].LastUpdateTime = metav1.Now().UTC().Format(time.RFC3339)

			// update the time to remark the starting of this operation
			if updateLastTransitionTime {
				aObj.Status.Conditions[i].LastTransitionTime = aObj.Status.Conditions[i].LastUpdateTime
				logrus.Debugf("addon %s condition %v is set with new LastTransitionTime %s", aObj.Name, cond, aObj.Status.Conditions[i].LastTransitionTime)
			}
			return
		}
	}

	// not find in previous loop, append new, instead of always: if cond.GetStatus(aObj) == ""  { cond.CreateUnknownIfNotExists(aObj) }
	if len(aObj.Status.Conditions) == 0 {
		aObj.Status.Conditions = make([]harvesterv1.Condition, 1)
	}
	newCond := harvesterv1.Condition{Type: cond, Status: status, Reason: reason, Message: message, LastUpdateTime: metav1.Now().UTC().Format(time.RFC3339)}
	if updateLastTransitionTime {
		newCond.LastTransitionTime = newCond.LastUpdateTime
		logrus.Debugf("addon %s condition %v is set with new LastTransitionTime %s", aObj.Name, cond, newCond.LastTransitionTime)
	}
	aObj.Status.Conditions = append(aObj.Status.Conditions, newCond)
}

func (h *Handler) getAddonConditionLastTransitionTime(aObj *harvesterv1.Addon, cond condition.Cond) (*metav1.Time, error) {
	for i := range aObj.Status.Conditions {
		if aObj.Status.Conditions[i].Type == cond {
			// no time info yet
			if aObj.Status.Conditions[i].LastTransitionTime == "" {
				return nil, nil
			}

			// metav1 itself has no Parse function
			tm, err := time.Parse(time.RFC3339, aObj.Status.Conditions[i].LastTransitionTime)
			if err != nil {
				return nil, fmt.Errorf("failed to parse addon %s condition %v LastTransitionTime %s %v", aObj.Name, cond, aObj.Status.Conditions[i].LastTransitionTime, err)
			}
			return &metav1.Time{Time: tm}, nil
		}
	}
	// not find
	return nil, nil
}

// update addon status and the related condition
func (h *Handler) updateAddonEnableStatus(aObj *harvesterv1.Addon, addonStatus harvesterv1.AddonState, status corev1.ConditionStatus, reason, message string) (*harvesterv1.Addon, error) {
	a := aObj.DeepCopy()
	a.Status.Status = addonStatus
	h.updateAddonConditions(a, harvesterv1.AddonEnableCondition, status, reason, message, addonStatus == harvesterv1.AddonEnabled)
	// clear failed condition
	if addonStatus == harvesterv1.AddonEnabled {
		h.updateAddonConditions(a, harvesterv1.AddonFailedCondition, status, reason, message, true)
	}
	// log error message
	if message != "" {
		h.updateAddonConditions(a, harvesterv1.AddonFailedCondition, status, reason, message, false)
	}
	logrus.Debugf("addon %s enable operation will be set to new status %s", a.Name, addonStatus)
	return h.addon.UpdateStatus(a)
}

func (h *Handler) updateAddonDisableStatus(aObj *harvesterv1.Addon, addonStatus harvesterv1.AddonState, status corev1.ConditionStatus, reason, message string) (*harvesterv1.Addon, error) {
	a := aObj.DeepCopy()
	a.Status.Status = addonStatus
	h.updateAddonConditions(a, harvesterv1.AddonDisableCondition, status, reason, message, addonStatus == harvesterv1.AddonDisabling)
	// clear failed condition
	if addonStatus == harvesterv1.AddonDisabling {
		h.updateAddonConditions(a, harvesterv1.AddonFailedCondition, status, reason, message, true)
	}
	// log error message
	if message != "" {
		h.updateAddonConditions(a, harvesterv1.AddonFailedCondition, status, reason, message, false)
	}
	logrus.Debugf("addon %s disable operation will be set to new status %s", a.Name, addonStatus)
	return h.addon.UpdateStatus(a)
}

func (h *Handler) updateAddonUpdateStatus(aObj *harvesterv1.Addon, addonStatus harvesterv1.AddonState, status corev1.ConditionStatus, reason, message string) (*harvesterv1.Addon, error) {
	a := aObj.DeepCopy()
	a.Status.Status = addonStatus
	h.updateAddonConditions(a, harvesterv1.AddonUpdateCondition, status, reason, message, addonStatus == harvesterv1.AddonUpdating)
	// clear failed condition
	if addonStatus == harvesterv1.AddonUpdating {
		h.updateAddonConditions(a, harvesterv1.AddonFailedCondition, status, reason, message, true)
	}
	// log error message
	if message != "" {
		h.updateAddonConditions(a, harvesterv1.AddonFailedCondition, status, reason, message, false)
	}
	logrus.Debugf("addon %s update operation will be set to new status %s", a.Name, addonStatus)
	return h.addon.UpdateStatus(a)
}

// directly update the status to final, no state transfer
func (h *Handler) updateAddonUpdateStatusDirectlyToFinal(aObj *harvesterv1.Addon, addonStatus harvesterv1.AddonState, status corev1.ConditionStatus, reason, message string) (*harvesterv1.Addon, error) {
	a := aObj.DeepCopy()
	a.Status.Status = addonStatus
	// in such case, set the LastTransitionTime as well
	h.updateAddonConditions(a, harvesterv1.AddonUpdateCondition, status, reason, message, true)
	// log error message, normally, here it has no error message
	if message != "" {
		h.updateAddonConditions(a, harvesterv1.AddonFailedCondition, status, reason, message, true)
	}
	logrus.Debugf("addon %s update operation will be set to final status directly %s", a.Name, a.Status.Status)
	return h.addon.UpdateStatus(a)
}

func (h *Handler) setAddonConditionLastTransitionTime(aObj *harvesterv1.Addon, cond condition.Cond) {
	for i := range aObj.Status.Conditions {
		if aObj.Status.Conditions[i].Type == cond {
			aObj.Status.Conditions[i].LastUpdateTime = metav1.Now().UTC().Format(time.RFC3339)
			return
		}
	}

	// not find
	if len(aObj.Status.Conditions) == 0 {
		aObj.Status.Conditions = make([]harvesterv1.Condition, 1)
	}
	aObj.Status.Conditions = append(aObj.Status.Conditions, harvesterv1.Condition{Type: cond, LastUpdateTime: metav1.Now().UTC().Format(time.RFC3339)})
}

// only update LastTransitionTime, if it is forgot to set
func (h *Handler) updateAddonConditionLastTransitionTime(aObj *harvesterv1.Addon, cond condition.Cond) (*harvesterv1.Addon, error) {
	a := aObj.DeepCopy()
	h.setAddonConditionLastTransitionTime(a, cond)
	return h.addon.UpdateStatus(a)
}

func (h *Handler) getUserOperationFromAnnotations(aObj *harvesterv1.Addon) *addonOperationRecord {
	if aObj.Annotations == nil {
		return nil
	}

	op, _ := aObj.Annotations[util.AnnotationAddonLastOperation]
	tm, _ := aObj.Annotations[util.AnnotationAddonLastOperationTimestamp]
	if op == "" || tm == "" {
		return nil
	}
	aor, err := newAddonOperationRecord(op, tm)
	if err != nil {
		// if not valid, skip it
		logrus.Debugf("addon %s has no valid user operation %v, check", aObj.Name, err)
		return nil
	}
	logrus.Debugf("addon %s has valid user operation %v", aObj.Name, *aor)
	return aor
}

// user operation is newer than the last condition timestamp
func (h *Handler) isNewOperation(aObj *harvesterv1.Addon, aor *addonOperationRecord, cond condition.Cond) bool {
	tm, err := h.getAddonConditionLastTransitionTime(aObj, cond)
	if err != nil {
		// info for debug
		logrus.Debugf("failed to convert addon %s LastTransitionTime per condtion %v, %v, check", aObj.Name, cond, err)
	}
	if tm == nil {
		return true
	}
	logrus.Debugf("compare addon %s LastTransitionTime, condtion %v time %s, user operation time %s", aObj.Name, cond, tm.Format(time.RFC3339), aor.OperationTimestampString)
	return tm.Before(&aor.OperationTimestamp)
}

// user update operation is newer than the LastTransitionTime of condition
func (h *Handler) isNewUpdateOperation(aObj *harvesterv1.Addon, aor *addonOperationRecord) bool {
	if aor.Operation != harvesterv1.AddonUpdateOperation {
		return false
	}
	return h.isNewOperation(aObj, aor, harvesterv1.AddonUpdateCondition)
}

// user enable operation is newer than the LastTransitionTime of condition
func (h *Handler) isNewEnableOperation(aObj *harvesterv1.Addon, aor *addonOperationRecord) bool {
	if aor.Operation != harvesterv1.AddonEnableOperation {
		return false
	}
	return h.isNewOperation(aObj, aor, harvesterv1.AddonEnableCondition)
}

// user disable operation is newer than the LastTransitionTime of condition
func (h *Handler) isNewDisableOperation(aObj *harvesterv1.Addon, aor *addonOperationRecord) bool {
	if aor.Operation != harvesterv1.AddonDisableOperation {
		return false
	}
	return h.isNewOperation(aObj, aor, harvesterv1.AddonDisableCondition)
}

// get the current addon related helmchart
// bool: if addonOwned or not
func (h *Handler) getAddonHelmChart(aObj *harvesterv1.Addon) (*helmv1.HelmChart, bool, error) {
	hc, err := h.helm.Cache().Get(aObj.Namespace, aObj.Name)
	if err != nil {
		// chart is gone
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("error querying helmchart %v", err)
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

// check if update is needed, when needed, also return values related string for further use
func (h *Handler) isHelmchartUpdateNeeded(aObj *harvesterv1.Addon, hc *helmv1.HelmChart) (bool, string, error) {
	vals, err := defaultValues(aObj)
	if err != nil {
		return false, "", fmt.Errorf("error generating default values of addon %s/%s: %v", aObj.Namespace, aObj.Name, err)
	}

	return (hc.Spec.ValuesContent != vals || hc.Spec.Version != aObj.Spec.Version || hc.Spec.Chart != aObj.Spec.Chart || hc.Spec.Repo != aObj.Spec.Repo), vals, nil
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
		return fmt.Errorf("error creating helmchart object %v", err)
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

// check if timeout; user can adjust it via annotations AnnotationAddonOperationTimeout
// return bool: if timeout; bool: if need to update the LastTransitionTime
func (h *Handler) isAddonOperationTimeout(aObj *harvesterv1.Addon, cond condition.Cond) (bool, bool) {
	start, _ := h.getAddonConditionLastTransitionTime(aObj, cond)
	// no timestamp, or parse error, need re-update
	if start == nil {
		return false, true
	}

	timeout := time.Duration(addonTimeout)
	if aObj.Annotations != nil {
		if timeoutAnno, ok := aObj.Annotations[util.AnnotationAddonOperationTimeout]; ok {
			i, err := strconv.ParseInt(timeoutAnno, 10, 32)
			if err != nil {
				logrus.Debugf("addon %s has invalid AnnotationAddonOperationTimeout, fallback to default value %v", aObj.Name, timeout)
			} else if i >= addonTimeoutMin && i <= addonTimeoutMax {
				timeout = time.Duration(i)
			} else {
				logrus.Debugf("addon %s AnnotationAddonOperationTimeout is not in range [%v ~ %v], fallback to default value %v", aObj.Name, addonTimeoutMin, addonTimeoutMax, timeout)
			}
		}
	}

	end := start.Time.Add(time.Minute * timeout)
	// metav1 Time has no After(), use times
	return time.Now().After(end), false
}

func (h *Handler) isAddonUpdateNeeded(aObj *harvesterv1.Addon) (bool, error) {
	hc, owned, err := h.getAddonHelmChart(aObj)
	if err != nil {
		return false, err
	}

	if hc == nil || !owned {
		return true, nil
	}

	// e.g. downstream helmchart was manually changed, addon will re-sync
	need, _, err := h.isHelmchartUpdateNeeded(aObj, hc)

	return need, err
}

func (h *Handler) enqueueAfter(aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	h.addon.EnqueueAfter(aObj.Namespace, aObj.Name, enqueueInterval*time.Second)
	return aObj, nil
}
