package backup

import (
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	backuputil "github.com/harvester/harvester/pkg/util/backup"
)

func (h *TargetHandler) reconcileThirdPartyConfig(setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting.DeletionTimestamp != nil {
		return setting, nil
	}
	target, err := backuputil.CurrentTarget()
	if err != nil {
		return setting, err
	}
	return setting, h.reconcileKopiaMaintenance(target)
}
