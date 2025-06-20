package setting

import (
	"fmt"
	"reflect"
	"strconv"

	kubevirtv1 "kubevirt.io/api/core/v1"
	virtconfig "kubevirt.io/kubevirt/pkg/virt-config"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvSettings "github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

func (h *Handler) syncMaxHotplugRatio(setting *harvesterv1.Setting) error {
	var value string
	if setting.Value != "" {
		value = setting.Value
	} else {
		value = setting.Default
	}

	num := uint64(virtconfig.DefaultMaxHotplugRatio)
	var err error

	if value != "" {
		num, err = strconv.ParseUint(value, 10, 32)
		if err != nil {
			return fmt.Errorf("invalid value/default `%s`: %w", value, err)
		}
		// when num is out of range, fallback to the default value, following uint32(num) is safe
		if num < util.MinHotplugRatioValue || num > util.MaxHotplugRatioValue {
			num = uint64(virtconfig.DefaultMaxHotplugRatio)
		}
	}

	kubevirt, err := h.kubeVirtConfigCache.Get(util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	if err != nil {
		return fmt.Errorf("failed to get kubevirt object %v/%v %w", util.HarvesterSystemNamespaceName, util.KubeVirtObjectName, err)
	}

	kubevirtCpy := kubevirt.DeepCopy()
	if kubevirtCpy.Spec.Configuration.LiveUpdateConfiguration == nil {
		kubevirtCpy.Spec.Configuration.LiveUpdateConfiguration = &kubevirtv1.LiveUpdateConfiguration{}
	}
	// skip gosec G115
	kubevirtCpy.Spec.Configuration.LiveUpdateConfiguration.MaxHotplugRatio = uint32(num) //nolint:gosec

	if !reflect.DeepEqual(kubevirt.Spec.Configuration.LiveUpdateConfiguration, kubevirtCpy.Spec.Configuration.LiveUpdateConfiguration) {
		if _, err := h.kubeVirtConfig.Update(kubevirtCpy); err != nil {
			return fmt.Errorf("failed to update %v as %v to kubevirt %w", harvSettings.MaxHotplugRatioSettingName, num, err)
		}
	}
	return nil
}
