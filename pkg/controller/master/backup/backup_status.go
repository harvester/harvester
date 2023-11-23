package backup

import (
	"fmt"
	"reflect"
	"strings"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	longhorntypes "github.com/longhorn/longhorn-manager/types"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const backupProgressComplete = 100

func (h *Handler) updateBackupProgress(volumeBackup *harvesterv1.VolumeBackup) error {
	if volumeBackup.ReadyToUse != nil && *volumeBackup.ReadyToUse {
		volumeBackup.BackupProgress = backupProgressComplete
		return nil
	}

	if volumeBackup.LonghornBackupName == nil {
		return nil
	}

	lhBackup, err := h.lhbackupCache.Get(util.LonghornSystemNamespaceName, *volumeBackup.LonghornBackupName)
	if err != nil {
		return err
	}

	volumeBackup.BackupProgress = lhBackup.Status.Progress
	return nil
}

func (h *Handler) updateConditions(vmBackup *harvesterv1.VirtualMachineBackup) error {
	var vmBackupCpy = vmBackup.DeepCopy()
	if IsBackupProgressing(vmBackupCpy) {
		updateBackupCondition(vmBackupCpy, newProgressingCondition(corev1.ConditionTrue, "", "Operation in progress"))
		updateBackupCondition(vmBackupCpy, newReadyCondition(corev1.ConditionFalse, "", "Not ready"))
	}

	ready := true
	errorMessage := ""
	var volumeSizeSum int64
	var progressWeightSum int64

	for i := range vmBackupCpy.Status.VolumeBackups {
		vb := &vmBackupCpy.Status.VolumeBackups[i]
		if vb.ReadyToUse == nil || !*vb.ReadyToUse {
			ready = false
		}

		if vmBackupCpy.Spec.Type == harvesterv1.Backup {
			if err := h.updateBackupProgress(vb); err != nil {
				return err
			}

			volumeSizeSum += vb.VolumeSize
			progressWeightSum += int64(vb.BackupProgress) * vb.VolumeSize
		}

		if vb.Error != nil {
			errorMessage = fmt.Sprintf("VolumeSnapshot %s in error state", *vb.Name)
			break
		}
	}

	if volumeSizeSum != 0 {
		vmBackupCpy.Status.Progress = int(progressWeightSum / volumeSizeSum)
	}

	if ready && (vmBackupCpy.Status.ReadyToUse == nil || !*vmBackupCpy.Status.ReadyToUse) {
		vmBackupCpy.Status.CreationTime = currentTime()
		vmBackupCpy.Status.Error = nil
		updateBackupCondition(vmBackupCpy, newProgressingCondition(corev1.ConditionFalse, "", "Operation complete"))
		updateBackupCondition(vmBackupCpy, newReadyCondition(corev1.ConditionTrue, "", "Operation complete"))
	}

	// check if the status need to update the error status
	if errorMessage != "" && (vmBackupCpy.Status.Error == nil || vmBackupCpy.Status.Error.Message == nil || *vmBackupCpy.Status.Error.Message != errorMessage) {
		vmBackupCpy.Status.Error = &harvesterv1.Error{
			Time:    currentTime(),
			Message: pointer.StringPtr(errorMessage),
		}
		updateBackupCondition(vmBackupCpy, newProgressingCondition(corev1.ConditionFalse, "Error", errorMessage))
		updateBackupCondition(vmBackupCpy, newReadyCondition(corev1.ConditionFalse, "", "Not Ready"))
	}

	vmBackupCpy.Status.ReadyToUse = pointer.BoolPtr(ready)

	if !reflect.DeepEqual(vmBackup.Status, vmBackupCpy.Status) {
		if _, err := h.vmBackups.Update(vmBackupCpy); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) updateVolumeSnapshotChanged(key string, snapshot *snapshotv1.VolumeSnapshot) (*snapshotv1.VolumeSnapshot, error) {
	if snapshot == nil || snapshot.DeletionTimestamp != nil {
		return nil, nil
	}

	controllerRef := metav1.GetControllerOf(snapshot)

	// If it has a ControllerRef, that's all that matters.
	if controllerRef != nil {
		ref := h.resolveVolSnapshotRef(snapshot.Namespace, controllerRef)
		if ref == nil {
			return nil, nil
		}
		h.vmBackupController.Enqueue(ref.Namespace, ref.Name)
	}
	return nil, nil
}

// resolveVolSnapshotRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (h *Handler) resolveVolSnapshotRef(namespace string, controllerRef *metav1.OwnerReference) *harvesterv1.VirtualMachineBackup {
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

// mountLonghornVolumes helps to mount the volumes to host if it is detached
func (h *Handler) mountLonghornVolumes(vm *kubevirtv1.VirtualMachine) error {
	for _, vol := range vm.Spec.Template.Spec.Volumes {
		if vol.PersistentVolumeClaim == nil {
			continue
		}
		name := vol.PersistentVolumeClaim.ClaimName

		pvc, err := h.pvcCache.Get(vm.Namespace, name)
		if err != nil {
			return fmt.Errorf("failed to get pvc %s/%s, error: %s", name, vm.Namespace, err.Error())
		}

		sc, err := h.storageClassCache.Get(*pvc.Spec.StorageClassName)
		if err != nil {
			return err
		}
		if sc.Provisioner != longhorntypes.LonghornDriverName {
			continue
		}

		volume, err := h.volumeCache.Get(util.LonghornSystemNamespaceName, pvc.Spec.VolumeName)
		if err != nil {
			return fmt.Errorf("failed to get volume %s/%s, error: %s", name, vm.Namespace, err.Error())
		}

		volCpy := volume.DeepCopy()
		if volume.Status.State == lhv1beta2.VolumeStateDetached || volume.Status.State == lhv1beta2.VolumeStateDetaching {
			volCpy.Spec.NodeID = volume.Status.OwnerID
		}

		if !reflect.DeepEqual(volCpy, volume) {
			logrus.Infof("mount detached volume %s to the node %s", volCpy.Name, volCpy.Spec.NodeID)
			if _, err = h.volumes.Update(volCpy); err != nil {
				return err
			}
		}
	}
	return nil
}

func getVolumeSnapshotContentName(volumeBackup harvesterv1.VolumeBackup) string {
	return fmt.Sprintf("%s-vsc", *volumeBackup.Name)
}

func (h *Handler) OnLHBackupChanged(key string, lhBackup *lhv1beta2.Backup) (*lhv1beta2.Backup, error) {
	if lhBackup == nil || lhBackup.DeletionTimestamp != nil || lhBackup.Status.SnapshotName == "" {
		return nil, nil
	}

	snapshotContent, err := h.snapshotContentCache.Get(strings.Replace(lhBackup.Status.SnapshotName, "snapshot", "snapcontent", 1))
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

	if controllerRef != nil {
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
			if _, err := h.vmBackups.Update(vmBackupCpy); err != nil {
				return nil, err
			}

			return nil, nil
		}

		//engueue to tigger progress update in updateConditions()
		h.vmBackupController.Enqueue(vmBackup.Namespace, vmBackup.Name)
	}
	return nil, nil
}
