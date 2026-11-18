package setting

import (
	"fmt"
	"reflect"

	"github.com/sirupsen/logrus"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

const (
	kubeVirtValuesKey                 = "kubevirt"
	kubeVirtAdditionalFeatureGatesKey = "additionalFeatureGates"
)

// syncKubeVirtFeatureGates writes the kubevirt-feature-gates setting to the
// harvester ManagedChart values at kubevirt.additionalFeatureGates. The kubevirt
// subchart merges that list into the rendered KubeVirt CR, so the change is part
// of the bundle's desired state: Fleet does not report the bundle as modified,
// and the value is carried over by the upgrade, which keeps the ManagedChart
// values and only bumps the chart version.
func (h *Handler) syncKubeVirtFeatureGates(setting *harvesterv1.Setting) error {
	// Gates that cannot be enabled any more, because the KubeVirt version of
	// this release made them GA, discontinued, or removed them, are skipped so a
	// value set on an older release never blocks the sync after an upgrade.
	gates, ignored := settings.KubeVirtFeatureGatesToApply(setting.Value)
	for name, reason := range ignored {
		logrus.Warnf("Skip KubeVirt feature gate %s from setting %s: %v", name, setting.Name, reason)
	}

	mChart, err := h.managedChartCache.Get(ManagedChartNamespace, HarvesterManagedChartName)
	if err != nil {
		return err
	}
	// The upgrade pauses the harvester ManagedChart while it rewrites it.
	// Return an error so the setting is retried after the upgrade resumes it.
	if mChart.Spec.Paused {
		return fmt.Errorf("ManagedChart %s/%s is paused, possibly by an upgrade; the KubeVirt feature gates will be applied when it resumes",
			ManagedChartNamespace, HarvesterManagedChartName)
	}

	mChartCopy := mChart.DeepCopy()
	if mChartCopy.Spec.Values == nil || mChartCopy.Spec.Values.Data == nil {
		if len(gates) == 0 {
			return nil
		}
		return fmt.Errorf("ManagedChart %s/%s has no values", ManagedChartNamespace, HarvesterManagedChartName)
	}

	kubeVirtValues := map[string]interface{}{}
	if v, ok := mChartCopy.Spec.Values.Data[kubeVirtValuesKey]; ok && v != nil {
		m, ok := v.(map[string]interface{})
		if !ok {
			return fmt.Errorf("ManagedChart value %s is not a map", kubeVirtValuesKey)
		}
		kubeVirtValues = m
	}

	current, exists := kubeVirtValues[kubeVirtAdditionalFeatureGatesKey]
	if len(gates) == 0 {
		if !exists {
			return nil
		}
		delete(kubeVirtValues, kubeVirtAdditionalFeatureGatesKey)
	} else {
		desired := make([]interface{}, 0, len(gates))
		for _, gate := range gates {
			desired = append(desired, gate)
		}
		if exists && reflect.DeepEqual(current, desired) {
			return nil
		}
		kubeVirtValues[kubeVirtAdditionalFeatureGatesKey] = desired
	}
	mChartCopy.Spec.Values.Data[kubeVirtValuesKey] = kubeVirtValues

	logrus.Infof("Update ManagedChart %s/%s value %s.%s to %v",
		ManagedChartNamespace, HarvesterManagedChartName, kubeVirtValuesKey, kubeVirtAdditionalFeatureGatesKey, gates)
	_, err = h.managedCharts.Update(mChartCopy)
	return err
}
