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

	if value == "" {
		value = strconv.Itoa(virtconfig.DefaultMaxHotplugRatio)
	}

	num, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid value `%s`: %s", setting.Value, err.Error())
	}

	kubevirt, err := h.kubeVirtConfigCache.Get(util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	if err != nil {
		return fmt.Errorf("failed to get kubevirt object %v/%v", util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	}

	kubevirtCpy := kubevirt.DeepCopy()
	if kubevirtCpy.Spec.Configuration.LiveUpdateConfiguration == nil {
		kubevirtCpy.Spec.Configuration.LiveUpdateConfiguration = &kubevirtv1.LiveUpdateConfiguration{}
	}
	kubevirtCpy.Spec.Configuration.LiveUpdateConfiguration.MaxHotplugRatio = uint32(num)

	if !reflect.DeepEqual(kubevirt.Spec.Configuration.LiveUpdateConfiguration, kubevirtCpy.Spec.Configuration.LiveUpdateConfiguration) {
		if _, err := h.kubeVirtConfig.Update(kubevirtCpy); err != nil {
			return fmt.Errorf("failed to update %v as %v to kubevirt %w", harvSettings.MaxHotplugRatioSettingName, value, err)
		}
	}
	return nil
}
