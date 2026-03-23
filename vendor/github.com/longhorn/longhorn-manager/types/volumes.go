package types

import (
	"fmt"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	VolumeOperationGeneric       = "generic"
	VolumeOperationSizeExpansion = "size-expansion"
)

func IsVolumeReady(v *longhorn.Volume, vrs []*longhorn.Replica, volOp string) (ready bool, msg string) {
	var allReplicaScheduled = true
	if len(vrs) == 0 {
		allReplicaScheduled = false
	}
	for _, r := range vrs {
		if r.Spec.NodeID == "" {
			allReplicaScheduled = false
		}
	}
	scheduledCondition := GetCondition(v.Status.Conditions, longhorn.VolumeConditionTypeScheduled)
	isCloningDesired := IsDataFromVolume(v.Spec.DataSource)
	isCloningCompleted := v.Status.CloneStatus.State == longhorn.VolumeCloneStateCompleted
	if v.Spec.NodeID == "" && v.Status.State != longhorn.VolumeStateDetached {
		return false, fmt.Sprintf("waiting for the volume to fully detach. current state: %v", v.Status.State)
	}
	if v.Status.State == longhorn.VolumeStateDetached && scheduledCondition.Status != longhorn.ConditionStatusTrue && !allReplicaScheduled {
		return false, "volume is currently in detached state with some un-schedulable replicas"
	}
	if v.Status.Robustness == longhorn.VolumeRobustnessFaulted {
		return false, "volume is faulted"
	}
	if isCloningDesired && !isCloningCompleted {
		return false, "volume has not finished cloning data"
	}
	switch volOp {
	case VolumeOperationSizeExpansion:
		// Allow DR volumes (standby) to expand even when RestoreRequired is true.
		// Block non-DR volumes that are still restoring from backup.
		if v.Status.RestoreRequired && !v.Status.IsStandby {
			return false, "cannot expand size: volume is restoring data from backup"
		}
	case VolumeOperationGeneric:
		fallthrough
	default:
		// For all other operations, disallow when restore is required.
		if v.Status.RestoreRequired {
			return false, "volume requires restoration"
		}
	}
	return true, ""
}
