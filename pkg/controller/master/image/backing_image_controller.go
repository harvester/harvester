package image

import (
	"fmt"
	"reflect"

	lhv1beta1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	"github.com/rancher/norman/condition"
	"k8s.io/apimachinery/pkg/api/errors"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctllhv1beta1 "github.com/harvester/harvester/pkg/generated/controllers/longhorn.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/harvester/pkg/util"
)

// backingImageHandler syncs upload progress from backing image to vm image status
type backingImageHandler struct {
	vmImages          ctlharvesterv1beta1.VirtualMachineImageClient
	vmImageCache      ctlharvesterv1beta1.VirtualMachineImageCache
	backingImages     ctllhv1beta1.BackingImageClient
	backingImageCache ctllhv1beta1.BackingImageCache
}

func (h *backingImageHandler) OnChanged(_ string, backingImage *lhv1beta1.BackingImage) (*lhv1beta1.BackingImage, error) {
	if backingImage == nil || backingImage.DeletionTimestamp != nil {
		return nil, nil
	}
	if backingImage.Annotations[util.AnnotationImageID] == "" || len(backingImage.Status.DiskFileStatusMap) != 1 {
		return nil, nil
	}
	namespace, name := ref.Parse(backingImage.Annotations[util.AnnotationImageID])
	vmImage, err := h.vmImageCache.Get(namespace, name)
	if errors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if !harvesterv1beta1.ImageInitialized.IsTrue(vmImage) || !harvesterv1beta1.ImageImported.IsUnknown(vmImage) {
		return nil, nil
	}
	toUpdate := vmImage.DeepCopy()
	for _, status := range backingImage.Status.DiskFileStatusMap {
		if status.State == lhv1beta1.BackingImageStateFailed {
			toUpdate = handleFail(toUpdate, condition.Cond(harvesterv1beta1.ImageImported), fmt.Errorf(status.Message))
			toUpdate.Status.Progress = status.Progress
		} else if status.State == lhv1beta1.BackingImageStateReady || status.State == lhv1beta1.BackingImageStateReadyForTransfer {
			harvesterv1beta1.ImageImported.True(toUpdate)
			harvesterv1beta1.ImageImported.Reason(toUpdate, "Imported")
			harvesterv1beta1.ImageImported.Message(toUpdate, status.Message)
			toUpdate.Status.Progress = status.Progress
			toUpdate.Status.Size = backingImage.Status.Size
		} else if status.Progress != toUpdate.Status.Progress {
			harvesterv1beta1.ImageImported.Unknown(toUpdate)
			harvesterv1beta1.ImageImported.Reason(toUpdate, "Importing")
			harvesterv1beta1.ImageImported.Message(toUpdate, status.Message)
			// backing image file upload progress can be 100 before it is ready
			// Set VM image progress to be 99 for better UX in this case
			if status.Progress == 100 {
				toUpdate.Status.Progress = 99
			} else {
				toUpdate.Status.Progress = status.Progress
			}
		}
	}

	if !reflect.DeepEqual(vmImage, toUpdate) {
		if _, err := h.vmImages.Update(toUpdate); err != nil {
			return nil, err
		}
	}

	return nil, nil
}
