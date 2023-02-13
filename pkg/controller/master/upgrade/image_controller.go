package upgrade

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
)

// vmImageHandler syncs upgrade repo image creation
type vmImageHandler struct {
	namespace     string
	upgradeClient ctlharvesterv1.UpgradeClient
	upgradeCache  ctlharvesterv1.UpgradeCache
}

func (h *vmImageHandler) OnChanged(key string, image *harvesterv1.VirtualMachineImage) (*harvesterv1.VirtualMachineImage, error) {
	if image == nil || image.DeletionTimestamp != nil || image.Labels == nil || image.Namespace != upgradeNamespace || image.Labels[harvesterUpgradeLabel] == "" {
		return image, nil
	}

	upgradeControllerLock.Lock()
	defer upgradeControllerLock.Unlock()

	upgrade, err := h.upgradeCache.Get(upgradeNamespace, image.Labels[harvesterUpgradeLabel])
	if err != nil {
		if apierrors.IsNotFound(err) {
			return image, nil
		}
		return nil, err
	}

	toUpdate := upgrade.DeepCopy()

	switch {
	case harvesterv1.ImageImported.IsTrue(image):
		setImageReadyCondition(toUpdate, corev1.ConditionTrue, "", "")
	case harvesterv1.ImageImported.IsFalse(image):
		setImageReadyCondition(toUpdate, corev1.ConditionFalse, harvesterv1.ImageImported.GetReason(image), harvesterv1.ImageImported.GetMessage(image))
	case harvesterv1.ImageRetryLimitExceeded.IsTrue(image):
		setImageReadyCondition(toUpdate, corev1.ConditionFalse, harvesterv1.ImageRetryLimitExceeded.GetReason(image), harvesterv1.ImageRetryLimitExceeded.GetMessage(image))
	default:
		return image, nil
	}

	if !reflect.DeepEqual(toUpdate, upgrade) {
		_, err := h.upgradeClient.Update(toUpdate)
		return image, err
	}

	return image, nil
}
