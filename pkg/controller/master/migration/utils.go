package migration

import (
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"
)

// when migration is done or aborted, vmi.Status.MigrationState has following conditions
func IsVmiMigrationDone(vmi *kubevirtv1.VirtualMachineInstance) bool {
	if vmi == nil || vmi.Status.MigrationState == nil {
		return false
	}

	return vmi.Status.MigrationState.Completed || vmi.Status.MigrationState.Failed
	//return isVmiMigrationDone(vmi)
}

// when migration is done or aborted, vmi.Status.MigrationState has following conditions
// caller ensures vmi and vmi.Status.MigrationState are not nil
func isVmiMigrationDone(vmi *kubevirtv1.VirtualMachineInstance) bool {
	// migrationState := vmi.Status.MigrationState
	//return migrationState.StartTimestamp != nil && migrationState.EndTimestamp != nil && (migrationState.Completed || migrationState.Failed)
	return vmi.Status.MigrationState.Completed || vmi.Status.MigrationState.Failed
}

// caller ensures vmi and vmi.Status.MigrationState are not nil
func isVmiWithHarvesterMigrationAnnotation(vmi *kubevirtv1.VirtualMachineInstance) bool {
	return vmi.Annotations[util.AnnotationMigrationUID] == string(vmi.Status.MigrationState.MigrationUID)
}

// migration is done or aborted
func IsVmimMigrationDone(vmim *kubevirtv1.VirtualMachineInstanceMigration) bool {
	if vmim == nil {
		return false
	}

	return vmim.Status.Phase == kubevirtv1.MigrationFailed || vmim.Status.Phase == kubevirtv1.MigrationSucceeded
}
