package migration

import (
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
)

// when migration is done or aborted, vmi.Status.MigrationState has Completed or Failed
func IsVmiResetHarvesterMigrationAnnotationRequired(vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vmi == nil || vmi.Status.MigrationState == nil {
		return false
	}
	return (vmi.Status.MigrationState.Completed || vmi.Status.MigrationState.Failed) &&
		vmi.Annotations[util.AnnotationMigrationUID] == string(vmi.Status.MigrationState.MigrationUID)
}

// migration is succeeded or failed (aborted)
func IsVmimMigrationDone(vmim *kubevirtv1.VirtualMachineInstanceMigration) bool {
	if vmim == nil {
		return false
	}

	return vmim.Status.Phase == kubevirtv1.MigrationFailed || vmim.Status.Phase == kubevirtv1.MigrationSucceeded
}
