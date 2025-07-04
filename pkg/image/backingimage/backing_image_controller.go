package backingimage

import (
	"fmt"

	lhv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

// backingImageHandler syncs upload progress from backing image to vm image status
type backingImageHandler struct {
	vmiCache ctlharvesterv1beta1.VirtualMachineImageCache
	vmio     common.VMIOperator
}

func (h *backingImageHandler) updateBackupTarget(vmi *harvesterv1.VirtualMachineImage) error {
	target, err := settings.DecodeBackupTarget(settings.BackupTargetSet.Get())
	if err != nil {
		return fmt.Errorf("failed to decode backup target: %w", err)
	}

	if target.IsDefaultBackupTarget() {
		return nil
	}

	_, err = h.vmio.UpdateBackupTarget(vmi, &harvesterv1.BackupTarget{
		Endpoint:     target.Endpoint,
		BucketName:   target.BucketName,
		BucketRegion: target.BucketRegion,
	})
	return err
}

func (h *backingImageHandler) OnChanged(_ string, bi *lhv1beta2.BackingImage) (*lhv1beta2.BackingImage, error) {
	if bi == nil || bi.DeletionTimestamp != nil {
		return nil, nil
	}
	if bi.Annotations[util.AnnotationImageID] == "" || len(bi.Status.DiskFileStatusMap) != 1 {
		return nil, nil
	}
	namespace, name := ref.Parse(bi.Annotations[util.AnnotationImageID])
	vmi, err := h.vmiCache.Get(namespace, name)
	if errors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	// There are two states that we care about here:
	// - ImageInitialized
	// - ImageImported
	// If ImageInitialized isn't yet true, it means there's no backing
	// image or storage class, so we've got nothing to work with yet and
	// should return immediately.
	if !h.vmio.IsInitialized(vmi) {
		return nil, nil
	}

	// If the VM image is imported, and we think we know everything about it, i.e. we've
	// now been through a series of progress updates during image download,
	// and those are finally done, so let's not worry about further updates.
	// TODO: Improve image to keep sync with LH backing image #6936
	if h.vmio.IsImported(vmi) {
		return nil, nil
	}

	err = nil
	for _, status := range bi.Status.DiskFileStatusMap {
		if status.State == lhv1beta2.BackingImageStateFailed {
			_, err = h.vmio.FailImported(vmi, fmt.Errorf("%s", status.Message), status.Progress)
			continue
		}

		if status.State == lhv1beta2.BackingImageStateReady {
			biSize := bi.Status.Size
			biVsize := bi.Status.VirtualSize
			_, err = h.vmio.Imported(vmi, status.Message, status.Progress, biSize, biVsize)
			continue
		}

		if status.Progress != vmi.Status.Progress {
			_, err = h.vmio.Importing(vmi, status.Message, status.Progress)
		}
	}

	// Only update backup target for restore-type backing images if no previous error occurred.
	updateBackup := err == nil && bi.Spec.SourceType == lhv1beta2.BackingImageDataSourceTypeRestore
	if updateBackup {
		err = h.updateBackupTarget(vmi)
	}
	return nil, err
}
