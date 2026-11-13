package common

import (
	"fmt"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
)

const (
	CompleteAnnotation = "backup.harvesterhci.io/remote-cleanup-complete"
	SkipAnnotation     = "backup.harvesterhci.io/skip-remote-cleanup"
)

// GetCleanupTarget returns the current target, or nil when the backup has no
// recorded target or the current target is disconnected.
func GetCleanupTarget(
	vmbr VMBackupReader,
	vmb *harvesterv1.VirtualMachineBackup,
) (*settings.BackupTarget, error) {
	if vmbr.GetBackupTarget(vmb) == nil {
		return nil, nil
	}
	target, err := settings.DecodeBackupTarget(settings.BackupTargetSet.Get())
	if err != nil {
		return nil, fmt.Errorf("failed to decode backup target: %w", err)
	}
	if target.IsDefaultBackupTarget() {
		return nil, nil
	}
	return target, nil
}

// Ready means remote cleanup has finished or was explicitly skipped while the
// target was disconnected. Neither decision should be repeated on retries.
func Ready(vmbr VMBackupReader, vmb *harvesterv1.VirtualMachineBackup) bool {
	annotations := vmbr.GetAnnotations(vmb)
	return annotations[CompleteAnnotation] == "true" || annotations[SkipAnnotation] == "true"
}

// Commit records the remote deletion decision before local Job results are
// released. Retries resume local cleanup without repeating remote work.
func Commit(
	vmbo VMBackupOperator,
	vmb *harvesterv1.VirtualMachineBackup,
	remoteDeleted bool,
) (*harvesterv1.VirtualMachineBackup, error) {
	if Ready(vmbo, vmb) {
		return vmb, nil
	}
	annotation := SkipAnnotation
	if remoteDeleted {
		annotation = CompleteAnnotation
	}
	updated := vmb.DeepCopy()
	if err := vmbo.SetAnnotation(updated, annotation, "true"); err != nil {
		return nil, err
	}
	return vmbo.Update(vmb, updated)
}
