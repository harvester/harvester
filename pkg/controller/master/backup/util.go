package backup

import (
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func isBackupReady(backup *harvesterv1.VirtualMachineBackup) bool {
	return backup.Status != nil && backup.Status.ReadyToUse != nil && *backup.Status.ReadyToUse
}

func isBackupProgressing(backup *harvesterv1.VirtualMachineBackup) bool {
	return vmBackupError(backup) == nil &&
		(backup.Status == nil || backup.Status.ReadyToUse == nil || !*backup.Status.ReadyToUse)
}

func isBackupError(backup *harvesterv1.VirtualMachineBackup) bool {
	return backup.Status != nil && backup.Status.Error != nil
}

func vmBackupError(vmBackup *harvesterv1.VirtualMachineBackup) *harvesterv1.Error {
	if vmBackup.Status != nil && vmBackup.Status.Error != nil {
		return vmBackup.Status.Error
	}
	return nil
}

func newReadyCondition(status corev1.ConditionStatus, message string) harvesterv1.Condition {
	return harvesterv1.Condition{
		Type:               harvesterv1.BackupConditionReady,
		Status:             status,
		Message:            message,
		LastTransitionTime: currentTime().Format(time.RFC3339),
	}
}

func newProgressingCondition(status corev1.ConditionStatus, message string) harvesterv1.Condition {
	return harvesterv1.Condition{
		Type:               harvesterv1.BackupConditionProgressing,
		Status:             status,
		Message:            message,
		LastTransitionTime: currentTime().Format(time.RFC3339),
	}
}

func updateBackupCondition(ss *harvesterv1.VirtualMachineBackup, c harvesterv1.Condition) {
	ss.Status.Conditions = updateCondition(ss.Status.Conditions, c, false)
}

func updateCondition(conditions []harvesterv1.Condition, c harvesterv1.Condition, includeReason bool) []harvesterv1.Condition {
	found := false
	for i := range conditions {
		if conditions[i].Type == c.Type {
			if conditions[i].Status != c.Status || (includeReason && conditions[i].Reason != c.Reason) {
				conditions[i] = c
			}
			found = true
			break
		}
	}

	if !found {
		conditions = append(conditions, c)
	}

	return conditions
}

func translateError(e *snapshotv1.VolumeSnapshotError) *harvesterv1.Error {
	if e == nil {
		return nil
	}

	return &harvesterv1.Error{
		Message: e.Message,
		Time:    e.Time,
	}
}

// variable so can be overridden in tests
var currentTime = func() *metav1.Time {
	t := metav1.Now()
	return &t
}
