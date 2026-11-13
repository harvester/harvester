package setting

import (
	"fmt"
	"strconv"

	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

// syncPersistentReservation sets Spec.Configuration.PersistentReservationConfiguration of the KubeVirt CR.
// When enabled, virt-operator adds the pr-helper container to virt-handler, and VMs can use LUN disks with
// SCSI-3 persistent reservation. The value is always set explicitly, also when disabled, so the Harvester
// default doesn't change if KubeVirt changes its own default. The chart doesn't render the field, so Helm
// upgrades don't reset it.
func (h *Handler) syncPersistentReservation(setting *harvesterv1.Setting) error {
	value := setting.Value
	if value == "" {
		value = setting.Default
	}

	enabled := false
	if value != "" {
		var err error
		if enabled, err = strconv.ParseBool(value); err != nil {
			return fmt.Errorf("invalid value %q for setting %s: %w", value, setting.Name, err)
		}
	}

	kubevirt, err := h.kubeVirtConfigCache.Get(util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	if err != nil {
		return fmt.Errorf("failed to get kubevirt object %v/%v: %w", util.HarvesterSystemNamespaceName, util.KubeVirtObjectName, err)
	}

	current := kubevirt.Spec.Configuration.PersistentReservationConfiguration
	if current != nil && current.Enabled != nil && *current.Enabled == enabled {
		return nil
	}

	kubevirtCpy := kubevirt.DeepCopy()
	kubevirtCpy.Spec.Configuration.PersistentReservationConfiguration = &kubevirtv1.PersistentReservationConfiguration{Enabled: &enabled}
	if _, err := h.kubeVirtConfig.Update(kubevirtCpy); err != nil {
		return fmt.Errorf("failed to update KubeVirt persistent reservation configuration: %w", err)
	}
	return nil
}
