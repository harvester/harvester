package setting

import (
	"fmt"

	"github.com/sirupsen/logrus"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

// upstream CRD field path Spec.Configuration.AdditionalGuestMemoryOverheadRatio
const overheadConfigName = "AdditionalGuestMemoryOverheadRatio"

func (h *Handler) syncAdditionalGuestMemoryOverheadRatio(setting *harvesterv1.Setting) error {
	var value string
	if setting.Value != "" {
		value = setting.Value
	} else {
		value = setting.Default
	}
	logrus.WithFields(logrus.Fields{
		"name": setting.Name,
	}).Debugf("update %v as %v to kubevirt", overheadConfigName, value)
	agmorc, err := settings.NewAdditionalGuestMemoryOverheadRatioConfig(value)
	if err != nil {
		return err
	}

	// sync with  kubevirt configuration
	kubevirt, err := h.kubeVirtConfigCache.Get(util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	if err != nil {
		return fmt.Errorf("failed to get kubevirt object %v/%v", util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	}
	// kubevirt already has AdditionalGuestMemoryOverheadRatio
	if kubevirt.Spec.Configuration.AdditionalGuestMemoryOverheadRatio != nil {
		// clear current
		if agmorc.IsEmpty() {
			kubevirtCpy := kubevirt.DeepCopy()
			kubevirtCpy.Spec.Configuration.AdditionalGuestMemoryOverheadRatio = nil
			if _, err := h.kubeVirtConfig.Update(kubevirtCpy); err != nil {
				return fmt.Errorf("failed to update %v as nil to kubevirt %w", overheadConfigName, err)
			}
			return nil
		}
		// update with new value
		if *kubevirt.Spec.Configuration.AdditionalGuestMemoryOverheadRatio != value {
			kubevirtCpy := kubevirt.DeepCopy()
			kubevirtCpy.Spec.Configuration.AdditionalGuestMemoryOverheadRatio = &value
			if _, err := h.kubeVirtConfig.Update(kubevirtCpy); err != nil {
				return fmt.Errorf("failed to update %v as %v to kubevirt %w", overheadConfigName, value, err)
			}
			return nil
		}
		// no change
		return nil
	}

	// kubevirt has no AdditionalGuestMemoryOverheadRatio
	// setting value is empty string or zero value, do nothing
	if agmorc.IsEmpty() {
		return nil
	}
	// create new AdditionalGuestMemoryOverheadRatio
	kubevirtCpy := kubevirt.DeepCopy()
	kubevirtCpy.Spec.Configuration.AdditionalGuestMemoryOverheadRatio = &value
	if _, err := h.kubeVirtConfig.Update(kubevirtCpy); err != nil {
		return fmt.Errorf("failed to update %v as %v to kubevirt %w", overheadConfigName, value, err)
	}

	return nil
}
