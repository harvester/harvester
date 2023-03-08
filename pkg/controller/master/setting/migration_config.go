package setting

import (
	"encoding/json"
	"reflect"

	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

const (
	kubevirtNamespace = "harvester-system"
	kubevirtName      = "kubevirt"
)

func (h *Handler) syncMigrationConfig(setting *harvesterv1.Setting) error {
	if setting.Value == "" {
		return h.clearMigrationConfig()
	}

	var migrationConfig kubevirtv1.MigrationConfiguration
	if err := json.Unmarshal([]byte(setting.Value), &migrationConfig); err != nil {
		return err
	}

	kvconfig, err := h.kubevirtConfigCache.Get(kubevirtNamespace, kubevirtName)
	if err != nil {
		return err
	}
	newObj := kvconfig.DeepCopy()
	newObj.Spec.Configuration.MigrationConfiguration = &migrationConfig

	if reflect.DeepEqual(newObj, kvconfig) {
		return nil
	}

	_, err = h.kubevirtConfigs.Update(newObj)
	return nil
}

func (h *Handler) clearMigrationConfig() error {
	old, err := h.kubevirtConfigCache.Get(kubevirtNamespace, kubevirtName)
	if err != nil {
		return err
	}
	newObj := old.DeepCopy()
	newObj.Spec.Configuration.MigrationConfiguration = nil

	_, err = h.kubevirtConfigs.Update(newObj)
	return err
}
