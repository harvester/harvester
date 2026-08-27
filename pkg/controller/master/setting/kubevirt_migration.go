package setting

import (
	"encoding/json"
	"fmt"
	"reflect"

	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

func (h *Handler) syncKubeVirtMigration(setting *harvesterv1.Setting) error {
	var value string
	if setting.Value != "" {
		value = setting.Value
	} else {
		value = setting.Default
	}

	var err error
	migrationConfiguration := &kubevirtv1.MigrationConfiguration{}

	if value != "" {
		err = json.Unmarshal([]byte(value), migrationConfiguration)
		if err != nil {
			return fmt.Errorf("invalid value: `%s`: %w", value, err)
		}
	}

	kubevirt, err := h.kubeVirtConfigCache.Get(util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	if err != nil {
		return fmt.Errorf("failed to get kubevirt object %v/%v %w", util.HarvesterSystemNamespaceName, util.KubeVirtObjectName, err)
	}

	kubevirtCpy := kubevirt.DeepCopy()
	if kubevirtCpy.Spec.Configuration.MigrationConfiguration == nil {
		kubevirtCpy.Spec.Configuration.MigrationConfiguration = &kubevirtv1.MigrationConfiguration{}
	}
	// ignore nodeDrainTaintKey and network field
	// The default nodeDrainTaintKey is "kubevirt.io/drain" and it's used in upgrade script.
	// The network field is handled by "vm-migration-network" setting.
	migrationConfiguration.NodeDrainTaintKey = nil
	migrationConfiguration.Network = kubevirtCpy.Spec.Configuration.MigrationConfiguration.Network

	if !reflect.DeepEqual(kubevirtCpy.Spec.Configuration.MigrationConfiguration, migrationConfiguration) {
		kubevirtCpy.Spec.Configuration.MigrationConfiguration = migrationConfiguration
		if _, err := h.kubeVirtConfig.Update(kubevirtCpy); err != nil {
			return fmt.Errorf("failed to update KubeVirt migration configuration, err: %w", err)
		}
	}
	return nil
}
