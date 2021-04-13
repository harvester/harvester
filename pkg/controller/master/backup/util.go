package backup

import (
	"fmt"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/rancher/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func vmBackupReady(vmBackup *harvesterv1.VirtualMachineBackup) bool {
	return vmBackup.Status != nil && vmBackup.Status.ReadyToUse != nil && *vmBackup.Status.ReadyToUse
}

func vmBackupProgressing(vmBackup *harvesterv1.VirtualMachineBackup) bool {
	return vmBackupError(vmBackup) == nil &&
		(vmBackup.Status == nil || vmBackup.Status.ReadyToUse == nil || !*vmBackup.Status.ReadyToUse)
}

func vmBackupError(vmBackup *harvesterv1.VirtualMachineBackup) *harvesterv1.Error {
	if vmBackup.Status != nil && vmBackup.Status.Error != nil {
		return vmBackup.Status.Error
	}
	return nil
}

func vmBackupContentReady(vmBackupContent *harvesterv1.VirtualMachineBackupContent) bool {
	return vmBackupContent.Status != nil && vmBackupContent.Status.ReadyToUse != nil && *vmBackupContent.Status.ReadyToUse
}

func getVMBackupContentName(vmBackup *harvesterv1.VirtualMachineBackup) string {
	if vmBackup.Status != nil && vmBackup.Status.VirtualMachineBackupContentName != nil {
		return *vmBackup.Status.VirtualMachineBackupContentName
	}

	return fmt.Sprintf("%s-%s", vmBackup.Name, "backupcontent")
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

func isContentReady(content *harvesterv1.VirtualMachineBackupContent) bool {
	return content.Status != nil && content.Status.ReadyToUse != nil && *content.Status.ReadyToUse
}

func isContentError(content *harvesterv1.VirtualMachineBackupContent) bool {
	return content.Status != nil && content.Status.Error != nil
}
