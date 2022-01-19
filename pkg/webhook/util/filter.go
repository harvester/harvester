package util

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	ctlbackup "github.com/harvester/harvester/pkg/controller/master/backup"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/webhook/indexeres"
)

func HasInProgressingVMBackupBySourceUID(cache ctlharvesterv1.VirtualMachineBackupCache, sourceUID string) (bool, error) {
	vmBackups, err := cache.GetByIndex(indexeres.VMBackupBySourceUIDIndex, sourceUID)
	if err != nil && !apierrors.IsNotFound(err) {
		return false, err
	}
	for _, vmBackup := range vmBackups {
		if ctlbackup.IsBackupProgressing(vmBackup) || ctlbackup.GetVMBackupError(vmBackup) != nil {
			return true, nil
		}
	}
	return false, nil
}
