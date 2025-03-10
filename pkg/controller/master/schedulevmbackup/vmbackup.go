package schedulevmbackup

import (
	"time"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/controller/master/backup"
	"github.com/harvester/harvester/pkg/util"
)

func (h *svmbackupHandler) OnVMBackupChange(_ string, vmBackup *harvesterv1.VirtualMachineBackup) (*harvesterv1.VirtualMachineBackup, error) {
	if vmBackup == nil || vmBackup.DeletionTimestamp != nil {
		return nil, nil
	}

	svmbackup := util.ResolveSVMBackupRef(h.svmbackupCache, vmBackup)
	if svmbackup == nil {
		return nil, nil
	}

	if time.Since(vmBackup.CreationTimestamp.Time) < reconcileInterval {
		h.svmbackupController.Enqueue(svmbackup.Namespace, svmbackup.Name)
	}

	if backup.GetVMBackupError(vmBackup) != nil {
		h.svmbackupController.Enqueue(svmbackup.Namespace, svmbackup.Name)
	}

	if backup.IsBackupReady(vmBackup) {
		h.svmbackupController.Enqueue(svmbackup.Namespace, svmbackup.Name)
	}

	if err := checkLHBackupUnexpectedProcessing(h, svmbackup, vmBackup); err != nil {
		return nil, err
	}

	return nil, nil
}

func (h *svmbackupHandler) OnVMBackupRemove(_ string, vmBackup *harvesterv1.VirtualMachineBackup) (*harvesterv1.VirtualMachineBackup, error) {
	if vmBackup == nil {
		return nil, nil
	}

	svmbackup := util.ResolveSVMBackupRef(h.svmbackupCache, vmBackup)
	if svmbackup == nil {
		return nil, nil
	}

	h.svmbackupController.EnqueueAfter(svmbackup.Namespace, svmbackup.Name, updateInterval)
	return nil, nil
}
