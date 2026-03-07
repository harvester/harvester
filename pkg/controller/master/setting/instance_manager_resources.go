package setting

import (
	"encoding/json"
	"fmt"
	"strings"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhtypes "github.com/longhorn/longhorn-manager/types"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const msgWaitForAttachedVolumes = "waiting for all volumes detached: %s"

func (h *Handler) syncLHIMResources(setting *harvesterv1.Setting) error {
	value := setting.Value
	if value == "" {
		value = setting.Default
	}
	if value == "" {
		return nil
	}

	resources, err := settings.DecodeLHIMResources(value)
	if err != nil {
		return fmt.Errorf("invalid JSON %q for %s: %w", value, setting.Name, err)
	}
	v1CPU := resources.CPU.V1
	v2CPU := resources.CPU.V2

	if err := lhtypes.ValidateCPUReservationValues(lhtypes.SettingNameGuaranteedInstanceManagerCPU, v1CPU); err != nil {
		return fmt.Errorf("invalid CPU value for cpu.v1 %q in %s: %w", v1CPU, setting.Name, err)
	}
	if err := lhtypes.ValidateCPUReservationValues(lhtypes.SettingNameGuaranteedInstanceManagerCPU, v2CPU); err != nil {
		return fmt.Errorf("invalid CPU value for cpu.v2 %q in %s: %w", v2CPU, setting.Name, err)
	}

	if err := h.checkLonghornVolumeDetached(); err != nil {
		return err
	}

	lhSetting, err := h.longhornSettingCache.Get(util.LonghornSystemNamespaceName, string(lhtypes.SettingNameGuaranteedInstanceManagerCPU))
	if err != nil {
		return err
	}

	lhValueBytes, err := json.Marshal(map[string]string{
		"v1": v1CPU,
		"v2": v2CPU,
	})
	if err != nil {
		return err
	}

	lhValue := string(lhValueBytes)
	if lhSetting.Value == lhValue {
		return nil
	}

	lhSettingCpy := lhSetting.DeepCopy()
	lhSettingCpy.Value = lhValue
	_, err = h.longhornSettings.Update(lhSettingCpy)
	return err
}

func (h *Handler) checkLonghornVolumeDetached() error {
	volumes, err := h.longhornVolumeCache.List(util.LonghornSystemNamespaceName, labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list longhorn volumes: %w", err)
	}

	attachedVolumes := make([]string, 0)
	for _, volume := range volumes {
		if volume.Status.State != lhv1beta2.VolumeStateDetached {
			attachedVolumes = append(attachedVolumes, volume.Name)
		}
	}

	if len(attachedVolumes) > 0 {
		return fmt.Errorf(msgWaitForAttachedVolumes, strings.Join(attachedVolumes, ","))
	}

	return nil
}
