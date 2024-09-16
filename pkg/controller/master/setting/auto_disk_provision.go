package setting

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	HarvesterManagedChartName = "harvester"
	NDMName                   = "harvester-node-disk-manager"
)

func (h *Handler) syncNDMAutoProvisionPaths(setting *harvesterv1.Setting) error {
	bundle, err := h.bundleCache.Get(util.FleetLocalNamespaceName, util.HarvesterBundleName)
	if err != nil {
		return err
	}
	bundleCopy := bundle.DeepCopy()

	NDMValues, ok := bundleCopy.Spec.Helm.Values.Data[NDMName]
	if !ok {
		return fmt.Errorf("NDM chart value not found in Bundle")
	}

	NDMValuesMap, ok := NDMValues.(map[string]interface{})
	if !ok {
		return fmt.Errorf("NDM chart value is not a map[string]interface{}")
	}

	autoProvFilters := strings.Split(setting.Value, ",")
	for i, filter := range autoProvFilters {
		autoProvFilters[i] = strings.TrimSpace(filter)
	}

	NDMValuesMap["autoProvisionFilter"] = autoProvFilters
	bundleCopy.Spec.Helm.Values.Data[NDMName] = NDMValuesMap

	logrus.Debugf("NDM values to be updated to Bundle: %v", bundleCopy.Spec.Helm.Values.Data[NDMName])
	if _, err := h.bundleClient.Update(bundleCopy); err != nil {
		return err
	}

	return nil
}
