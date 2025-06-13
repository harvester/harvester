package addon

import (
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvesterutil "github.com/harvester/harvester/pkg/util"
)

// MonitorAddonRancherMonitoring will track status change of rancher-monitroing addon
func (h *Handler) MonitorAddonRancherMonitoring(_ string, aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	if aObj == nil || aObj.DeletionTimestamp != nil {
		return nil, nil
	}

	if aObj.Name != harvesterutil.RancherMonitoringName || aObj.Namespace != harvesterutil.CattleMonitoringSystemNamespaceName {
		return aObj, nil
	}

	// deployed successfully
	if aObj.Spec.Enabled && aObj.Status.Status == harvesterv1.AddonDeployed {
		// patch pv ReclaimPolicy
		_, pvname, err := h.patchGrafanaPVReclaimPolicy(harvesterutil.CattleMonitoringSystemNamespaceName, harvesterutil.GrafanaPVCName)

		if err != nil {
			return aObj, err
		}

		// annotate pv name
		if pvname != "" {
			return h.annotateGrafanaPVName(pvname, aObj)
		}
	}

	// disabled successfully, the pv has already from Bound to Released/Available
	if !aObj.Spec.Enabled && aObj.Status.Status == harvesterv1.AddonDisabled {
		logrus.Debugf("start to patch grafana pv ClaimRef")
		return h.patchGrafanaPVClaimRef(aObj)
	}

	return aObj, nil
}

// return:
// bool: really patched;  string: pv-name;    error
func (h *Handler) patchGrafanaPVReclaimPolicy(namespace, name string) (bool, string, error) {
	pvc, err := h.pvc.Cache().Get(namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// grafana may not enable pvc
			logrus.Debugf("grafana PVC %s/%s is not found, skip patch ReclaimPolicy", namespace, name)
			return false, "", nil
		}

		return false, "", fmt.Errorf("error fetching grafana pvc %s/%s: %v", namespace, name, err)
	}

	// pv name format: volumeName: pvc-a95cdf6b-26bc-4ed6-8aaf-5f5d6b0b6dbd
	pv, err := h.pv.Cache().Get(pvc.Spec.VolumeName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Debugf("grafana PV %s is not found, retry", pvc.Spec.VolumeName)
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

	logrus.Debugf("grafana PV %s is patched with new ReclaimPolicy %s", pvc.Spec.VolumeName, corev1.PersistentVolumeReclaimRetain)
	return true, pvc.Spec.VolumeName, nil
}

// MonitorAddon will track the deployment of Addon
func (h *Handler) annotateGrafanaPVName(pvname string, aObj *harvesterv1.Addon) (*harvesterv1.Addon, error) {
	if aObj == nil || aObj.DeletionTimestamp != nil {
		return nil, nil
	}

	if aObj.Annotations != nil {
		// if p is not find, the return p is ""
		if p := aObj.Annotations[harvesterutil.AnnotationGrafanaPVName]; p == pvname {
			return aObj, nil
		}
	}

	a := aObj.DeepCopy()
	if a.Annotations == nil {
		a.Annotations = make(map[string]string, 1)
	}

	a.Annotations[harvesterutil.AnnotationGrafanaPVName] = pvname
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
		if p, ok := aObj.Annotations[harvesterutil.AnnotationGrafanaPVName]; ok {
			pvname = p
		}
	}

	// nothing to do
	if pvname == "" {
		logrus.Debugf("there is no pvname found on addon %s annotations, skip patch ClaimRef", aObj.Name)
		return aObj, nil
	}

	// pv name format: volumeName: pvc-a95cdf6b-26bc-4ed6-8aaf-5f5d6b0b6dbd
	pv, err := h.pv.Cache().Get(pvname)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Debugf("grafana PV %s is not found, skip patch ClaimRef", pvname)
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
		logrus.Debugf("grafana PV %s status is %s, CAN NOT patch ClaimRef, skip patch ClaimRef", pvname, pv.Status.Phase)
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
