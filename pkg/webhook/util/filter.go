package util

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
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

func HasInProgressingVMRestoreOnSameTarget(cache ctlharvesterv1.VirtualMachineRestoreCache, targetNamespace, targetName string) (bool, error) {
	vmRestores, err := cache.GetByIndex(indexeres.VMRestoreByTargetNamespaceAndName, fmt.Sprintf("%s-%s", targetNamespace, targetName))
	if err != nil && !apierrors.IsNotFound(err) {
		return false, err
	}

	for _, vmRestore := range vmRestores {
		if vmRestore != nil && vmRestore.Status != nil {
			for _, condition := range vmRestore.Status.Conditions {
				if condition.Type == harvesterv1.BackupConditionProgressing && condition.Status == v1.ConditionTrue {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
