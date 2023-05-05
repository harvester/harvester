package rancher

import (
	"reflect"

	rancherv3api "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
)

var lockedFeatures = map[string]bool{
	"harvester":                true,
	"rke2":                     true,
	"multi-cluster-management": true,
}

// RancherFeatureOnChange updates the lockedValue of some required features to prevent users from changing them.
func (h *Handler) RancherFeatureOnChange(key string, feature *rancherv3api.Feature) (*rancherv3api.Feature, error) {
	if feature == nil || feature.DeletionTimestamp != nil {
		return nil, nil
	}

	if lockedValue, ok := lockedFeatures[feature.Name]; ok {
		var featureValue bool
		if feature.Spec.Value == nil {
			featureValue = feature.Status.Default
		} else {
			featureValue = *feature.Spec.Value
		}

		// Don't change lockedValue when it's not equal to current value.
		if featureValue != lockedValue {
			return feature, nil
		}

		featureCopy := feature.DeepCopy()
		featureCopy.Status.LockedValue = &lockedValue
		if !reflect.DeepEqual(feature, featureCopy) {
			return h.RancherFeatures.Update(featureCopy)
		}
	}

	return feature, nil
}
