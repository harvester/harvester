package image

import (
	"time"

	"github.com/sirupsen/logrus"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/image/backend"
	"github.com/harvester/harvester/pkg/image/common"
	"github.com/harvester/harvester/pkg/util"
)

const (
	checkInterval = 1 * time.Second
)

// vmImageHandler syncs status on vm image changes, and manage a storageclass & a backingimage per vm image
type vmImageHandler struct {
	vmiClient     ctlharvesterv1.VirtualMachineImageClient
	vmiController ctlharvesterv1.VirtualMachineImageController
	vmio          common.VMIOperator
	backends      map[harvesterv1.VMIBackend]backend.Backend
}

func (h *vmImageHandler) OnChanged(_ string, vmi *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if vmi == nil || vmi.DeletionTimestamp != nil || harvesterv1.ImageRetryLimitExceeded.IsTrue(vmi) {
		return vmi, nil
	}

	if h.vmio.IsImported(vmi) {
		// sync display_name to labels in order to list by labelSelector
		if vmi.Spec.DisplayName != vmi.Labels[util.LabelImageDisplayName] {
			toUpdate := vmi.DeepCopy()
			if toUpdate.Labels == nil {
				toUpdate.Labels = map[string]string{}
			}
			toUpdate.Labels[util.LabelImageDisplayName] = vmi.Spec.DisplayName
			return h.vmiClient.Update(toUpdate)
		}

		// sync virtualSize (handles the case for existing images that were
		// imported before this field was added, because adding the field to
		// the CRD triggers vmImageHandler.OnChanged)
		if h.vmio.GetVirtualSize(vmi) == 0 {
			toUpdate, err := h.backends[util.GetVMIBackend(vmi)].UpdateVirtualSize(vmi)
			if err != nil {
				// If for some reason we're unable to get the backing image,
				// we set the BackingImageMissing condition on the image,
				// including the error message, so it can be seen via
				// `kubectl describe`.  The image is not marked failed or
				// anything, in case this is some sort of transient error.
				// In the unlikely event that we do hit this case, and whatever
				// error was present is later fixed, the user can re-trigger
				// this code by making a temporary change to the image (e.g.
				// add/change the image description).  Once set, the
				// BackingImageMissing condition is never automatically removed,
				// but can be deleted manually with `kubectl edit`.
				logrus.WithError(err).WithFields(logrus.Fields{
					"namespace": toUpdate.Namespace,
					"name":      toUpdate.Name,
				}).Error("failed to get backing image for vmimage")
				return h.vmio.BackingImageMissing(toUpdate, err)
			}
			return toUpdate, nil
		}

		return vmi, nil
	}

	return h.processVMImage(vmi)
}

func (h *vmImageHandler) processVMImage(vmi *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	err := h.backends[util.GetVMIBackend(vmi)].Check(vmi)
	if common.IsRetryAble(err) {
		return h.handleRetry(vmi)
	}

	if common.IsRetryLater(err) {
		h.vmiController.EnqueueAfter(vmi.Namespace, vmi.Name, checkInterval)
		return vmi, nil
	}

	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"namespace": vmi.Namespace,
			"name":      vmi.Name,
		}).Error("failed to process vm image")
		return vmi, err
	}

	return vmi, nil
}

func (h *vmImageHandler) OnRemove(_ string, vmi *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if vmi == nil {
		return nil, nil
	}

	return vmi, h.backends[util.GetVMIBackend(vmi)].Delete(vmi)
}

func (h *vmImageHandler) initialize(vmi *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	toUpdate, err := h.backends[util.GetVMIBackend(vmi)].Initialize(vmi)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"namespace": toUpdate.Namespace,
			"name":      toUpdate.Name,
		}).Error("failed to initialize vm image")
		return h.vmio.FailInitial(toUpdate, err)
	}
	return h.vmio.Initialized(toUpdate)
}

func (h *vmImageHandler) handleRetry(vmi *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if vmi.Status.LastFailedTime == "" {
		return h.initialize(vmi)
	}

	ts, err := time.Parse(time.RFC3339, vmi.Status.LastFailedTime)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"namespace": vmi.Namespace,
			"name":      vmi.Name,
		}).Errorf("failed to parse lastFailedTime %s for vmimage", vmi.Status.LastFailedTime)
		return h.vmio.UpdateLastFailedTime(vmi)
	}

	ts = ts.Add(10 * time.Second)
	if time.Now().Before(ts) {
		h.vmiController.EnqueueAfter(vmi.Namespace, vmi.Name, 10*time.Second)
		return vmi, nil
	}
	return h.initialize(vmi)
}
