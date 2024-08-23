package setting

import (
	"fmt"

	"github.com/sirupsen/logrus"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

func (h *Handler) syncAdditionalGuestMemoryOverheadRatio(setting *harvesterv1.Setting) error {
	var value string
	if setting.Value != "" {
		value = setting.Value
	} else {
		value = setting.Default
	}
	logrus.Debugf("update AdditionalGuestMemoryOverheadRatio as %v to kubevirt", value)
	if err := settings.ValidateAdditionalGuestMemoryOverheadRatioHelper(value); err != nil {
		return err
	}

	// Write to kubevirt configuration
	kubevirt, err := h.kubeVirtConfigCache.Get(util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	if err != nil {
		return fmt.Errorf("fail to get kubevirt object %v/%v", util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	}

	// kubevirt already has AdditionalGuestMemoryOverheadRatio
	if kubevirt.Spec.Configuration.AdditionalGuestMemoryOverheadRatio != nil {
		// clear current
		if value == "" {
			kubevirtCpy := kubevirt.DeepCopy()
			kubevirtCpy.Spec.Configuration.AdditionalGuestMemoryOverheadRatio = nil
			if _, err := h.kubeVirtConfig.Update(kubevirtCpy); err != nil {
				return fmt.Errorf("fail to update AdditionalGuestMemoryOverheadRatio as nil to kubevirt %w", err)
			}
			return nil
		}

		// update with new value
		if *kubevirt.Spec.Configuration.AdditionalGuestMemoryOverheadRatio != value {
			kubevirtCpy := kubevirt.DeepCopy()
			kubevirtCpy.Spec.Configuration.AdditionalGuestMemoryOverheadRatio = &value
			if _, err := h.kubeVirtConfig.Update(kubevirtCpy); err != nil {
				return fmt.Errorf("fail to update AdditionalGuestMemoryOverheadRatio as  %v to kubevirt %w", value, err)
			}
			return nil
		}

		// no change
		return nil
	}

	// kubevirt has no configuration yet
	// setting value is empty string, do noting
	if value == "" {
		return nil
	}
	// create new AdditionalGuestMemoryOverheadRatio
	kubevirtCpy := kubevirt.DeepCopy()
	kubevirtCpy.Spec.Configuration.AdditionalGuestMemoryOverheadRatio = &value
	if _, err := h.kubeVirtConfig.Update(kubevirtCpy); err != nil {
		return fmt.Errorf("fail to update AdditionalGuestMemoryOverheadRatio as %v to kubevirt %w", value, err)
	}

	return nil
}
