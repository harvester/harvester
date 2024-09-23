package schedulevmbackup

import (
	"reflect"
	"strings"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func (h *svmbackupHandler) resolveVolSnapshotRef(namespace string, controllerRef *metav1.OwnerReference) *harvesterv1.VirtualMachineBackup {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != vmBackupKind.Kind {
		return nil
	}
	backup, err := h.vmBackupCache.Get(namespace, controllerRef.Name)
	if err != nil {
		return nil
	}
	if backup.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return backup
}

// TODO: check if this function could be merged with pkg/controller/master/backup/backup_status.go OnLHBackupChanged()
// Since in v1.4.0 we should not introduce too many changes in vmbackp controller
func (h *svmbackupHandler) OnLHBackupChanged(_ string, lhBackup *lhv1beta2.Backup) (*lhv1beta2.Backup, error) {
	if lhBackup == nil || lhBackup.DeletionTimestamp != nil || lhBackup.Spec.SnapshotName == "" {
		return nil, nil
	}

	snapshotContent, err := h.snapshotContentCache.Get(strings.Replace(lhBackup.Spec.SnapshotName, "snapshot", "snapcontent", 1))
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, nil
	}

	snapshot, err := h.snapshotCache.Get(snapshotContent.Spec.VolumeSnapshotRef.Namespace, snapshotContent.Spec.VolumeSnapshotRef.Name)
	if err != nil {
		return nil, err
	}

	controllerRef := metav1.GetControllerOf(snapshot)
	if controllerRef == nil {
		return nil, nil
	}

	vmBackup := h.resolveVolSnapshotRef(snapshot.Namespace, controllerRef)
	if vmBackup == nil || vmBackup.Status == nil || vmBackup.Status.BackupTarget == nil {
		return nil, nil
	}

	vmBackupCpy := vmBackup.DeepCopy()
	for i, volumeBackup := range vmBackupCpy.Status.VolumeBackups {
		if *volumeBackup.Name == snapshot.Name {
			vmBackupCpy.Status.VolumeBackups[i].LonghornBackupName = pointer.StringPtr(lhBackup.Name)
		}
	}

	if !reflect.DeepEqual(vmBackup.Status, vmBackupCpy.Status) {
		if _, err := h.vmBackupClient.Update(vmBackupCpy); err != nil {
			return nil, err
		}

		return nil, nil
	}

	return nil, nil
}
