package backup

import (
	"fmt"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterapiv1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
)

func vmBackupReady(vmBackup *harvesterapiv1.VirtualMachineBackup) bool {
	return vmBackup.Status != nil && vmBackup.Status.ReadyToUse != nil && *vmBackup.Status.ReadyToUse
}

func vmBackupProgressing(vmBackup *harvesterapiv1.VirtualMachineBackup) bool {
	return vmBackupError(vmBackup) == nil &&
		(vmBackup.Status == nil || vmBackup.Status.ReadyToUse == nil || !*vmBackup.Status.ReadyToUse)
}

func vmBackupError(vmBackup *harvesterapiv1.VirtualMachineBackup) *harvesterapiv1.Error {
	if vmBackup.Status != nil && vmBackup.Status.Error != nil {
		return vmBackup.Status.Error
	}
	return nil
}

func vmBackupContentReady(vmBackupContent *harvesterapiv1.VirtualMachineBackupContent) bool {
	return vmBackupContent.Status != nil && vmBackupContent.Status.ReadyToUse != nil && *vmBackupContent.Status.ReadyToUse
}

func getVMBackupContentName(vmBackup *harvesterapiv1.VirtualMachineBackup) string {
	if vmBackup.Status != nil && vmBackup.Status.VirtualMachineBackupContentName != nil {
		return *vmBackup.Status.VirtualMachineBackupContentName
	}

	return fmt.Sprintf("%s-%s", vmBackup.Name, "backupcontent")
}

func newReadyCondition(status corev1.ConditionStatus, message string) harvesterapiv1.Condition {
	return harvesterapiv1.Condition{
		Type:               harvesterapiv1.BackupConditionReady,
		Status:             status,
		Message:            message,
		LastTransitionTime: currentTime().Format(time.RFC3339),
	}
}

func newProgressingCondition(status corev1.ConditionStatus, message string) harvesterapiv1.Condition {
	return harvesterapiv1.Condition{
		Type:               harvesterapiv1.BackupConditionProgressing,
		Status:             status,
		Message:            message,
		LastTransitionTime: currentTime().Format(time.RFC3339),
	}
}

func updateBackupCondition(ss *harvesterapiv1.VirtualMachineBackup, c harvesterapiv1.Condition) {
	ss.Status.Conditions = updateCondition(ss.Status.Conditions, c, false)
}

func updateCondition(conditions []harvesterapiv1.Condition, c harvesterapiv1.Condition, includeReason bool) []harvesterapiv1.Condition {
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

func translateError(e *snapshotv1.VolumeSnapshotError) *harvesterapiv1.Error {
	if e == nil {
		return nil
	}

	return &harvesterapiv1.Error{
		Message: e.Message,
		Time:    e.Time,
	}
}

// variable so can be overridden in tests
var currentTime = func() *metav1.Time {
	t := metav1.Now()
	return &t
}

func isContentReady(content *harvesterapiv1.VirtualMachineBackupContent) bool {
	return content.Status != nil && content.Status.ReadyToUse != nil && *content.Status.ReadyToUse
}

func isContentError(content *harvesterapiv1.VirtualMachineBackupContent) bool {
	return content.Status != nil && content.Status.Error != nil
}
