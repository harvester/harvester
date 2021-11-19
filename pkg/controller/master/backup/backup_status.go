package backup

import (
	"fmt"
	"reflect"
	"strings"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	kubevirtv1 "kubevirt.io/client-go/api/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

// reconcileBackupStatus handles vmBackup status content on change and create volumeSnapshots if not exist
func (h *Handler) reconcileBackupStatus(vmBackup *harvesterv1.VirtualMachineBackup) error {
	var deletedSnapshots, skippedSnapshots []string

	var backupReady = isBackupReady(vmBackup)
	var backupError = isBackupError(vmBackup)

	// create CSI volume snapshots
	for i, volumeBackup := range vmBackup.Status.VolumeBackups {
		if volumeBackup.Name == nil {
			continue
		}

		var vsName = *volumeBackup.Name

		volumeSnapshot, err := h.getVolumeSnapshot(vmBackup.Namespace, vsName)
		if err != nil {
			return err
		}

		if volumeSnapshot == nil {
			// check if snapshot was deleted
			if backupReady {
				logrus.Warningf("VolumeSnapshot %s no longer exists", vsName)
				h.recorder.Eventf(
					vmBackup,
					corev1.EventTypeWarning,
					volumeSnapshotMissingEvent,
					"VolumeSnapshot %s no longer exists",
					vsName,
				)
				deletedSnapshots = append(deletedSnapshots, vsName)
				continue
			}

			if backupError {
				logrus.Infof("Not creating snapshot %s because content in error state", vsName)
				skippedSnapshots = append(skippedSnapshots, vsName)
				continue
			}

			volumeSnapshot, err = h.createVolumeSnapshot(vmBackup, volumeBackup)
			if err != nil {
				return err
			}
		}

		if volumeSnapshot.Status != nil {
			vmBackup.Status.VolumeBackups[i].ReadyToUse = volumeSnapshot.Status.ReadyToUse
			vmBackup.Status.VolumeBackups[i].CreationTime = volumeSnapshot.Status.CreationTime
			vmBackup.Status.VolumeBackups[i].Error = translateError(volumeSnapshot.Status.Error)
		}

	}

	var ready = true
	var errorMessage = ""
	backupCpy := vmBackup.DeepCopy()
	if len(deletedSnapshots) > 0 {
		ready = false
		errorMessage = fmt.Sprintf("volumeSnapshots (%s) missing", strings.Join(deletedSnapshots, ","))
	} else if len(skippedSnapshots) > 0 {
		ready = false
		errorMessage = fmt.Sprintf("volumeSnapshots (%s) skipped because in error state", strings.Join(skippedSnapshots, ","))
	} else {
		for _, vb := range vmBackup.Status.VolumeBackups {
			if vb.ReadyToUse == nil || !*vb.ReadyToUse {
				ready = false
			}

			if vb.Error != nil {
				errorMessage = "VolumeSnapshot in error state"
				break
			}
		}
	}

	if ready && (backupCpy.Status.ReadyToUse == nil || !*backupCpy.Status.ReadyToUse) {
		backupCpy.Status.CreationTime = currentTime()
		updateBackupCondition(backupCpy, newProgressingCondition(corev1.ConditionFalse, "Operation complete"))
		updateBackupCondition(backupCpy, newReadyCondition(corev1.ConditionTrue, "Operation complete"))
	}

	// check if the status need to update the error status
	if errorMessage != "" && (backupCpy.Status.Error == nil || backupCpy.Status.Error.Message == nil || *backupCpy.Status.Error.Message != errorMessage) {
		backupCpy.Status.Error = &harvesterv1.Error{
			Time:    currentTime(),
			Message: &errorMessage,
		}
	}

	backupCpy.Status.ReadyToUse = &ready

	if !reflect.DeepEqual(vmBackup.Status, backupCpy.Status) {
		if _, err := h.vmBackups.Update(backupCpy); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) getVolumeSnapshot(namespace, name string) (*snapshotv1.VolumeSnapshot, error) {
	snapshot, err := h.snapshotCache.Get(namespace, name)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return snapshot, nil
}

func (h *Handler) createVolumeSnapshot(backup *harvesterv1.VirtualMachineBackup, volumeBackup harvesterv1.VolumeBackup) (*snapshotv1.VolumeSnapshot, error) {
	logrus.Debugf("attempting to create VolumeSnapshot %s", *volumeBackup.Name)

	sc, err := h.snapshotClassCache.Get(settings.VolumeSnapshotClass.Get())
	if err != nil {
		return nil, fmt.Errorf("%s/%s VolumeSnapshot requested but no storage class, err: %s",
			backup.Namespace, volumeBackup.PersistentVolumeClaim.ObjectMeta.Name, err.Error())
	}

	snapshot := &snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      *volumeBackup.Name,
			Namespace: backup.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         harvesterv1.SchemeGroupVersion.String(),
					Kind:               vmBackupKind.Kind,
					Name:               backup.Name,
					UID:                backup.UID,
					Controller:         pointer.BoolPtr(true),
					BlockOwnerDeletion: pointer.BoolPtr(true),
				},
			},
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source: snapshotv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: &volumeBackup.PersistentVolumeClaim.ObjectMeta.Name,
			},
			VolumeSnapshotClassName: pointer.StringPtr(sc.Name),
		},
	}

	volumeSnapshot, err := h.snapshots.Create(snapshot)
	if err != nil {
		return nil, err
	}

	h.recorder.Eventf(
		backup,
		corev1.EventTypeNormal,
		volumeSnapshotCreateEvent,
		"Successfully created VolumeSnapshot %s",
		snapshot.Name,
	)

	return volumeSnapshot, nil
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

// reconcileLonghornVolumes helps to mount the volumes to host if it is detached
func (h *Handler) reconcileLonghornVolumes(vm *kubevirtv1.VirtualMachine) error {
	for _, vol := range vm.Spec.Template.Spec.Volumes {
		if vol.PersistentVolumeClaim == nil {
			continue
		}
		name := vol.PersistentVolumeClaim.ClaimName

		pvc, err := h.pvcCache.Get(vm.Namespace, name)
		if err != nil {
			return fmt.Errorf("failed to get pvc %s/%s, error: %s", name, vm.Namespace, err.Error())
		}

		volume, err := h.volumeCache.Get(util.LonghornSystemNamespaceName, pvc.Spec.VolumeName)
		if err != nil {
			return fmt.Errorf("failed to get volume %s/%s, error: %s", name, vm.Namespace, err.Error())
		}

		volCpy := volume.DeepCopy()
		if volume.Status.State == types.VolumeStateDetached || volume.Status.State == types.VolumeStateDetaching {
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
