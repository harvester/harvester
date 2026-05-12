package backup

import (
	"fmt"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func (h *Handler) updateConditions(vmb *harvesterv1.VirtualMachineBackup) error {
	var vmbCpy = vmb.DeepCopy()
	if h.vmbo.IsProcessing(vmbCpy) {
		vmbCpy = h.vmbo.SetProcessingCondition(vmbCpy)
	}

	ready := true
	var volBackupErr error
	var volumeSizeSum int64
	var progressWeightSum int64

	backupEngine := h.getBackupEngine(vmbCpy)
	for i := range h.vmbo.GetVolBackups(vmbCpy) {
		vb := h.vmbo.GetVolBackup(vmbCpy, i)
		if !h.vmbo.GetVolBackupReadyToUse(vb) {
			ready = false
		}

		volumeSize, err := backupEngine.UpdateProgress(vb)
		if err != nil {
			return err
		}

		volumeSizeSum += volumeSize
		progressWeightSum += int64(h.vmbo.GetVolBackupProgress(vb)) * volumeSizeSum

		if h.vmbo.GetVolBackupError(vb) != nil {
			volBackupErr = fmt.Errorf("VolumeSnapshot %s in error state", *h.vmbo.GetVolBackupName(vb))
			break
		}
	}

	if volumeSizeSum != 0 {
		if err := h.vmbo.SetProgress(vmbCpy, int(progressWeightSum/volumeSizeSum)); err != nil {
			return err
		}
	}

	if ready && !h.vmbo.IsReady(vmbCpy) {
		vmbCpy = h.vmbo.SetCompleteCondition(vmbCpy)
	}

	if volBackupErr != nil && !h.vmbo.IsErrMsgSynced(vmbCpy, volBackupErr.Error()) {
		vmbCpy = h.vmbo.SetErrorCondition(vmbCpy, volBackupErr)
	}

	if err := h.vmbo.SetReadyToUse(vmbCpy, ready); err != nil {
		return err
	}
	_, err := h.vmbo.Update(vmb, vmbCpy)
	return err
}

func (h *Handler) updateVolumeSnapshotChanged(_ string, vs *snapshotv1.VolumeSnapshot) (*snapshotv1.VolumeSnapshot, error) {
	if vs == nil || vs.DeletionTimestamp != nil {
		return nil, nil
	}

	controllerRef := metav1.GetControllerOf(vs)

	// If it has a ControllerRef, that's all that matters.
	if controllerRef != nil {
		ref := h.resolveVolSnapshotRef(vs.Namespace, controllerRef)
		if ref == nil {
			return nil, nil
		}
		h.vmbController.Enqueue(ref.Namespace, ref.Name)
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
	vmb, err := h.vmbCache.Get(namespace, controllerRef.Name)
	if err != nil {
		return nil
	}
	if h.vmbo.GetUID(vmb) != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return vmb
}
