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
	resources, err := settings.DecodeLHIMResources(setting.Value)
	if err != nil {
		return fmt.Errorf("invalid JSON %q for %s: %w", setting.Value, setting.Name, err)
	}
	if resources.IsEmpty() {
		// Ignore the initial default value so Harvester does not overwrite an
		// existing Longhorn setting until the user has configured this setting
		// with a meaningful value at least once.
		if setting.Annotations[util.AnnotationHash] == "" {
			return nil
		}
		return h.syncLHIMResourcesToLonghornDefault()
	}
	v1CPU := resources.CPU.V1
	v2CPU := resources.CPU.V2

	if err := lhtypes.ValidateSetting(string(lhtypes.SettingNameGuaranteedInstanceManagerCPU), v1CPU); err != nil {
		return fmt.Errorf("invalid CPU value for cpu.v1 %q in %s: %w", v1CPU, setting.Name, err)
	}
	if err := lhtypes.ValidateSetting(string(lhtypes.SettingNameGuaranteedInstanceManagerCPU), v2CPU); err != nil {
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
		string(lhv1beta2.DataEngineTypeV1): v1CPU,
		string(lhv1beta2.DataEngineTypeV2): v2CPU,
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

func (h *Handler) syncLHIMResourcesToLonghornDefault() error {
	lhSetting, err := h.longhornSettingCache.Get(util.LonghornSystemNamespaceName, string(lhtypes.SettingNameGuaranteedInstanceManagerCPU))
	if err != nil {
		return err
	}

	defaultValue := defaultLonghornLHIMResourcesValue()
	if lhSetting.Value == defaultValue {
		return nil
	}

	if err := h.checkLonghornVolumeDetached(); err != nil {
		return err
	}

	lhSettingCpy := lhSetting.DeepCopy()
	lhSettingCpy.Value = defaultValue
	_, err = h.longhornSettings.Update(lhSettingCpy)
	return err
}

func defaultLonghornLHIMResourcesValue() string {
	return lhtypes.SettingDefinitionGuaranteedInstanceManagerCPU.Default
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
