package util

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	backupcommon "github.com/harvester/harvester/pkg/backup/common"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	restorecommon "github.com/harvester/harvester/pkg/restore/common"
	"github.com/harvester/harvester/pkg/webhook/indexeres"
)

func HasActiveBackup(
	cache ctlharvesterv1.VirtualMachineBackupCache,
	vmbr backupcommon.VMBackupReader,
	sourceUID string,
) (bool, error) {
	vmbs, err := cache.GetByIndex(indexeres.VMBackupBySourceUIDIndex, sourceUID)
	if err != nil && !apierrors.IsNotFound(err) {
		return false, err
	}
	for _, vmb := range vmbs {
		if vmbr.IsProcessing(vmb) || vmbr.GetError(vmb) != nil {
			return true, nil
		}
	}
	return false, nil
}

func HasActiveRestore(
	cache ctlharvesterv1.VirtualMachineRestoreCache,
	vmrr restorecommon.VMRestoreReader,
	targetNamespace,
	targetName string,
) (bool, error) {
	vmrs, err := cache.GetByIndex(indexeres.VMRestoreByTargetNamespaceAndName, fmt.Sprintf("%s-%s", targetNamespace, targetName))
	if err != nil && !apierrors.IsNotFound(err) {
		return false, err
	}

	for _, vmr := range vmrs {
		if vmr == nil {
			continue
		}
		if vmrr.IsFailed(vmr) {
			continue
		}
		if vmrr.IsProgressing(vmr) {
			return true, nil
		}
	}
	return false, nil
}
